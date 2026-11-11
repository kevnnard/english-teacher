package tui

import (
	"context"
	"errors"
	"fmt"
	"math/rand/v2"
	"strings"
	"time"
	"unicode"

	tea "charm.land/bubbletea/v2"

	"github.com/kevnnard/english-teacher/internal/check"
	"github.com/kevnnard/english-teacher/internal/domain"
	"github.com/kevnnard/english-teacher/internal/speech"
)

// Shadowing (spec S6): the Speak screen plays a sentence from the
// learner's own examples, the learner says it while it is recorded, and
// whisper writes down what it heard, on this computer. Each word of the
// sentence is marked understood or not, by check.Understood.
//
// Words not understood become practice items, for this round only: at
// the end of the round every sentence that had a word not understood
// comes back once, as "practice again", and the result lists the words to
// practise. Nothing of it is stored but the activity in today's log.

// speakItems is how many sentences one round plays.
const speakItems = 8

// maxSpeakMinutes caps the study time one round can record, like
// maxQuizMinutes: a round left open does not fill the daily goal.
const maxSpeakMinutes = maxQuizMinutes

// whisperHelp says how to set up listening, for the Speak screen.
const whisperHelp = "Speak needs whisper to hear you. Install whisper.cpp with `brew install whisper-cpp` " +
	"(or see the whisper.cpp README), then run `teacher setup whisper` to download its model. " +
	"Recording needs pw-record (pipewire) or arecord (alsa-utils). Run `teacher doctor` to check."

// whisperOffNotice is whisperHelp in one line, for a screen where speaking
// is only one way to answer, like Talk.
const whisperOffNotice = "Speaking is off: `brew install whisper-cpp`, then `teacher setup whisper`"

// heardNothing is what the app says when whisper wrote nothing down.
const heardNothing = "Whisper heard nothing. Is your microphone plugged in and turned up?"

// speakPhase is where the learner is in a round.
type speakPhase int

const (
	speakIntro     speakPhase = iota // what the round is; enter starts
	speakReady                       // the sentence played; enter starts recording
	speakRecording                   // enter stops, esc cancels
	speakListening                   // whisper is transcribing
	speakAnswered                    // the marks of the last try
	speakResult                      // every sentence is done
)

// speakItem is one sentence of a round.
type speakItem struct {
	sentence string
	again    bool // a practice-again turn, at the end of the round
	tries    int  // recordings whisper transcribed
	full     bool // the first try understood every word
	missed   bool // some try had a word not understood
}

// speakState is the Speak screen: one round, its tries and its result.
type speakState struct {
	phase       speakPhase
	count       int // rounds started, mixed into the seed so each one differs
	items       []speakItem
	pos         int  // the item on screen
	queuedAgain bool // the practice-again turns are already queued

	recording speech.Recording // the recording in progress, while recording
	try       int              // recordings started, so an old transcript is ignored

	marks   []check.WordMark // the words of the last try
	heard   string           // what whisper heard in the last try
	note    string           // what went wrong with the last recording, if anything
	offHint string           // why whisper is off, found while recording

	practice  []string  // the words not understood, in order, once each
	startedAt time.Time // when the first sentence played, for the study minutes
}

// typing reports whether every key belongs to the screen: while it
// records or listens, only enter and esc mean anything.
func (s speakState) typing() bool {
	return s.phase == speakRecording || s.phase == speakListening
}

// mainItems is how many sentences the round has, practice-again turns
// aside.
func (s speakState) mainItems() int {
	n := 0
	for _, it := range s.items {
		if !it.again {
			n++
		}
	}
	return n
}

// score is how many sentences were fully understood at the first try.
func (s speakState) score() int {
	n := 0
	for _, it := range s.items {
		if !it.again && it.full {
			n++
		}
	}
	return n
}

// transcribedMsg is what whisper heard in recording try; err is why the
// recording or the transcription failed. talk is set when the recording
// was a Talk line rather than a Speak sentence.
type transcribedMsg struct {
	try  int
	talk bool
	text string
	err  error
}

// transcribe returns a command that stops rec and transcribes it with tr.
// Both take a while, so they run outside the UI.
func transcribe(rec speech.Recording, tr speech.Transcriber, msg transcribedMsg) tea.Cmd {
	return func() tea.Msg {
		wav, err := rec.Stop()
		if err != nil {
			msg.err = err
			return msg
		}
		msg.text, msg.err = tr.Transcribe(context.Background(), wav)
		return msg
	}
}

// speakOptions are the sentences the learner's cards give now, with the
// seed a round would start with: dictation's sentences, at most
// speakItems.
func (a App) speakOptions() []string {
	seed := uint64(a.now().UnixNano())
	all := dictationSentences(a.cards, rand.New(rand.NewPCG(seed, uint64(a.speaking.count+1))))
	return all[:min(len(all), speakItems)]
}

// whisperOn reports whether the app can record and transcribe.
func (a App) whisperOn() bool {
	return a.recorder != nil && a.transcriber != nil && a.speaking.offHint == ""
}

// canSpeak reports whether enter on the intro starts a round: it needs
// audio, whisper and enough sentences.
func (a App) canSpeak() bool {
	return a.speaker != nil && a.whisperOn() && len(a.speakOptions()) >= minDictationItems
}

// startSpeak plays the first sentence of a new round.
func (a App) startSpeak() (App, tea.Cmd) {
	if !a.canSpeak() {
		return a, nil
	}
	var items []speakItem
	for _, s := range a.speakOptions() {
		items = append(items, speakItem{sentence: s})
	}
	a.speaking = speakState{
		phase: speakReady, count: a.speaking.count + 1, try: a.speaking.try,
		items: items, startedAt: a.now(),
	}
	return a, a.playSpeak(false)
}

// playSpeak says the sentence on screen, slowly when slow is set.
func (a App) playSpeak(slow bool) tea.Cmd {
	s := a.speaking
	return a.say([]string{s.items[s.pos].sentence}, speech.Options{Slow: slow})
}

// startRecording starts recording the learner. Starting a program takes
// milliseconds, so it happens right here.
func (a *App) startRecording() {
	s := &a.speaking
	s.note = ""
	rec, err := a.recorder.Start(context.Background())
	if err != nil {
		a.whisperFailed(err)
		return
	}
	s.recording, s.phase = rec, speakRecording
	s.try++
}

// stopRecording stops the recording and transcribes it, in a command:
// both take a while, and the page says "Listening…" meanwhile.
func (a *App) stopRecording() tea.Cmd {
	s := &a.speaking
	rec := s.recording
	s.recording, s.phase = nil, speakListening
	return transcribe(rec, a.transcriber, transcribedMsg{try: s.try})
}

// cancelRecording throws the recording away; the sentence waits for
// another try.
func (a *App) cancelRecording() tea.Cmd {
	s := &a.speaking
	rec := s.recording
	s.recording, s.phase, s.note = nil, speakReady, "Recording cancelled."
	return func() tea.Msg {
		rec.Cancel()
		return nil
	}
}

// whisperFailed shows why a recording failed: a missing piece sends the
// learner to the install page, anything else allows another try.
func (a *App) whisperFailed(err error) {
	s := &a.speaking
	var unavailable *speech.UnavailableError
	switch {
	case errors.As(err, &unavailable):
		s.offHint, s.phase = unavailable.Error(), speakIntro
	case errors.Is(err, speech.ErrUnavailable):
		s.offHint, s.phase = err.Error(), speakIntro
	default:
		s.note, s.phase = "Could not hear you: "+err.Error(), speakReady
	}
}

// gotTranscript marks the words of the sentence on screen that whisper
// heard.
func (a App) gotTranscript(msg transcribedMsg) App {
	s := &a.speaking
	if msg.try != s.try || s.phase != speakListening {
		return a // a transcript of an older recording
	}
	if msg.err != nil {
		a.whisperFailed(msg.err)
		return a
	}
	it := &s.items[s.pos]
	s.heard = msg.text
	s.marks = check.Understood(it.sentence, msg.text)
	s.phase = speakAnswered

	full := true
	for _, m := range s.marks {
		if !m.Understood {
			full = false
			s.addPractice(m.Word)
		}
	}
	if it.tries == 0 {
		it.full = full
	}
	it.tries++
	it.missed = it.missed || !full
	return a
}

// addPractice adds word to the words to practise, once, without its
// punctuation.
func (s *speakState) addPractice(word string) {
	w := strings.TrimFunc(word, func(r rune) bool { return unicode.IsPunct(r) && r != '\'' })
	w = strings.ToLower(w)
	for _, p := range s.practice {
		if p == w {
			return
		}
	}
	s.practice = append(s.practice, w)
}

// nextSpeak moves to the next sentence. After the last one, the
// sentences with a word not understood come back once; after those, the
// round ends and is recorded, once.
func (a App) nextSpeak() (App, tea.Cmd) {
	s := &a.speaking
	if s.pos+1 == len(s.items) && !s.queuedAgain {
		s.queuedAgain = true
		for _, it := range s.items {
			if it.missed {
				s.items = append(s.items, speakItem{sentence: it.sentence, again: true})
			}
		}
	}
	if s.pos+1 < len(s.items) {
		s.pos++
		s.phase, s.marks, s.heard, s.note = speakReady, nil, "", ""
		return a, a.playSpeak(false)
	}
	s.phase = speakResult
	minutes := min(int(a.now().Sub(s.startedAt)/time.Minute), maxSpeakMinutes)
	activity := fmt.Sprintf("Speak: %d/%d", s.score(), s.mainItems())
	a.addToToday(domain.Day{Minutes: max(minutes, 0), Activities: []string{activity}})
	return a, nil
}

func (a App) updateSpeak(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	s := &a.speaking
	k := msg.String()
	switch s.phase {
	case speakIntro:
		if k == "enter" {
			return a.startSpeak()
		}
	case speakReady:
		switch k {
		case "enter":
			a.startRecording()
		case "p", "s":
			return a, a.playSpeak(k == "s")
		}
	case speakRecording:
		switch k {
		case "enter":
			return a, a.stopRecording()
		case "esc":
			return a, a.cancelRecording()
		}
	case speakAnswered:
		switch k {
		case "enter":
			return a.nextSpeak()
		case "r":
			s.phase, s.marks, s.heard = speakReady, nil, ""
			return a, a.playSpeak(false)
		case "p", "s":
			return a, a.playSpeak(k == "s")
		}
	case speakResult:
		if k == "n" {
			a.speaking = speakState{count: s.count, try: s.try}
		}
	}
	return a, nil
}
