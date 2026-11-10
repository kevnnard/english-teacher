package tui

import (
	"context"
	"fmt"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/kevnnard/english-teacher/internal/domain"
	"github.com/kevnnard/english-teacher/internal/teacher"
)

// maxWritingMinutes caps the study time one writing task can record.
const maxWritingMinutes = 30

// recentWordCount is how many of the newest words a writing task suggests.
const recentWordCount = 10

type writePhase int

const (
	writeIdle      writePhase = iota // nothing asked yet
	writeTaskWait                    // waiting for a task
	writeTaskFail                    // the teacher could not give a task
	writeComposing                   // the learner is writing
	writeChecking                    // waiting for the correction
	writeCorrected                   // the correction is on screen
)

// writeState is the Write screen: one task, the learner's text and its
// correction.
type writeState struct {
	phase      writePhase
	req        int // the id of the latest request; older answers are ignored
	task       teacher.WritingTask
	text       string
	startedAt  time.Time // when the task appeared, for the study minutes
	correction teacher.Correction
	cards      []domain.Card // the error cards, kept until they are saved
	added      int
	err        error // the last failed request or save, shown on the page
}

// taskMsg and correctionMsg are the teacher's answers to request req.
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

// askTask starts a request for a new writing task.
func (a App) askTask() (App, tea.Cmd) {
	if a.teacher == nil {
		return a, nil
	}
	a.write = writeState{phase: writeTaskWait, req: a.write.req + 1}
	req, tc, level, recent := a.write.req, a.teacher, a.profile.Level, a.recentWords()
	return a, func() tea.Msg {
		t, err := tc.WritingTask(context.Background(), level, recent, nil)
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

// askCorrection sends the learner's text to the teacher.
func (a App) askCorrection() (App, tea.Cmd) {
	text := strings.TrimSpace(a.write.text)
	if text == "" {
		return a, nil
	}
	a.write.req++
	a.write.phase, a.write.err = writeChecking, nil
	req, tc, level, task := a.write.req, a.teacher, a.profile.Level, a.write.task.Task
	return a, func() tea.Msg {
		c, err := tc.Correct(context.Background(), level, task, text)
		return correctionMsg{req: req, correction: c, err: err}
	}
}

// gotCorrection shows the correction, turns every error into a card and
// records the writing as study.
func (a App) gotCorrection(msg correctionMsg) App {
	w := &a.write
	if msg.req != w.req || w.phase != writeChecking {
		return a
	}
	if msg.err != nil {
		w.phase, w.err = writeComposing, msg.err
		return a
	}
	w.phase, w.correction = writeCorrected, msg.correction

	w.cards = nil
	for _, f := range msg.correction.Fixes {
		w.cards = append(w.cards, f.Card(sentenceWith(w.text, f.Wrong)))
	}
	a.saveFixCards()
	minutes := min(int(a.now().Sub(w.startedAt)/time.Minute), maxWritingMinutes)
	a.addToToday(domain.Day{
		Minutes:    max(minutes, 0),
		Activities: []string{fmt.Sprintf("Writing: %d/%d", msg.correction.Total(), teacher.MaxTotal)},
	})
	return a
}

// saveFixCards stores the error cards of the correction. On failure the
// cards stay in the state, so `s` can try again.
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

func (a App) updateWrite(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	w := &a.write
	k := msg.String()
	switch w.phase {
	case writeIdle, writeTaskFail:
		if k == "enter" {
			return a.askTask()
		}
	case writeCorrected:
		switch {
		case k == "n":
			return a.askTask()
		case k == "s" && len(w.cards) > 0:
			a.saveFixCards()
		}
	case writeComposing:
		switch k {
		case "ctrl+d":
			return a.askCorrection()
		case "esc":
			a.screen = Home // the draft stays for later
		case "enter":
			w.text += "\n"
		case "backspace":
			w.text = dropLastRune(w.text)
		default:
			w.text += msg.Text
		}
	}
	return a, nil
}

func (a App) writeView(w, h int) string {
	r := a.st.main
	s := a.write
	width := max(w, 20)
	var b strings.Builder
	b.WriteString(r.title.Render("Write") + "\n\n")
	switch {
	case a.teacher == nil:
		b.WriteString(r.secondary.Render("The AI teacher is off. Install Gentle Shell (the pi command) and see `teacher models`."))
		return b.String()
	case s.phase == writeIdle:
		b.WriteString(r.target.Render("Write a few sentences and the teacher corrects them.") + "\n")
		b.WriteString(r.secondary.Render("Every error comes back as a card, so you practice the right way to say it."))
		return b.String()
	case s.phase == writeTaskWait:
		b.WriteString(r.secondary.Render("Choosing a task for you… this takes about 10 seconds."))
		return b.String()
	case s.phase == writeTaskFail:
		b.WriteString(r.danger.Render("✗ Could not get a task: "+s.err.Error()) + "\n\n")
		b.WriteString(r.label.Render("Press enter to try again."))
		return b.String()
	}

	b.WriteString(r.target.Render(ansi.Wordwrap(s.task.Task, width, "")) + "\n")
	if s.task.Hint != "" {
		b.WriteString(r.label.Render(ansi.Wordwrap(s.task.Hint, width, "")) + "\n")
	}
	b.WriteString("\n")

	switch s.phase {
	case writeComposing:
		if s.err != nil {
			b.WriteString(r.danger.Render("✗ Could not correct it: "+s.err.Error()) + "\n")
			b.WriteString(r.label.Render("Your text is still here; press ctrl+d to try again.") + "\n\n")
		}
		b.WriteString(r.secondary.Render("Your text") + "\n")
		b.WriteString(indent(ansi.Wrap(s.text, width-2, "")+r.accent.Render("▏"), 2))
	case writeChecking:
		b.WriteString(indent(r.secondary.Render(ansi.Wrap(s.text, width-2, "")), 2) + "\n\n")
		b.WriteString(r.secondary.Render("Correcting your text… this takes about 10 seconds."))
	case writeCorrected:
		b.WriteString(a.correctionView(width))
	}
	return b.String()
}

func (a App) correctionView(width int) string {
	r := a.st.main
	s := a.write
	c := s.correction
	var b strings.Builder
	b.WriteString(r.value.Render(fmt.Sprintf("%d/%d", c.Total(), teacher.MaxTotal)))
	if c.Comment != "" {
		b.WriteString(r.secondary.Render("  " + c.Comment))
	}
	b.WriteString("\n\n" + r.label.Render("Corrected") + "\n")
	b.WriteString(indent(r.target.Render(ansi.Wordwrap(c.Corrected, width-2, "")), 2) + "\n\n")

	if len(c.Fixes) == 0 {
		b.WriteString(r.success.Render("✓ ") + r.secondary.Render("No errors. Nice work!"))
		return b.String()
	}
	b.WriteString(r.label.Render("Errors") + "\n")
	for _, f := range c.Fixes {
		b.WriteString("  " + r.danger.Render("✗ "+f.Wrong) + r.label.Render("  →  ") + r.success.Render("✓ "+f.Right) + "\n")
		if f.Why != "" {
			b.WriteString(indent(r.secondary.Render(ansi.Wordwrap(f.Why, width-4, "")), 4) + "\n")
		}
	}
	b.WriteString("\n")
	switch {
	case s.err != nil:
		b.WriteString(r.danger.Render("✗ Could not add the cards: "+s.err.Error()) + "\n")
		b.WriteString(r.label.Render("Press s to try again."))
	case s.added == 1:
		b.WriteString(r.success.Render("✓ ") + r.secondary.Render("Added 1 card to practice."))
	case s.added > 1:
		b.WriteString(r.success.Render("✓ ") + r.secondary.Render(fmt.Sprintf("Added %d cards to practice.", s.added)))
	default:
		b.WriteString(r.secondary.Render("These errors are already in your cards."))
	}
	return b.String()
}

func (a App) writeHints() []string {
	switch {
	case a.teacher == nil:
		return []string{"tab screens", "esc home", "? help"}
	}
	switch a.write.phase {
	case writeIdle:
		return []string{"enter get a task", "tab screens", "esc home", "? help"}
	case writeTaskFail:
		return []string{"enter try again", "esc home"}
	case writeComposing:
		return []string{"type your text", "enter new line", "ctrl+d send", "esc home"}
	case writeCorrected:
		if len(a.write.cards) > 0 {
			return []string{"s save cards", "n new task", "esc home"}
		}
		return []string{"n new task", "tab screens", "esc home"}
	}
	return []string{"tab screens", "esc home"}
}
