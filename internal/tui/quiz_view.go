package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/x/ansi"

	"github.com/kevnnard/english-teacher/internal/check"
	"github.com/kevnnard/english-teacher/internal/teacher"
)

func (a App) quizView(w int) string {
	width := max(w, 20)
	switch a.quiz.phase {
	case quizLoading, quizFailed:
		return a.quizLoadingView(width)
	case quizAsking, quizAnswered:
		return a.quizQuestionView(width)
	case quizResult:
		return a.quizResultView(width)
	}
	return a.quizPickerView(width)
}

// quizTitle is the title of a quiz page, like "Quiz · Grammar".
func (a App) quizTitle() string {
	if a.quiz.mode == quizGrammar && a.check.stage == checkQuiz {
		return "Quiz · level check " + a.check.next.String()
	}
	if a.quiz.mode == quizGrammar {
		return "Quiz · Grammar"
	}
	if a.quiz.mode == quizCloze {
		return "Quiz · Grammar in context"
	}
	return "Quiz · Vocabulary"
}

// quizPickerView offers the two quizzes, with what the grammar one practices.
func (a App) quizPickerView(width int) string {
	r := a.st.main
	q := a.quiz
	var b strings.Builder
	b.WriteString(r.title.Render("Quiz") + "\n\n")
	b.WriteString(r.target.Render(ansi.Wordwrap("Check what you remember. A quiz never changes your review dates.", width, "")) + "\n")
	b.WriteString(r.secondary.Render("Pick a quiz:") + "\n\n")
	modes := []string{
		fmt.Sprintf("Vocabulary · %d questions from your cards", vocabQuestions),
		fmt.Sprintf("Grammar · %d questions from the teacher", teacher.GrammarItems),
		fmt.Sprintf("Grammar in context · %d gaps in your sentences", teacher.ClozeItems),
	}
	for i, m := range modes {
		if quizMode(i) == q.cursor {
			b.WriteString(r.accent.Render("▸ "+m) + "\n")
		} else {
			b.WriteString("  " + r.value.Render(m) + "\n")
		}
	}
	about := "Choose the word or type it, from its Spanish meaning."
	if q.cursor == quizGrammar {
		about = "Practices: " + strings.Join(teacher.GrammarTopics(a.studyLevel(), a.focus()), ", ")
	}
	if q.cursor == quizCloze {
		about = "Type the missing words, mixing: " + strings.Join(teacher.ClozePoints(a.studyLevel(), a.recurringKinds()), ", ")
	}
	b.WriteString("\n" + r.label.Render(ansi.Wordwrap(about, width, "")))
	if q.notice != "" {
		b.WriteString("\n\n" + r.danger.Render(ansi.Wordwrap(q.notice, width, "")))
	}
	return b.String()
}

// quizLoadingView waits for the grammar items, or shows why they failed.
func (a App) quizLoadingView(width int) string {
	r := a.st.main
	q := a.quiz
	s := r.title.Render(a.quizTitle()) + "\n\n" +
		r.secondary.Render(ansi.Wordwrap("Practices: "+strings.Join(q.topics, ", "), width, "")) + "\n\n"
	if q.phase == quizFailed {
		return s + r.danger.Render(ansi.Wordwrap("✗ Could not get the quiz: "+q.err.Error(), width, "")) + "\n" +
			r.label.Render("Press enter to try again.")
	}
	return s + r.secondary.Render("Writing your questions… this takes about 10 seconds.")
}

// quizQuestionView is one question and, once answered, its feedback.
func (a App) quizQuestionView(width int) string {
	r := a.st.main
	q := a.quiz
	cur := q.questions[q.pos]
	var b strings.Builder
	b.WriteString(r.title.Render(a.quizTitle()) + "\n")
	progress := fmt.Sprintf("Question %d of %d · ✓ %d", q.pos+1, len(q.questions), q.score())
	if q.mode == quizCloze { // no points here: each one shows after its answer
		progress = fmt.Sprintf("Gap %d of %d · ✓ %d", q.pos+1, len(q.questions), q.score())
	}
	if q.mode == quizGrammar && len(q.topics) > 0 {
		progress += " · " + strings.Join(q.topics, ", ")
	}
	b.WriteString(r.label.Render(ansi.Truncate(progress, width, "…")) + "\n\n")

	var ans *quizAnswer
	if q.phase == quizAnswered {
		ans = &q.answers[q.pos]
	}
	switch {
	case q.mode == quizCloze:
		b.WriteString(a.clozePromptView(cur, width))
	case q.mode == quizGrammar:
		b.WriteString(r.target.Render(ansi.Wordwrap(cur.prompt, width, "")) + "\n\n")
	case cur.typed():
		b.WriteString(r.secondary.Render("Type the English for:") + "\n")
		b.WriteString("  " + r.prompt.Render(cur.prompt) + "\n")
		if cur.example != "" {
			b.WriteString("  " + r.label.Render(ansi.Wordwrap(`"`+cur.example+`"`, width-2, "")) + "\n")
		}
		b.WriteString("\n")
	default:
		b.WriteString(r.secondary.Render("Which English word means:") + "\n")
		b.WriteString("  " + r.prompt.Render(cur.prompt) + "\n\n")
	}

	if cur.typed() {
		b.WriteString(a.typedAnswerView(cur, ans))
	} else {
		b.WriteString(a.optionsView(cur, ans))
	}
	if ans == nil {
		return b.String()
	}
	if q.mode == quizCloze {
		b.WriteString(a.clozeFeedbackView(cur, *ans, width))
	} else if cur.why != "" {
		b.WriteString("\n" + indent(r.secondary.Render(ansi.Wordwrap(cur.why, width-2, "")), 2))
	}
	next := "Press enter for the next question."
	if q.pos == len(q.questions)-1 {
		next = "Press enter to see your result."
	}
	return b.String() + "\n\n" + r.label.Render(next)
}

// optionsView lists the options with their keys; after the answer it
// marks the right option and a wrong choice.
func (a App) optionsView(cur quizQuestion, ans *quizAnswer) string {
	r := a.st.main
	var b strings.Builder
	for i, o := range cur.options {
		line := fmt.Sprintf("  %s  %s", r.accent.Render(fmt.Sprint(i+1)), r.value.Render(o))
		if ans != nil {
			switch {
			case i == cur.answer:
				line = fmt.Sprintf("  %s  %s", r.success.Render(fmt.Sprint(i+1)), r.success.Render(o+" ✓"))
			case i == ans.picked:
				line = fmt.Sprintf("  %s  %s", r.danger.Render(fmt.Sprint(i+1)), r.danger.Render(o+" ✗"))
			}
		}
		b.WriteString(line + "\n")
	}
	if ans == nil {
		return strings.TrimSuffix(b.String(), "\n")
	}
	if ans.right {
		return b.String() + "\n" + r.success.Render("✓ correct")
	}
	return b.String() + "\n" + r.danger.Render("✗ not quite") + r.label.Render(" · the answer is ") + r.target.Render(cur.word)
}

// typedAnswerView is the answer input, then the check of the answer: ✓,
// almost with the letter diff, or wrong with the right answer.
func (a App) typedAnswerView(cur quizQuestion, ans *quizAnswer) string {
	r := a.st.main
	if ans == nil {
		return r.secondary.Render("Your answer") + "\n  " + r.accent.Render("› ") + a.quiz.input + r.accent.Render("▏")
	}
	s := r.secondary.Render("Your answer") + "\n  " + r.accent.Render("› ") + ans.typed + "\n  "
	switch ans.result.Kind {
	case check.Exact:
		return s + r.success.Render("✓ correct")
	case check.Almost:
		n := ans.result.Distance
		return s + a.diffView(ans.result.Diff) + "   " +
			r.warning.Render(fmt.Sprintf("✗ almost · %d %s", n, plural(n, "letter", "letters"))) +
			r.label.Render(" · the answer is ") + r.target.Render(cur.word)
	}
	return s + r.danger.Render("✗ not quite") + r.label.Render(" · the answer is ") + r.target.Render(cur.word)
}

// quizResultView is the score and the questions to look at again.
func (a App) quizResultView(width int) string {
	r := a.st.main
	q := a.quiz
	var b strings.Builder
	b.WriteString(r.title.Render(a.quizTitle()) + "\n\n")
	score := r.accent.Render(fmt.Sprintf("%d/%d", q.score(), len(q.questions)))
	if q.score() == len(q.questions) {
		score = r.success.Render(fmt.Sprintf("✓ %d/%d", q.score(), len(q.questions)))
	}
	// One line for the score, so eight wrong grammar items still fit on 24 rows.
	b.WriteString(score + r.label.Render("  ·  saved in today's log; your review dates did not change") + "\n")
	if len(q.missed) > 0 {
		b.WriteString(r.success.Render(ansi.Truncate("Added to Review: "+strings.Join(q.missed, ", "), width, "…")) + "\n")
	}
	if q.missedErr != nil {
		b.WriteString(r.danger.Render(ansi.Truncate("✗ Could not add the chunks: "+q.missedErr.Error(), width, "…")) + "\n")
	}

	var wrong []string
	for i, ans := range q.answers {
		if !ans.right {
			wrong = append(wrong, a.wrongLine(q.questions[i], ans, width)...)
		}
	}
	if len(wrong) > 0 {
		if len(q.missed) == 0 && q.missedErr == nil { // with that line, eight wrong items still fit on 24 rows
			b.WriteString("\n")
		}
		b.WriteString(r.secondary.Render("To review") + "\n" + strings.Join(wrong, "\n"))
	}
	return strings.TrimSuffix(b.String(), "\n")
}

// wrongLine is one wrongly answered question in the result: the prompt,
// the right answer and what the learner gave, then the why of a grammar item.
func (a App) wrongLine(cur quizQuestion, ans quizAnswer, width int) []string {
	r := a.st.main
	gave := "you wrote nothing"
	switch {
	case !cur.typed():
		gave = "you chose " + cur.options[ans.picked]
	case ans.typed != "":
		gave = "you wrote " + ans.typed
	}
	line := "  " + r.danger.Render("✗ ") + r.value.Render(cur.prompt) + r.label.Render(" → ") +
		r.success.Render(cur.word) + r.label.Render(" · "+gave)
	lines := []string{ansi.Truncate(line, width, "…")}
	why := cur.why
	if cur.point != "" {
		why = cur.point + " · " + why
	}
	if why != "" {
		// One line each: the full why was on the feedback page.
		lines = append(lines, "    "+r.secondary.Render(ansi.Truncate(why, max(width-4, 1), "…")))
	}
	return lines
}

func (a App) quizHints() []string {
	q := a.quiz
	switch q.phase {
	case quizPick:
		return []string{"↑↓ choose", "enter start", "tab screens", "esc home"}
	case quizLoading:
		return []string{"tab screens", "esc home"}
	case quizFailed:
		return []string{"enter try again", "n other quiz", "esc home"}
	case quizAsking:
		if q.mode == quizCloze {
			return []string{"type the missing words", "enter check", "esc home"}
		}
		if q.questions[q.pos].typed() {
			return []string{"type the word", "enter check", "esc home"}
		}
		return []string{"1–4 choose", "tab screens", "esc home"}
	case quizAnswered:
		next := "enter next"
		if q.pos == len(q.questions)-1 {
			next = "enter result"
		}
		if q.questions[q.pos].chunk.Valid() {
			return []string{next, "a add the chunk", "esc home"}
		}
		return []string{next, "tab screens", "esc home"}
	}
	return []string{"n new quiz", "tab screens", "esc home"}
}

// clozePromptView is a cloze item before its answer: the sentence with its
// gap, the hint in brackets and whether the sentence is the learner's own.
// The grammar point is not shown yet, so the learner works out which rule
// applies.
func (a App) clozePromptView(cur quizQuestion, width int) string {
	r := a.st.main
	line := cur.prompt
	if cur.hint != "" {
		line += "  (" + cur.hint + ")"
	}
	s := r.secondary.Render("Type the missing words:") + "\n" +
		indent(r.prompt.Render(ansi.Wordwrap(line, width-2, "")), 2) + "\n"
	if cur.yours {
		s += "  " + r.label.Render("from your own sentences") + "\n"
	}
	return s + "\n"
}

// clozeFeedbackView follows the check of a cloze answer: the grammar
// point, with its rule in one line when the answer was wrong, then the
// chunk of the item, if any, and what a did with it.
func (a App) clozeFeedbackView(cur quizQuestion, ans quizAnswer, width int) string {
	r := a.st.main
	point := cur.point
	if !ans.right && cur.why != "" {
		point += " · " + cur.why
	}
	s := "\n  " + r.accent.Render(ansi.Truncate(point, max(width-2, 1), "…"))
	if c := cur.chunk; c.Valid() {
		s += "\n  " + r.value.Render(ansi.Truncate("Chunk: "+c.Phrase+" = "+c.Meaning, max(width-2, 1), "…"))
		if note := a.quiz.chunkNote; note != "" {
			style := r.success
			if !strings.HasPrefix(note, "✓") {
				style = r.label
			}
			s += "\n  " + style.Render(ansi.Truncate(note, max(width-2, 1), "…"))
		}
	}
	return s
}
