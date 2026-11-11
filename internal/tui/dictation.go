package tui

import (
	"fmt"
	"math/rand/v2"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/kevnnard/english-teacher/internal/check"
	"github.com/kevnnard/english-teacher/internal/domain"
	"github.com/kevnnard/english-teacher/internal/speech"
)

// Dictation (spec S4): the Listen screen plays a sentence from the
// learner's own examples and the learner types what they heard. Like a
// quiz, it is practice, not grading: it never changes a schedule.

// maxListenMinutes caps the study time one dictation can record, like
// maxQuizMinutes: a dictation left open does not fill the daily goal.
const maxListenMinutes = maxQuizMinutes

// dictationPhase is where the learner is in a dictation.
type dictationPhase int

const (
	dictationIntro    dictationPhase = iota // what dictation is; enter starts
	dictationAsking                         // the sentence played; the learner types it
	dictationAnswered                       // the feedback of the sentence on screen
	dictationResult                         // every sentence is answered
)

// dictationAnswer is what the learner typed for one sentence.
type dictationAnswer struct {
	typed  string       // what the learner wrote; "" means "I don't know"
	result check.Result // the check of typed against the sentence
	right  bool         // exact, or almost: a small typo still counts
	missed []string     // the words of the sentence typed wrong or not at all
}

// dictationState is the Listen screen: one dictation, its answers and
// its result.
type dictationState struct {
	phase     dictationPhase
	count     int      // dictations started, mixed into the seed so each one differs
	items     []string // the sentences, in the order they are played
	answers   []dictationAnswer
	pos       int       // the sentence on screen
	input     string    // the typed answer so far
	startedAt time.Time // when the first sentence played, for the study minutes
}

// typing reports whether the keys belong to the typed answer. The input
// is always focused while a sentence waits for its answer.
func (d dictationState) typing() bool { return d.phase == dictationAsking }

// score is how many answers were right.
func (d dictationState) score() int {
	n := 0
	for _, ans := range d.answers {
		if ans.right {
			n++
		}
	}
	return n
}

// dictationOptions are the sentences the learner's cards give now, with
// the seed a dictation would start with.
func (a App) dictationOptions() []string {
	seed := uint64(a.now().UnixNano())
	return dictationSentences(a.cards, rand.New(rand.NewPCG(seed, uint64(a.dictation.count+1))))
}

// canDictate reports whether enter on the intro starts a dictation: it
// needs audio and enough sentences.
func (a App) canDictate() bool {
	return a.speaker != nil && len(a.dictationOptions()) >= minDictationItems
}

// startDictation plays the first sentence of a new dictation.
func (a App) startDictation() (App, tea.Cmd) {
	if !a.canDictate() {
		return a, nil
	}
	items := a.dictationOptions()
	a.dictation = dictationState{
		phase: dictationAsking, count: a.dictation.count + 1,
		items: items, startedAt: a.now(),
	}
	return a, a.playDictation(false)
}

// playDictation says the sentence on screen, slowly when slow is set.
func (a App) playDictation(slow bool) tea.Cmd {
	d := a.dictation
	return a.say([]string{d.items[d.pos]}, speech.Options{Slow: slow})
}

// checkDictation records the learner's answer to the sentence on screen
// and shows its feedback. The last answer records the dictation, once.
func (a *App) checkDictation() {
	d := &a.dictation
	sentence := d.items[d.pos]
	res := check.Answer(dictationKey(d.input), dictationKey(sentence))
	ans := dictationAnswer{typed: d.input, result: res, right: res.Kind != check.Wrong}
	if !ans.right {
		ans.missed = missedWords(d.input, sentence)
	}
	d.answers = append(d.answers, ans)
	d.input, d.phase = "", dictationAnswered
	if len(d.answers) < len(d.items) {
		return
	}
	minutes := min(int(a.now().Sub(d.startedAt)/time.Minute), maxListenMinutes)
	activity := fmt.Sprintf("Listen: %d/%d", d.score(), len(d.items))
	a.addToToday(domain.Day{Minutes: max(minutes, 0), Activities: []string{activity}})
}

func (a App) updateDictation(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	d := &a.dictation
	k := msg.String()
	switch k {
	case "ctrl+p", "ctrl+s":
		if d.phase == dictationAsking || d.phase == dictationAnswered {
			return a, a.playDictation(k == "ctrl+s")
		}
		return a, nil
	}
	switch d.phase {
	case dictationIntro:
		if k == "enter" {
			return a.startDictation()
		}
	case dictationAsking:
		switch k {
		case "enter":
			a.checkDictation()
		case "esc":
			a.screen = Home // the dictation stays for later
		default:
			d.input = editInput(d.input, msg)
		}
	case dictationAnswered:
		if k == "enter" {
			if d.pos+1 < len(d.items) {
				d.pos++
				d.phase = dictationAsking
				return a, a.playDictation(false)
			}
			d.phase = dictationResult
		}
	case dictationResult:
		if k == "n" {
			a.dictation = dictationState{count: d.count}
		}
	}
	return a, nil
}
