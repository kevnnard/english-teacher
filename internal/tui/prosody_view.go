package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/x/ansi"

	"github.com/kevnnard/english-teacher/internal/domain"
)

// prosodyTitle heads every page of a word-stress or intonation round.
func (a App) prosodyTitle() string {
	if a.pairs.kind == pairsTone {
		return "Pairs · Intonation"
	}
	return "Pairs · Word stress"
}

// prosodyNoun is what one item of the kind on screen is called.
func (a App) prosodyNoun() string {
	if a.pairs.kind == pairsTone {
		return "Sentence"
	}
	return "Word"
}

func (a App) prosodyView(width int) string {
	switch a.prosody.phase {
	case pairsAsking, pairsAnswered:
		return a.prosodyItemView(width)
	case pairsResult:
		return a.prosodyResultView(width)
	}
	return a.prosodyIntroView(width)
}

// prosodyIntroView says what a round is and which drills come first, or
// how to turn audio on.
func (a App) prosodyIntroView(width int) string {
	r := a.st.main
	var b strings.Builder
	b.WriteString(r.title.Render(a.prosodyTitle()) + "\n\n")
	what := "Hear a word and choose its stressed syllable: PHOtograph, phoTOGraphy. " +
		"English writes no accent, and the stress moves inside a word family."
	count := fmt.Sprintf("%d words", stressItems)
	if a.pairs.kind == pairsTone {
		what = "Hear a sentence and choose: a question, where the voice rises at the end ↗, " +
			"or a statement, where it falls ↘. The words are the same; only the voice tells."
		count = fmt.Sprintf("%d sentences", toneItems)
	}
	b.WriteString(r.target.Render(ansi.Wordwrap(what, width, "")) + "\n\n")
	if a.speaker == nil {
		b.WriteString(r.warning.Render(audioOffNotice) + "\n")
		b.WriteString(r.label.Render(ansi.Wordwrap("This needs a voice: run `teacher setup voice`, then `teacher doctor` to check that piper and a player are found.", width, "")))
		return b.String()
	}
	var labels []string
	order := a.prosodyOrder()
	for _, d := range order[:min(3, len(order))] {
		labels = append(labels, d.Label)
	}
	b.WriteString(r.value.Render(ansi.Wordwrap(count+" · Weakest first: "+strings.Join(labels, ", "), width, "")) + "\n")
	b.WriteString(r.label.Render(ansi.Wordwrap("Each one plays when it appears: press its number. p plays it again, s slower. Press enter to start.", width, "")))
	return b.String()
}

// prosodyItemView is one word or sentence: the options, then the
// feedback.
func (a App) prosodyItemView(width int) string {
	r := a.st.main
	p := a.prosody
	it := p.items[p.pos]
	var b strings.Builder
	b.WriteString(r.title.Render(a.prosodyTitle()) + "\n")
	b.WriteString(r.label.Render(fmt.Sprintf("%s %d of %d · ✓ %d", a.prosodyNoun(), p.pos+1, len(p.items), p.score())) + "\n\n")
	question := "Which syllable is stressed?"
	if a.pairs.kind == pairsTone {
		question = "Did the voice rise or fall?"
	}
	b.WriteString(r.secondary.Render(question) + "  " + r.target.Render(it.Shown) + "\n\n")
	var opts []string
	for i, c := range it.Choices {
		opts = append(opts, r.accent.Render(fmt.Sprintf("%d ", i+1))+r.target.Render(c))
	}
	b.WriteString(indent(ansi.Wordwrap(strings.Join(opts, "     "), width-2, ""), 2))
	if p.phase == pairsAsking {
		return b.String()
	}
	b.WriteString("\n\n")
	if p.right(p.pos) {
		b.WriteString("  " + r.success.Render("✓ "+it.Reveal))
	} else {
		d := drillByID(it.Drill)
		wrong := "✗ The stress is on " + strings.ToUpper(it.Choices[it.Answer]) + ": " + it.Reveal
		if a.pairs.kind == pairsTone {
			wrong = "✗ You heard a " + strings.Fields(it.Choices[it.Answer])[0] + ": " + it.Reveal
		}
		b.WriteString("  " + r.danger.Render(wrong) + "\n")
		b.WriteString("  " + r.secondary.Render(d.Label) + "\n")
		b.WriteString(indent(r.value.Render(ansi.Wordwrap(d.Tip, width-4, "")), 2))
	}
	next := "p plays it again. Press enter for the next one."
	if p.pos == len(p.items)-1 {
		next = "p plays it again. Press enter to see your result."
	}
	return b.String() + "\n\n" + r.label.Render(ansi.Wordwrap(next, width, ""))
}

// prosodyResultView is the score, each drill of the round, and the
// weakest one with its tip.
func (a App) prosodyResultView(width int) string {
	r := a.st.main
	p := a.prosody
	var b strings.Builder
	b.WriteString(r.title.Render(a.prosodyTitle()) + "\n\n")
	n := len(p.items)
	score := r.accent.Render(fmt.Sprintf("%d/%d", p.score(), n))
	if p.score() == n {
		score = r.success.Render(fmt.Sprintf("✓ %d/%d", p.score(), n))
	}
	b.WriteString(score + r.label.Render("  ·  saved in today's log") + "\n")
	if p.saveErr != nil {
		b.WriteString(r.danger.Render("Could not save your results: "+p.saveErr.Error()) + "\n")
	}
	b.WriteString(a.roundStatsView(domain.TallyProsody(p.items, p.rights()), width, func(id string) (string, string) {
		d := drillByID(id)
		return d.Label, d.Tip
	}))
	return strings.TrimSuffix(b.String(), "\n")
}

// prosodyHints are the keys of a word-stress or intonation round.
func (a App) prosodyHints() []string {
	p := a.prosody
	switch p.phase {
	case pairsAsking:
		return []string{fmt.Sprintf("1–%d choose", len(p.items[p.pos].Choices)), "p again", "s slower", "tab screens", "esc home"}
	case pairsAnswered:
		next := "enter next"
		if p.pos == len(p.items)-1 {
			next = "enter result"
		}
		return []string{next, "p again", "s slower", "esc home"}
	case pairsResult:
		return []string{"n new round", "tab screens", "esc home"}
	}
	return a.pairsIntroHints()
}
