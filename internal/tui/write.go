package tui

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/kevnnard/english-teacher/internal/check"
	"github.com/kevnnard/english-teacher/internal/domain"
	"github.com/kevnnard/english-teacher/internal/teacher"
)

// maxWritingMinutes caps the study time one writing task can record.
const maxWritingMinutes = 30

// recentWordCount is how many of the newest words a writing task suggests.
const recentWordCount = 10

// focusKinds is how many recurring error kinds the next task targets.
const focusKinds = 2

// recurringCount is how many times an error kind must be seen before it
// counts as a pattern.
const recurringCount = 2

// writePhase is where the learner is in the six steps of Write (spec S13):
// plan (the task), write, self-correct, feedback, rewrite and result.
type writePhase int

const (
	writeIdle      writePhase = iota // nothing asked yet
	writeTaskWait                    // waiting for a task
	writeTaskFail                    // the teacher could not give a task
	writeComposing                   // steps 1 and 2: the task and its phrases; the learner writes
	writeChecking                    // waiting for the correction
	writeFixing                      // step 3: the learner fixes each marked error alone
	writeJudging                     // waiting for the judge of the learner's fixes
	writeFeedback                    // step 4: what was fixed, what was not and why
	writeRewriting                   // step 5: the learner writes each unfixed error again
	writeResult                      // step 6: the rubric, the cards and the pattern
)

// majorFix is one major error and what the learner did with it.
type majorFix struct {
	teacher.Fix
	attempt string       // the learner's own fix in step 3; "" means "I don't know"
	fixed   bool         // the attempt fixes the error
	rewrite check.Result // the step 5 check of an error the learner did not fix
}

// writeState is the Write screen: one task, the learner's text, its
// correction and the learner's work on the major errors.
type writeState struct {
	phase      writePhase
	req        int // the id of the latest request; older answers are ignored
	task       teacher.WritingTask
	text       string
	startedAt  time.Time // when the task appeared, for the study minutes
	correction teacher.Correction

	majors   []majorFix
	cursor   int    // the error being fixed (step 3) or the position in rewrites (step 5)
	input    string // what the learner is typing in step 3 or 5
	checked  bool   // step 5: the current rewrite was checked
	judged   []int  // indexes in majors sent to the judge
	judgeErr error  // the judge failed; its attempts count as not fixed
	rewrites []int  // indexes in majors the learner did not fix, for step 5
	minors   bool   // the minor details are shown

	patternKind  string // the most repeated error kind of this text, when it is a pattern
	patternCount int    // how many times patternKind has been seen, this text included
	logErr       error  // the major errors could not be logged

	cards []domain.Card // the error cards, kept until they are saved
	added int
	err   error // the last failed request or save, shown on the page
}

// typing reports whether every key is a letter of the learner's text.
func (w writeState) typing() bool {
	switch w.phase {
	case writeComposing, writeFixing:
		return true
	case writeRewriting:
		return !w.checked
	}
	return false
}

// taskMsg, correctionMsg and judgeMsg are the teacher's answers to request req.
type taskMsg struct {
	req  int
	task teacher.WritingTask
	err  error
}

type correctionMsg struct {
	req        int
	correction teacher.Correction
	err        error
}

type judgeMsg struct {
	req      int
	verdicts []bool
	err      error
}

// askTask starts a request for a new writing task aimed at the learner's
// recurring errors.
func (a App) askTask() (App, tea.Cmd) {
	if a.teacher == nil {
		return a, nil
	}
	a.write = writeState{phase: writeTaskWait, req: a.write.req + 1}
	req, tc, level, recent, focus := a.write.req, a.teacher, a.studyLevel(), a.recentWords(), a.focus()
	return a, func() tea.Msg {
		t, err := tc.WritingTask(context.Background(), level, recent, focus)
		return taskMsg{req: req, task: t, err: err}
	}
}

func (a App) gotTask(msg taskMsg) App {
	if msg.req != a.write.req || a.write.phase != writeTaskWait {
		return a
	}
	if msg.err != nil {
		a.write.phase, a.write.err = writeTaskFail, msg.err
		return a
	}
	a.write.phase, a.write.task, a.write.startedAt = writeComposing, msg.task, a.now()
	return a
}

// focus is the error kinds the next task should practice: the focusKinds
// most counted kinds seen at least recurringCount times, in teacher.Kinds
// order on a tie. "Other" says nothing to practice, so it never counts.
func (a App) focus() []string {
	kinds := a.recurringKinds()
	return kinds[:min(len(kinds), focusKinds)]
}

// recurringKinds are the error kinds seen at least recurringCount times,
// most frequent first, without OtherKind.
func (a App) recurringKinds() []string {
	var kinds []string
	for _, k := range teacher.Kinds {
		if k.Name != teacher.OtherKind && a.mistakeCounts[k.Name] >= recurringCount {
			kinds = append(kinds, k.Name)
		}
	}
	slices.SortStableFunc(kinds, func(x, y string) int { return a.mistakeCounts[y] - a.mistakeCounts[x] })
	return kinds
}

// askCorrection sends the learner's text to the teacher.
func (a App) askCorrection() (App, tea.Cmd) {
	text := strings.TrimSpace(a.write.text)
	if text == "" {
		return a, nil
	}
	a.write.req++
	a.write.phase, a.write.err = writeChecking, nil
	req, tc, level, task := a.write.req, a.teacher, a.studyLevel(), a.write.task.Task
	return a, func() tea.Msg {
		c, err := tc.Correct(context.Background(), level, task, text)
		return correctionMsg{req: req, correction: c, err: err}
	}
}

// gotCorrection records the writing as study and starts the
// self-correction of the major errors.
func (a App) gotCorrection(msg correctionMsg) App {
	w := &a.write
	if msg.req != w.req || w.phase != writeChecking {
		return a
	}
	if msg.err != nil {
		w.phase, w.err = writeComposing, msg.err
		return a
	}
	w.correction = msg.correction
	minutes := min(int(a.now().Sub(w.startedAt)/time.Minute), maxWritingMinutes)
	a.addToToday(domain.Day{
		Minutes:    max(minutes, 0),
		Activities: []string{fmt.Sprintf("Writing: %d/%d", msg.correction.Total(), teacher.MaxTotal)},
	})
	a.checkWritten(msg.correction.Total())

	w.majors = nil
	for _, f := range msg.correction.Majors() {
		w.majors = append(w.majors, majorFix{Fix: f})
	}
	if len(w.majors) == 0 {
		a.startFeedback()
		return a
	}
	w.phase, w.cursor, w.input = writeFixing, 0, ""
	return a
}

// saveFixCards stores the error cards. On failure the cards stay in the
// state, so `s` can try again.
func (a *App) saveFixCards() {
	w := &a.write
	if len(w.cards) == 0 || a.add == nil {
		w.cards = nil
		return
	}
	n, err := a.add(w.cards)
	if err != nil {
		w.err = err
		return
	}
	a.addCards(w.cards)
	w.added, w.cards, w.err = n, nil, nil
}

// recentWords are the newest words of the deck, newest first.
func (a App) recentWords() []string {
	known := a.knownWords()
	var recent []string
	for i := len(known) - 1; i >= 0 && len(recent) < recentWordCount; i-- {
		recent = append(recent, known[i])
	}
	return recent
}

// sentenceWith returns the sentence of text that contains chunk, ignoring
// case, or "" when none does. It becomes the example of the error's card.
func sentenceWith(text, chunk string) string {
	chunk = strings.ToLower(strings.TrimSpace(chunk))
	start := 0
	for i, r := range text {
		end := i + len(string(r))
		if r != '.' && r != '!' && r != '?' && r != '\n' && end != len(text) {
			continue
		}
		s := strings.TrimSpace(text[start:end])
		start = end
		if chunk != "" && strings.Contains(strings.ToLower(s), chunk) {
			return s
		}
	}
	return ""
}

// editInput applies a key to a one-line input: backspace deletes, any
// other key adds its text.
func editInput(s string, msg tea.KeyPressMsg) string {
	if msg.String() == "backspace" {
		return dropLastRune(s)
	}
	return s + msg.Text
}

func (a App) updateWrite(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	w := &a.write
	k := msg.String()
	switch w.phase {
	case writeIdle, writeTaskFail:
		if k == "enter" {
			return a.askTask()
		}
	case writeComposing:
		switch k {
		case "ctrl+d":
			return a.askCorrection()
		case "esc":
			a.screen = Home // the draft stays for later
		case "enter":
			w.text += "\n"
		default:
			w.text = editInput(w.text, msg)
		}
	case writeFixing:
		switch k {
		case "enter":
			return a.submitAttempt()
		case "esc":
			a.screen = Home // the attempts stay for later
		default:
			w.input = editInput(w.input, msg)
		}
	case writeFeedback:
		switch {
		case k == "enter" && len(w.rewrites) > 0:
			w.phase, w.cursor, w.input, w.checked = writeRewriting, 0, "", false
		case k == "enter":
			w.phase = writeResult
		case k == "d":
			w.minors = !w.minors
		case k == "s" && len(w.cards) > 0:
			a.saveFixCards()
		case k == "n":
			return a.askTask()
		}
	case writeRewriting:
		switch {
		case w.checked && k == "enter":
			w.nextRewrite()
		case w.checked:
			// Only enter goes on; the other keys are global.
		case k == "enter":
			w.checkRewrite()
		case k == "esc":
			a.screen = Home
		default:
			w.input = editInput(w.input, msg)
		}
	case writeResult:
		if k == "n" {
			return a.askTask()
		}
	}
	return a, nil
}
