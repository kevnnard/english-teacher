package tui

import (
	"context"
	"errors"
	"slices"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/kevnnard/english-teacher/internal/domain"
	"github.com/kevnnard/english-teacher/internal/speech"
)

// Listening (spec S3): p reads aloud what the learner is looking at, on
// the screens where hearing it helps and gives nothing away. While the
// learner types, p is a letter, so audio is never offered then.

// slowRepeat is how soon a second p on the same text must come to be
// played slowly: "p p" means "again, slower".
const slowRepeat = 2 * time.Second

// audioOffNotice is the notice when the app has no speaker at all.
const audioOffNotice = "Audio is off: run `teacher doctor`"

// listenState remembers the last play, so pressing p again soon is
// slower, and the notice of the last audio problem.
type listenState struct {
	texts  []string  // what the last p asked to say
	at     time.Time // when
	notice string    // why audio did not play; cleared by the next key
}

// spokenMsg says a play ended; err is why it failed, if it did.
type spokenMsg struct{ err error }

// listenTexts is what p says on the page on screen, in order, or nil
// when there is nothing to hear yet.
func (a App) listenTexts() []string {
	switch a.screen {
	case Review:
		return a.reviewListen()
	case Learn:
		if s := a.story; s.on {
			if s.phase == storyGlossary && len(s.chapter.Glossary) > 0 {
				return []string{s.chapter.Glossary[s.cursor].Word}
			}
			return nil
		}
		l := a.learn
		if l.phase == learnReading && len(l.reading.Glossary) > 0 {
			return []string{l.reading.Glossary[l.cursor].Word}
		}
	case Write:
		if right := a.write.rightVersion(); right != "" {
			return []string{right}
		}
	case Quiz:
		if q := a.quiz; q.phase == quizAnswered {
			return []string{q.questions[q.pos].word}
		}
	case Listen:
		if d := a.dictation; !a.long.on && d.phase == dictationAnswered {
			return []string{d.items[d.pos]}
		}
	}
	return nil
}

// reviewListen is the word of the card on screen and, once revealed, its
// example. A production card's English is its answer, so it is said only
// after the answer is checked.
func (a App) reviewListen() []string {
	s := a.current()
	if len(s.queue) == 0 || s.done() {
		return nil
	}
	c := a.cards[s.queue[s.pos]]
	switch {
	case c.Direction == domain.Production && s.phase != checked:
		return nil
	case s.phase == revealed && c.Example != "":
		return []string{c.Word, c.Example}
	}
	return []string{c.Word}
}

// rightVersion is the right version of the error the learner works on in
// the feedback (the first one not fixed) or in a checked rewrite (the
// current one); "" anywhere else.
func (w writeState) rightVersion() string {
	switch {
	case w.phase == writeFeedback && len(w.rewrites) > 0:
		return w.majors[w.rewrites[0]].Right
	case w.phase == writeRewriting && w.checked:
		return w.majors[w.rewrites[w.cursor]].Right
	}
	return ""
}

// readAllText is what P reads in Learn: the whole text or chapter, or ""
// when none is on screen.
func (a App) readAllText() string {
	if a.screen != Learn {
		return ""
	}
	if s := a.story; s.on {
		if s.phase == storyReading || s.phase == storyGlossary {
			return s.saved.Text
		}
		return ""
	}
	if a.learn.phase == learnReading {
		return a.learn.reading.Text
	}
	return ""
}

// listenKey handles p and P. It reports false when the key has nothing
// to say here, so the screen gets it as usual.
func (a App) listenKey(k string) (App, tea.Cmd, bool) {
	var texts []string
	switch k {
	case "p":
		texts = a.listenTexts()
	case "P", "shift+p":
		if t := a.readAllText(); t != "" {
			texts = []string{t}
		}
	}
	if len(texts) == 0 {
		return a, nil, false
	}
	a, cmd := a.speak(texts)
	return a, cmd, true
}

// speak returns a command that says texts one after the other, without
// blocking the UI. The same texts again within slowRepeat are slower.
func (a App) speak(texts []string) (App, tea.Cmd) {
	if a.speaker == nil {
		a.listen.notice = audioOffNotice
		return a, nil
	}
	return a, a.say(texts, speech.Options{Slow: a.slowAgain(texts)})
}

// slowAgain reports whether texts are what the last play asked for, less
// than slowRepeat ago, so this play is slower; it remembers this play. A
// third press is normal speed again.
func (a *App) slowAgain(texts []string) bool {
	now := a.now()
	slow := slices.Equal(texts, a.listen.texts) && now.Sub(a.listen.at) < slowRepeat
	a.listen.texts, a.listen.at = texts, now
	if slow {
		a.listen.at = time.Time{}
	}
	return slow
}

// say returns a command that says texts one after the other with opts,
// without blocking the UI; nil without a speaker.
func (a App) say(texts []string, opts speech.Options) tea.Cmd {
	sp := a.speaker
	if sp == nil {
		return nil
	}
	return func() tea.Msg {
		for _, t := range texts {
			if err := sp.Say(context.Background(), t, opts); err != nil {
				return spokenMsg{err: err}
			}
		}
		return spokenMsg{}
	}
}

// gotSpoken shows why a play failed. Audio problems never stop the app.
func (a App) gotSpoken(msg spokenMsg) App {
	var unavailable *speech.UnavailableError
	switch {
	case msg.err == nil:
	case errors.As(msg.err, &unavailable):
		a.listen.notice = "Audio is off: " + unavailable.Hint
	case errors.Is(msg.err, speech.ErrUnavailable):
		a.listen.notice = audioOffNotice
	default:
		a.listen.notice = "Could not play audio: " + msg.err.Error()
	}
	return a
}

// listenHints are the audio keys for the status bar: none without a
// speaker, so the app looks as before when audio is not set up.
func (a App) listenHints() []string {
	if a.speaker == nil {
		return nil
	}
	var hints []string
	if len(a.listenTexts()) > 0 {
		hints = append(hints, "p listen")
	}
	if a.readAllText() != "" {
		hints = append(hints, "P read all")
	}
	return hints
}
