package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/x/ansi"

	"github.com/kevnnard/english-teacher/internal/progress"
)

// levelRowFormat lays out one criterion or check part: the mark, the name,
// the current value and the target.
const levelRowFormat = "%-14s %-20s %s"

func (a App) levelView(w int) string {
	width := max(w, 20)
	switch a.check.stage {
	case checkOff:
		return a.readinessView(width)
	case checkReadingWait, checkReadingFail, checkReading, checkReadingDone:
		return a.checkReadingView(width)
	case checkDone:
		return a.checkResultView(width)
	}
	return a.checkStepView(width)
}

// mark is ✓ or ✗. The shape carries the meaning, not only the color.
func (a App) mark(ok bool) string {
	if ok {
		return a.st.main.success.Render("✓")
	}
	return a.st.main.danger.Render("✗")
}

// readinessView lists every criterion with its value, its target and, when
// it is not met, a tip; it offers the level check once all are met.
func (a App) readinessView(width int) string {
	r := a.st.main
	rd := a.readiness()
	var b strings.Builder
	head := fmt.Sprintf("Level %s (%s)", rd.Level, rd.Level.Name())
	if rd.Next == "" {
		b.WriteString(r.title.Render(head+" · the top level") + "\n\n")
		b.WriteString(r.secondary.Render(ansi.Wordwrap("There is no level above C2. Keep reading, writing and talking.", width, "")))
		return b.String()
	}
	b.WriteString(r.title.Render(fmt.Sprintf("%s → %s (%s) · %d of %d ready",
		head, rd.Next, rd.Next.Name(), rd.MetCount(), len(rd.Criteria))) + "\n\n")
	for _, c := range rd.Criteria {
		b.WriteString(a.mark(c.Met) + " " + r.value.Render(fmt.Sprintf(levelRowFormat, c.Name, c.Current, c.Target)) + "\n")
		if !c.Met {
			b.WriteString(r.label.Render(ansi.Truncate("    "+c.Tip, width, "…")) + "\n")
		}
	}
	b.WriteString("\n")
	switch {
	case !rd.Ready():
		b.WriteString(r.secondary.Render("The level check opens when every line has a ✓."))
	case a.teacher == nil:
		b.WriteString(r.success.Render("Ready for "+rd.Next.String()+".") + " " +
			r.secondary.Render("The level check needs the AI teacher: see `teacher models`."))
	default:
		b.WriteString(r.success.Render("Ready for "+rd.Next.String()+".") + " " +
			r.secondary.Render("A short reading, writing and grammar quiz at "+rd.Next.String()+".") + "\n")
		b.WriteString(r.accent.Render("enter") + "  " + r.target.Render("take the level check"))
	}
	return b.String()
}

// checkTitle is "Level check · A2 · reading".
func (a App) checkTitle(part string) string {
	return a.st.main.title.Render(fmt.Sprintf("Level check · %s · %s", a.check.next, part)) + "\n\n"
}

// checkReadingView is the check's reading: its text and one question at a
// time, with no feedback until the end, like a test.
func (a App) checkReadingView(width int) string {
	r := a.st.main
	c := a.check
	var b strings.Builder
	b.WriteString(a.checkTitle("reading"))
	switch c.stage {
	case checkReadingWait:
		b.WriteString(r.secondary.Render("Writing a reading for you… this takes about 10 seconds."))
		return b.String()
	case checkReadingFail:
		b.WriteString(r.danger.Render(ansi.Wordwrap("✗ Could not get the reading: "+c.err.Error(), width, "")) + "\n\n")
		b.WriteString(r.label.Render("Press enter to try again."))
		return b.String()
	case checkReadingDone:
		b.WriteString(r.value.Render(fmt.Sprintf("Reading: %d/%d", c.result.Reading.Right, c.result.Reading.Total)) + "\n\n")
		b.WriteString(r.accent.Render("enter") + "  " + r.target.Render("write a short text at "+c.next.String()))
		return b.String()
	}
	if c.reading.Title != "" {
		b.WriteString(r.target.Render(c.reading.Title) + "\n")
	}
	b.WriteString(r.value.Render(ansi.Wordwrap(c.reading.Text, width, "")) + "\n\n")
	i := len(c.picked)
	q := c.reading.Questions[i]
	b.WriteString(r.label.Render(fmt.Sprintf("Question %d of %d", i+1, len(c.reading.Questions))) + "\n")
	b.WriteString(r.target.Render(ansi.Wordwrap(q.Question, width, "")) + "\n")
	for n, o := range q.Options {
		b.WriteString("  " + r.accent.Render(fmt.Sprint(n+1)) + " " + r.value.Render(o) + "\n")
	}
	return strings.TrimSuffix(b.String(), "\n")
}

// checkStepView is the Level screen between the writing and the quiz, or
// while one of them runs on its own screen.
func (a App) checkStepView(width int) string {
	r := a.st.main
	c := a.check
	var b strings.Builder
	b.WriteString(a.checkTitle("in progress"))
	b.WriteString(r.value.Render(fmt.Sprintf("Reading: %d/%d", c.result.Reading.Right, c.result.Reading.Total)) + "\n")
	switch c.stage {
	case checkWriting:
		b.WriteString("\n" + r.secondary.Render(ansi.Wordwrap("The writing part is on the Write screen.", width, "")))
	case checkQuizReady:
		b.WriteString(r.value.Render(fmt.Sprintf("Writing: %d/%d", c.result.Writing.Right, c.result.Writing.Total)) + "\n\n")
		b.WriteString(r.accent.Render("enter") + "  " + r.target.Render("take the grammar quiz at "+c.next.String()))
	case checkQuiz:
		b.WriteString(r.value.Render(fmt.Sprintf("Writing: %d/%d", c.result.Writing.Right, c.result.Writing.Total)) + "\n\n")
		b.WriteString(r.secondary.Render(ansi.Wordwrap("The grammar quiz is on the Quiz screen.", width, "")))
	}
	return b.String()
}

// checkResultView says whether the check was passed and how each part went.
func (a App) checkResultView(width int) string {
	r := a.st.main
	c := a.check
	var b strings.Builder
	b.WriteString(a.checkTitle("result"))
	switch {
	case c.err != nil:
		b.WriteString(r.danger.Render(ansi.Wordwrap("✗ Could not save your new level: "+c.err.Error(), width, "")) + "\n")
		b.WriteString(r.secondary.Render("You passed, but you stay at "+c.from.String()+" until it saves. Try `teacher set level`.") + "\n\n")
	case c.passed:
		b.WriteString(r.success.Render(fmt.Sprintf("Passed! You are now %s (%s).", c.next, c.next.Name())) + "\n\n")
	default:
		b.WriteString(r.warning.Render("Not yet. You stay at "+c.from.String()+" for now.") + "\n\n")
	}
	failed := map[string]bool{}
	for _, f := range c.result.Failed() {
		failed[f] = true
	}
	parts := []struct {
		key, name string
		score     progress.Score
		needs     string
	}{
		{"reading", "Reading", c.result.Reading, fmt.Sprintf("needs %d/%d", progress.CheckReadingRight, progress.CheckReadingOf)},
		{"writing", "Writing", c.result.Writing, fmt.Sprintf("needs %d/%d", progress.CheckWritingScore, progress.WritingMax)},
		{"grammar quiz", "Grammar quiz", c.result.Quiz, fmt.Sprintf("needs %d%%", progress.CheckQuizPercent)},
	}
	for _, p := range parts {
		score := fmt.Sprintf("%d/%d", p.score.Right, p.score.Total)
		b.WriteString(a.mark(!failed[p.key]) + " " + r.value.Render(fmt.Sprintf(levelRowFormat, p.name, score, p.needs)) + "\n")
	}
	b.WriteString("\n" + r.accent.Render("enter") + "  " + r.label.Render("back to your level"))
	return b.String()
}
