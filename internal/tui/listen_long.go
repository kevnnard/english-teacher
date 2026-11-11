package tui

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/kevnnard/english-teacher/internal/domain"
	"github.com/kevnnard/english-teacher/internal/speech"
	"github.com/kevnnard/english-teacher/internal/teacher"
)

// Long listening (spec S2, extensive listening) lives inside Listen: s
// switches between dictation and a dialogue or story of two to five
// minutes. The learner first hears it with only its progress on screen,
// then answers its questions, then reads the transcript and replays it
// sentence by sentence.
//
// Playback is one command per sentence: when a sentence ends, its message
// starts the next one. Every playback has a cancelable context; pausing,
// replaying, esc or leaving the screen cancels it, which stops piper and
// the player at once, and the message of a cancelled sentence is ignored
// because its play id is old.

// maxLongListenMinutes caps the study time one long listening can record:
// up to five minutes of audio, replays and the questions.
const maxLongListenMinutes = 30

// longPhase is where the learner is in a long listening.
type longPhase int

const (
	longIdle       longPhase = iota // not opened since the app started
	longLoading                     // waiting for the teacher
	longFailed                      // the teacher failed
	longListening                   // hearing the text without seeing it
	longQuestions                   // answering the comprehension questions
	longTranscript                  // reading the text and replaying its sentences
)

// longState is the long-listening half of Listen.
type longState struct {
	on       bool // Listen shows the long listening instead of dictation
	phase    longPhase
	req      int // the id of the latest request; older answers are ignored
	count    int // texts asked for; odd ones are dialogues, even ones stories
	text     teacher.Listening
	speakers []string // the speakers in the order they first speak, for their voices
	pos      int      // the sentence playing or, in the transcript, selected

	playing bool               // a sentence is being said now
	heard   bool               // the text played to the end once, so the questions are open
	play    int                // the id of the current playback; answers of older ones are ignored
	ctx     context.Context    // the context of the current playback, shared by its sentences
	cancel  context.CancelFunc // cancels ctx; nil when nothing plays

	question  int   // the question on screen
	answers   []int // the option chosen for each answered question, from 0
	startedAt time.Time
	err       error // why the text could not be written
}

// stop cancels the playback, if any. The sentence being said stops at
// once and its message, which comes later, is ignored.
func (s *longState) stop() {
	if s.cancel != nil {
		s.cancel()
		s.ctx, s.cancel = nil, nil
	}
	s.playing = false
	s.play++
}

// score is how many questions were answered right.
func (s longState) score() int {
	n := 0
	for i, c := range s.answers {
		if c == s.text.Questions[i].Answer {
			n++
		}
	}
	return n
}

// answered reports whether the question on screen has its answer.
func (s longState) answered() bool { return s.question < len(s.answers) }

// listeningMsg is the teacher's answer to request req.
type listeningMsg struct {
	req  int
	text teacher.Listening
	err  error
}

// lineSpokenMsg says sentence pos of playback play ended; err is why it
// failed, if it did.
type lineSpokenMsg struct {
	play, pos int
	err       error
}

// toggleLong switches Listen between dictation and long listening. The
// first time, with audio and a teacher, it asks for a text.
func (a App) toggleLong() (App, tea.Cmd) {
	s := &a.long
	s.on = !s.on
	if !s.on {
		s.stop()
		return a, nil
	}
	if s.phase != longIdle || a.speaker == nil || a.teacher == nil {
		return a, nil // without audio or pi the view says why; nothing is asked
	}
	return a.askListening()
}

// askListening asks for a new text: a dialogue, then a story, in turns.
func (a App) askListening() (App, tea.Cmd) {
	s := &a.long
	s.stop()
	s.count++
	s.req++
	s.phase, s.err = longLoading, nil
	kind := teacher.ListeningDialogue
	if s.count%2 == 0 {
		kind = teacher.ListeningStory
	}
	req, tc, level, known := s.req, a.teacher, a.profile.Level, a.knownWords()
	return a, func() tea.Msg {
		text, err := tc.ListeningText(context.Background(), level, known, kind)
		return listeningMsg{req: req, text: text, err: err}
	}
}

// gotListening shows the answer to the latest request and starts playing
// it, unless the learner has left the screen meanwhile.
func (a App) gotListening(msg listeningMsg) (App, tea.Cmd) {
	s := &a.long
	if msg.req != s.req || s.phase != longLoading {
		return a, nil
	}
	if msg.err != nil {
		s.phase, s.err = longFailed, msg.err
		return a, nil
	}
	s.text, s.phase, s.pos, s.heard, s.question, s.answers = msg.text, longListening, 0, false, 0, nil
	s.startedAt = a.now()
	s.speakers = nil
	for _, l := range msg.text.Lines {
		if !slices.Contains(s.speakers, l.Speaker) {
			s.speakers = append(s.speakers, l.Speaker)
		}
	}
	if a.screen != Listen || !s.on {
		return a, nil // it waits, paused, for the learner to come back
	}
	return a.playLong(0, false)
}

// lineVoice is the voice of sentence i: with two or more voices installed,
// each speaker keeps one; else every line uses the configured voice.
func (a App) lineVoice(i int) string {
	if len(a.voices) < 2 {
		return ""
	}
	n := slices.Index(a.long.speakers, a.long.text.Lines[i].Speaker)
	return a.voices[max(n, 0)%len(a.voices)]
}

// playLong stops any playback and plays sentence pos, slowly when slow
// is set, with a new cancelable context. While listening, each sentence
// chains to the next one when it ends.
func (a App) playLong(pos int, slow bool) (App, tea.Cmd) {
	s := &a.long
	s.stop()
	ctx, cancel := context.WithCancel(context.Background())
	s.ctx, s.cancel, s.playing, s.pos = ctx, cancel, true, pos
	return a, a.sayLine(ctx, s.play, pos, slow)
}

// sayLine returns a command that says sentence pos of playback play.
func (a App) sayLine(ctx context.Context, play, pos int, slow bool) tea.Cmd {
	sp, text := a.speaker, a.long.text.Lines[pos].Text
	opts := speech.Options{Slow: slow, Voice: a.lineVoice(pos)}
	return func() tea.Msg {
		return lineSpokenMsg{play: play, pos: pos, err: sp.Say(ctx, text, opts)}
	}
}

// gotLineSpoken goes on to the next sentence while listening, or ends the
// playback. A failed sentence shows why and opens the questions, so a
// broken audio setup never traps the learner.
func (a App) gotLineSpoken(msg lineSpokenMsg) (App, tea.Cmd) {
	s := &a.long
	if msg.play != s.play || !s.playing {
		return a, nil // a cancelled or older playback
	}
	if msg.err != nil {
		s.stop()
		s.heard = true
		if !errors.Is(msg.err, context.Canceled) {
			a = a.gotSpoken(spokenMsg{err: msg.err})
		}
		return a, nil
	}
	if s.phase == longListening && msg.pos+1 < len(s.text.Lines) {
		s.pos = msg.pos + 1
		return a, a.sayLine(s.ctx, s.play, s.pos, false) // the same playback, the same context
	}
	if s.phase == longListening {
		s.heard = true
	}
	s.stop()
	return a, nil
}

// stopLongWhenAway stops the playback when the learner is no longer on
// Listen's long listening.
func (a App) stopLongWhenAway() App {
	if a.long.playing && (a.screen != Listen || !a.long.on) {
		a.long.stop()
	}
	return a
}

func (a App) updateLong(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	s := &a.long
	k := msg.String()
	if k == "s" {
		return a.toggleLong()
	}
	switch s.phase {
	case longFailed:
		if k == "enter" && a.speaker != nil && a.teacher != nil {
			return a.askListening()
		}
	case longListening:
		switch k {
		case "space":
			if s.playing {
				s.stop()
				return a, nil
			}
			return a.playLong(s.pos, false)
		case "r":
			return a.playLong(0, false)
		case "enter":
			if s.heard {
				s.stop()
				s.phase = longQuestions
			}
		}
	case longQuestions:
		a.answerLong(k)
	case longTranscript:
		switch k {
		case "down", "j":
			s.pos = min(s.pos+1, len(s.text.Lines)-1)
		case "up", "k":
			s.pos = max(s.pos-1, 0)
		case "p":
			return a.playLong(s.pos, a.slowAgain([]string{s.text.Lines[s.pos].Text}))
		case "n":
			return a.askListening()
		}
	}
	return a, nil
}

// answerLong handles a key on the questions: 1–3 answers the question on
// screen, and enter goes on to the next one or, after the last, to the
// transcript. The last answer records the activity, once.
func (a *App) answerLong(k string) {
	s := &a.long
	if s.answered() {
		if k != "enter" {
			return
		}
		if s.question+1 < len(s.text.Questions) {
			s.question++
		} else {
			s.phase, s.pos = longTranscript, 0
		}
		return
	}
	if len(k) != 1 || k[0] < '1' || int(k[0]-'1') >= teacher.ListeningOptions {
		return
	}
	s.answers = append(s.answers, int(k[0]-'1'))
	if len(s.answers) < len(s.text.Questions) {
		return
	}
	minutes := min(int(a.now().Sub(s.startedAt)/time.Minute), maxLongListenMinutes)
	activity := fmt.Sprintf("Listening: %d/%d", s.score(), len(s.text.Questions))
	a.addToToday(domain.Day{Minutes: max(minutes, 0), Activities: []string{activity}})
}
