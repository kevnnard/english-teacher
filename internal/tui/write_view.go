package tui

import (
	"fmt"
	"sort"
	"strings"

	"github.com/charmbracelet/x/ansi"

	"github.com/kevnnard/english-teacher/internal/check"
	"github.com/kevnnard/english-teacher/internal/teacher"
)

func (a App) writeView(w, h int) string {
	r := a.st.main
	s := a.write
	width := max(w, 20)
	var b strings.Builder
	title := "Write"
	if a.check.stage == checkWriting {
		title = "Write · level check " + a.check.next.String()
	}
	b.WriteString(r.title.Render(title) + "\n\n")
	switch {
	case a.teacher == nil:
		b.WriteString(r.secondary.Render("The AI teacher is off. Install Gentle Shell (the pi command) and see `teacher models`."))
		return b.String()
	case s.phase == writeIdle:
		b.WriteString(r.target.Render("Write a few sentences and the teacher corrects them.") + "\n")
		b.WriteString(r.secondary.Render("You fix the main errors yourself first; the ones you can't fix become cards."))
		return b.String()
	case s.phase == writeTaskWait:
		b.WriteString(r.secondary.Render("Choosing a task for you… this takes about 10 seconds."))
		return b.String()
	case s.phase == writeTaskFail:
		b.WriteString(r.danger.Render("✗ Could not get a task: "+s.err.Error()) + "\n\n")
		b.WriteString(r.label.Render("Press enter to try again."))
		return b.String()
	}

	switch s.phase {
	case writeComposing, writeChecking:
		b.WriteString(a.planView(width))
	case writeFixing, writeJudging:
		b.WriteString(a.fixView(width))
	case writeFeedback:
		b.WriteString(a.feedbackView(width))
	case writeRewriting:
		b.WriteString(a.rewriteView(width))
	case writeResult:
		b.WriteString(a.resultView(width))
	}
	return b.String()
}

// step is the "Step 3/6 · Fix it yourself" line at the top of each step.
func (a App) step(n int, name string) string {
	return a.st.main.label.Render(fmt.Sprintf("Step %d/6 · %s", n, name)) + "\n"
}

// planView is steps 1 and 2: the task, its useful phrases and the text.
func (a App) planView(width int) string {
	r := a.st.main
	s := a.write
	var b strings.Builder
	b.WriteString(r.target.Render(ansi.Wordwrap(s.task.Task, width, "")) + "\n")
	if s.task.Hint != "" {
		b.WriteString(r.label.Render(ansi.Wordwrap(s.task.Hint, width, "")) + "\n")
	}
	if len(s.task.Phrases) > 0 {
		b.WriteString("\n" + r.secondary.Render("Useful phrases") + "\n")
		for _, p := range s.task.Phrases {
			b.WriteString("  " + r.accent.Render("• "+p) + "\n")
		}
	}
	b.WriteString("\n")

	if s.phase == writeChecking {
		b.WriteString(indent(r.secondary.Render(ansi.Wrap(s.text, width-2, "")), 2) + "\n\n")
		b.WriteString(r.secondary.Render("Correcting your text… this takes about 10 seconds."))
		return b.String()
	}
	if s.err != nil {
		b.WriteString(r.danger.Render("✗ Could not correct it: "+s.err.Error()) + "\n")
		b.WriteString(r.label.Render("Your text is still here; press ctrl+d to try again.") + "\n\n")
	}
	b.WriteString(r.secondary.Render("Your text") + "\n")
	b.WriteString(indent(ansi.Wrap(s.text, width-2, "")+r.accent.Render("▏"), 2))
	return b.String()
}

// fixView is step 3: the text with the major errors marked, without the
// answers, and one try to fix each.
func (a App) fixView(width int) string {
	r := a.st.main
	s := a.write
	var b strings.Builder
	b.WriteString(a.step(3, "Fix it yourself"))
	b.WriteString(r.target.Render("Fix the marked errors yourself.") + "\n")
	b.WriteString(r.label.Render("One try each; press enter with nothing typed if you don't know.") + "\n\n")
	b.WriteString(indent(ansi.Wrap(a.markedText(), width-2, ""), 2) + "\n\n")

	for i, m := range s.majors {
		line := fmt.Sprintf("%d. «%s»", i+1, m.Wrong)
		hint := r.label.Render("  " + teacher.KindLabel(m.Kind))
		switch {
		case s.phase == writeFixing && i == s.cursor:
			b.WriteString(r.accent.Render("▸ "+line) + hint + "\n")
			b.WriteString("     " + r.secondary.Render("your fix: ") + s.input + r.accent.Render("▏") + "\n")
		case i < s.cursor || s.phase == writeJudging:
			tried := m.attempt
			if tried == "" {
				tried = "(I don't know)"
			}
			b.WriteString("  " + r.secondary.Render(line) + hint + "\n")
			b.WriteString("     " + r.label.Render("→ "+tried) + "\n")
		default:
			b.WriteString("  " + r.secondary.Render(line) + hint + "\n")
		}
	}
	if s.phase == writeJudging {
		b.WriteString("\n" + r.secondary.Render("Checking your fixes… this takes a few seconds."))
	}
	return strings.TrimSuffix(b.String(), "\n")
}

// markedText is the learner's text with every major error underlined and
// between « », so it shows even without colors.
func (a App) markedText() string {
	r := a.st.main
	text := a.write.text
	lower := strings.ToLower(text)
	if len(lower) != len(text) { // a rare letter changed size; match the case exactly
		lower = text
	}
	type span struct{ start, end int }
	var spans []span
	for _, m := range a.write.majors {
		chunk := m.Wrong
		if lower != text {
			chunk = strings.ToLower(chunk)
		}
		i := strings.Index(lower, chunk)
		if i < 0 || chunk == "" {
			continue
		}
		overlaps := false
		for _, sp := range spans {
			if i < sp.end && sp.start < i+len(chunk) {
				overlaps = true
			}
		}
		if !overlaps {
			spans = append(spans, span{i, i + len(chunk)})
		}
	}
	sort.Slice(spans, func(i, j int) bool { return spans[i].start < spans[j].start })

	mark := r.accent.Underline(true)
	var b strings.Builder
	at := 0
	for _, sp := range spans {
		b.WriteString(r.secondary.Render(text[at:sp.start]))
		b.WriteString(mark.Render("«" + text[sp.start:sp.end] + "»"))
		at = sp.end
	}
	b.WriteString(r.secondary.Render(text[at:]))
	return b.String()
}

// feedbackView is step 4: each major error fixed or not, with its why,
// the minor details collapsed and the cards added.
func (a App) feedbackView(width int) string {
	r := a.st.main
	s := a.write
	var b strings.Builder
	b.WriteString(a.step(4, "Feedback"))
	if s.judgeErr != nil {
		b.WriteString(r.danger.Render(ansi.Wordwrap("✗ Could not check your fixes: "+s.judgeErr.Error(), width, "")) + "\n")
		b.WriteString(r.label.Render("Fixes that were not exactly the teacher's count as not fixed.") + "\n")
	}
	b.WriteString("\n")
	switch {
	case len(s.majors) == 0 && len(s.correction.Fixes) == 0:
		b.WriteString(r.success.Render("✓ ") + r.secondary.Render("No errors. Nice work!"))
		return b.String()
	case len(s.majors) == 0:
		b.WriteString(r.success.Render("✓ ") + r.secondary.Render("No major errors, only small details.") + "\n")
	}

	for _, m := range s.majors {
		if m.fixed {
			b.WriteString("  " + r.success.Render("✓ fixed by you") + "  " + r.secondary.Render(m.Wrong+" → "+m.attempt) + "\n")
		} else {
			b.WriteString("  " + r.danger.Render("✗ "+m.Wrong) + r.label.Render(" → ") + r.success.Render(m.Right) + "\n")
			if m.attempt != "" {
				b.WriteString(r.label.Render("    you tried: "+m.attempt) + "\n")
			}
		}
		if m.Why != "" {
			b.WriteString(indent(r.secondary.Render(ansi.Wordwrap(m.Why, width-4, "")), 4) + "\n")
		}
	}

	if minors := s.correction.Minors(); len(minors) > 0 {
		b.WriteString("\n")
		if !s.minors {
			b.WriteString(r.label.Render(fmt.Sprintf("+%d minor details (d shows them)", len(minors))) + "\n")
		} else {
			b.WriteString(r.label.Render("Minor details (d hides them)") + "\n")
			for _, f := range minors {
				b.WriteString("  " + r.secondary.Render(f.Wrong+" → "+f.Right) + "\n")
				if f.Why != "" {
					b.WriteString(indent(r.label.Render(ansi.Wordwrap(f.Why, width-4, "")), 4) + "\n")
				}
			}
		}
	}

	b.WriteString("\n" + a.cardsLine())
	if s.logErr != nil {
		b.WriteString("\n" + r.danger.Render("✗ Could not save the error log: "+s.logErr.Error()))
	}
	return b.String()
}

// cardsLine says which cards the errors became.
func (a App) cardsLine() string {
	r := a.st.main
	s := a.write
	switch {
	case s.err != nil:
		return r.danger.Render("✗ Could not add the cards: "+s.err.Error()) + "\n" + r.label.Render("Press s to try again.")
	case s.added == 1:
		return r.success.Render("✓ ") + r.secondary.Render("Added 1 card to practice.")
	case s.added > 1:
		return r.success.Render("✓ ") + r.secondary.Render(fmt.Sprintf("Added %d cards to practice.", s.added))
	case len(s.rewrites) == 0:
		return r.success.Render("✓ ") + r.secondary.Render("You fixed every error yourself, so no new cards.")
	}
	return r.secondary.Render("These errors are already in your cards.")
}

// rewriteView is step 5: the Spanish meaning of an unfixed error, and the
// learner writes the English from memory.
func (a App) rewriteView(width int) string {
	r := a.st.main
	s := a.write
	m := s.majors[s.rewrites[s.cursor]]
	var b strings.Builder
	b.WriteString(a.step(5, fmt.Sprintf("Rewrite from memory %d/%d", s.cursor+1, len(s.rewrites))) + "\n")
	if m.Meaning != "" {
		b.WriteString(r.label.Render("Say it in English:") + "\n")
		b.WriteString("  " + r.prompt.Render(" "+ansi.Wordwrap(m.Meaning, width-4, "")+" ") + "\n\n")
	} else {
		b.WriteString(r.label.Render("Write it in English:") + "\n")
		b.WriteString("  " + r.prompt.Render(" the correct version of «"+m.Wrong+"» ") + "\n\n")
	}
	if !s.checked {
		b.WriteString("  " + r.secondary.Render("› ") + s.input + r.accent.Render("▏"))
		return b.String()
	}
	b.WriteString("  " + r.secondary.Render("› "+s.input) + "\n\n")
	switch m.rewrite.Kind {
	case check.Exact:
		b.WriteString("  " + r.success.Render("✓ "+m.Right))
	case check.Almost:
		b.WriteString("  " + r.warning.Render("✗ almost  ") + a.diffView(m.rewrite.Diff) +
			r.label.Render("  → ") + r.success.Render(m.Right))
	default:
		b.WriteString("  " + r.danger.Render("✗ ") + r.label.Render("right answer: ") + r.success.Render(m.Right))
	}
	return b.String()
}

// resultView is step 6: the rubric, the total and grade, the cards and the
// pattern line.
func (a App) resultView(width int) string {
	r := a.st.main
	s := a.write
	c := s.correction
	rb := c.Rubric
	var b strings.Builder
	b.WriteString(a.step(6, "Result") + "\n")
	b.WriteString(r.value.Render(fmt.Sprintf("Grammar %d/%d · Vocabulary %d/%d · Coherence %d/%d · Task %d/%d",
		rb.Grammar, teacher.RubricMax, rb.Vocabulary, teacher.RubricMax, rb.Coherence, teacher.RubricMax,
		rb.Task, teacher.RubricMax)) + "\n")
	b.WriteString(r.title.Render(fmt.Sprintf("%d/%d %s", c.Total(), teacher.MaxTotal, c.Grade())) + "\n")
	if c.Comment != "" {
		b.WriteString(r.secondary.Render(ansi.Wordwrap(c.Comment, width, "")) + "\n")
	}
	b.WriteString("\n" + r.label.Render("Corrected") + "\n")
	b.WriteString(indent(r.target.Render(ansi.Wordwrap(c.Corrected, width-2, "")), 2) + "\n\n")

	if len(s.rewrites) > 0 {
		right := 0
		for _, i := range s.rewrites {
			if s.majors[i].rewrite.Kind == check.Exact {
				right++
			}
		}
		b.WriteString(r.secondary.Render(fmt.Sprintf("Rewrite: %d/%d right from memory.", right, len(s.rewrites))) + "\n")
	}
	if len(s.majors) > 0 {
		b.WriteString(a.cardsLine() + "\n")
	}
	if s.patternKind != "" {
		b.WriteString(r.warning.Render(fmt.Sprintf("Pattern: %s time with %s", ordinal(s.patternCount),
			teacher.KindLabel(s.patternKind))) + "\n")
	}
	return strings.TrimSuffix(b.String(), "\n")
}

func (a App) writeHints() []string {
	if a.teacher == nil {
		return []string{"tab screens", "esc home", "? help"}
	}
	s := a.write
	switch s.phase {
	case writeIdle:
		return []string{"enter get a task", "tab screens", "esc home", "? help"}
	case writeTaskFail:
		return []string{"enter try again", "esc home"}
	case writeComposing:
		return []string{"type your text", "enter new line", "ctrl+d send", "esc home"}
	case writeFixing:
		return []string{"type your fix", "enter next (empty: I don't know)", "esc home"}
	case writeFeedback:
		hints := []string{"enter result"}
		if len(s.rewrites) > 0 {
			hints = []string{"enter rewrite"}
		}
		if len(s.cards) > 0 {
			hints = append(hints, "s save cards")
		}
		if len(s.correction.Minors()) > 0 {
			hints = append(hints, "d minor details")
		}
		return append(hints, "n new task", "esc home")
	case writeRewriting:
		if s.checked {
			return []string{"enter next", "esc home"}
		}
		return []string{"type the English", "enter check", "esc home"}
	case writeResult:
		return []string{"n new task", "tab screens", "esc home"}
	}
	return []string{"tab screens", "esc home"}
}
