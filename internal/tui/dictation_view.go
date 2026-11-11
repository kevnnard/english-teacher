package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/x/ansi"

	"github.com/kevnnard/english-teacher/internal/check"
)

func (a App) dictationView(w int) string {
	width := max(w, 20)
	switch a.dictation.phase {
	case dictationAsking, dictationAnswered:
		return a.dictationItemView(width)
	case dictationResult:
		return a.dictationResultView(width)
	}
	return a.dictationIntroView(width)
}

// dictationIntroView says what dictation is, or why it cannot start: no
// audio, or too few short example sentences.
func (a App) dictationIntroView(width int) string {
	r := a.st.main
	var b strings.Builder
	b.WriteString(r.title.Render("Listen · Dictation") + "\n\n")
	b.WriteString(r.target.Render(ansi.Wordwrap("Hear a sentence and type it. Dictation never changes your review dates.", width, "")) + "\n\n")
	n := len(a.dictationOptions())
	switch {
	case a.speaker == nil:
		b.WriteString(r.warning.Render(audioOffNotice) + "\n")
		b.WriteString(r.label.Render(ansi.Wordwrap("Dictation needs a voice: run `teacher setup voice`, then `teacher doctor` to check that piper and a player are found.", width, "")))
	case n < minDictationItems:
		b.WriteString(r.secondary.Render("Not enough sentences yet.") + "\n")
		b.WriteString(r.label.Render(ansi.Wordwrap(fmt.Sprintf(
			"Dictation plays the example sentences of your cards, up to %d words long, and needs at least %d.",
			maxDictationWords, minDictationItems), width, "")) + "\n\n")
		b.WriteString(r.value.Render(ansi.Wordwrap("Add words with an example sentence (in Learn, or with `teacher import`),", width, "")) + "\n")
		b.WriteString(r.value.Render(ansi.Wordwrap("then study them in Review.", width, "")))
	default:
		b.WriteString(r.value.Render(fmt.Sprintf("%d sentences from your cards", n)) + "\n")
		b.WriteString(r.label.Render(ansi.Wordwrap("Each one plays when it appears. ctrl+p plays it again, ctrl+s slower. Press enter to start.", width, "")))
	}
	return b.String()
}

// dictationItemView is one sentence: the input while the learner types,
// then the check and the sentence.
func (a App) dictationItemView(width int) string {
	r := a.st.main
	d := a.dictation
	var b strings.Builder
	b.WriteString(r.title.Render("Listen · Dictation") + "\n")
	b.WriteString(r.label.Render(fmt.Sprintf("Sentence %d of %d · ✓ %d", d.pos+1, len(d.items), d.score())) + "\n\n")
	b.WriteString(r.secondary.Render("Type what you hear") + "\n")
	if d.phase == dictationAsking {
		b.WriteString("  " + r.accent.Render("› ") + d.input + r.accent.Render("▏"))
		return b.String()
	}
	ans := d.answers[d.pos]
	sentence := d.items[d.pos]
	b.WriteString("  " + r.accent.Render("› ") + ans.typed + "\n\n")
	switch {
	case ans.result.Kind == check.Exact:
		b.WriteString("  " + r.success.Render("✓ correct") + r.label.Render(" · ") + r.target.Render(sentence))
	case ans.right:
		n := ans.result.Distance
		b.WriteString("  " + a.diffView(ans.result.Diff) + "\n  " +
			r.success.Render(fmt.Sprintf("✓ almost · %d %s", n, plural(n, "letter", "letters"))) +
			r.label.Render(" · ") + r.target.Render(sentence))
	default:
		verdict := "✗ not quite"
		if strings.TrimSpace(ans.typed) == "" {
			verdict = "✗ you wrote nothing"
		}
		b.WriteString("  " + r.danger.Render(verdict) + r.label.Render(" · the sentence is") + "\n")
		b.WriteString(indent(r.target.Render(ansi.Wordwrap(sentence, width-4, "")), 4))
		if len(ans.missed) > 0 && strings.TrimSpace(ans.typed) != "" {
			b.WriteString("\n  " + r.label.Render("Missed: ") + r.danger.Render(strings.Join(ans.missed, ", ")))
		}
	}
	next := "Press enter for the next sentence."
	if d.pos == len(d.items)-1 {
		next = "Press enter to see your result."
	}
	return b.String() + "\n\n" + r.label.Render(next)
}

// dictationResultView is the score and the sentences to hear again.
func (a App) dictationResultView(width int) string {
	r := a.st.main
	d := a.dictation
	var b strings.Builder
	b.WriteString(r.title.Render("Listen · Dictation") + "\n\n")
	score := r.accent.Render(fmt.Sprintf("%d/%d", d.score(), len(d.items)))
	if d.score() == len(d.items) {
		score = r.success.Render(fmt.Sprintf("✓ %d/%d", d.score(), len(d.items)))
	}
	b.WriteString(score + r.label.Render("  ·  saved in today's log; your review dates did not change") + "\n")

	var missed []string
	for i, ans := range d.answers {
		if !ans.right {
			missed = append(missed, "  "+r.danger.Render("✗ ")+r.value.Render(ansi.Truncate(d.items[i], max(width-4, 1), "…")))
		}
	}
	if len(missed) > 0 {
		b.WriteString("\n" + r.secondary.Render("To practice") + "\n" + strings.Join(missed, "\n"))
	}
	return strings.TrimSuffix(b.String(), "\n")
}

// dictationHints are the keys of the Listen screen. While a sentence
// waits for its answer the input has every letter, so the audio keys are
// ctrl keys.
func (a App) dictationHints() []string {
	d := a.dictation
	switch d.phase {
	case dictationAsking:
		return []string{"type what you hear", "ctrl+p again", "ctrl+s slower", "enter check", "esc home"}
	case dictationAnswered:
		next := "enter next"
		if d.pos == len(d.items)-1 {
			next = "enter result"
		}
		return []string{next, "ctrl+p again", "ctrl+s slower", "tab screens", "esc home"}
	case dictationResult:
		return []string{"n new dictation", "tab screens", "esc home"}
	}
	if a.canDictate() {
		return []string{"enter start", "tab screens", "esc home"}
	}
	return []string{"tab screens", "esc home", "? help"}
}
