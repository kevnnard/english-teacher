package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/x/ansi"

	"github.com/kevnnard/english-teacher/internal/domain"
)

// pairsTitle heads every page of the Pairs screen.
const pairsTitle = "Pairs · Minimal pairs"

func (a App) pairsView(w int) string {
	width := max(w, 20)
	if a.pairs.kind != pairsMinimal {
		return a.prosodyView(width)
	}
	switch a.pairs.phase {
	case pairsAsking, pairsAnswered:
		return a.pairsItemView(width)
	case pairsResult:
		return a.pairsResultView(width)
	}
	return a.pairsIntroView(width)
}

// pairsIntroView says what a round is and which sounds come first, or
// how to turn audio on.
func (a App) pairsIntroView(width int) string {
	r := a.st.main
	var b strings.Builder
	b.WriteString(r.title.Render(pairsTitle) + "\n\n")
	b.WriteString(r.target.Render(ansi.Wordwrap(
		"Hear one word of a pair like ship / sheep and choose which one it was. "+
			"These sounds are one in Spanish, so the ear has to learn them.", width, "")) + "\n\n")
	if a.speaker == nil {
		b.WriteString(r.warning.Render(audioOffNotice) + "\n")
		b.WriteString(r.label.Render(ansi.Wordwrap("Minimal pairs need a voice: run `teacher setup voice`, then `teacher doctor` to check that piper and a player are found.", width, "")))
		return b.String()
	}
	var labels []string
	for _, c := range a.pairsOrder()[:3] {
		labels = append(labels, c.Label)
	}
	b.WriteString(r.value.Render(fmt.Sprintf("%d words · Weakest first: %s", pairsItems, strings.Join(labels, ", "))) + "\n")
	voices := "Words play in one voice; add a second voice to hear two."
	if len(a.voices) >= 2 {
		voices = fmt.Sprintf("Words take turns in your %d voices.", len(a.voices))
	}
	b.WriteString(r.label.Render(ansi.Wordwrap(voices+" Each word plays when it appears: press 1 or 2. p plays it again, s slower. Press enter to start.", width, "")))
	return b.String()
}

// pairsItemView is one word: the two choices, then the feedback.
func (a App) pairsItemView(width int) string {
	r := a.st.main
	p := a.pairs
	it := p.items[p.pos]
	var b strings.Builder
	b.WriteString(r.title.Render(pairsTitle) + "\n")
	b.WriteString(r.label.Render(fmt.Sprintf("Word %d of %d · ✓ %d", p.pos+1, len(p.items), p.score())) + "\n\n")
	b.WriteString(r.secondary.Render("Which word did you hear?") + "\n\n")
	b.WriteString("  " + r.accent.Render("1 ") + r.target.Render(it.Pair[0]) +
		"     " + r.accent.Render("2 ") + r.target.Render(it.Pair[1]))
	if p.phase == pairsAsking {
		return b.String()
	}
	b.WriteString("\n\n")
	if p.right(p.pos) {
		b.WriteString("  " + r.success.Render("✓ "+it.Word()))
	} else {
		c := contrastByID(it.Contrast)
		b.WriteString("  " + r.danger.Render("✗ You heard: "+it.Word()) + "\n")
		b.WriteString("  " + r.secondary.Render(c.Label) + "\n")
		b.WriteString(indent(r.value.Render(ansi.Wordwrap(c.Tip, width-4, "")), 2))
	}
	next := "1 and 2 play each word. Press enter for the next word."
	if p.pos == len(p.items)-1 {
		next = "1 and 2 play each word. Press enter to see your result."
	}
	return b.String() + "\n\n" + r.label.Render(ansi.Wordwrap(next, width, ""))
}

// pairsResultView is the score, each contrast of the round, and the
// weakest one with its tip.
func (a App) pairsResultView(width int) string {
	r := a.st.main
	p := a.pairs
	var b strings.Builder
	b.WriteString(r.title.Render(pairsTitle) + "\n\n")
	n := len(p.items)
	score := r.accent.Render(fmt.Sprintf("%d/%d", p.score(), n))
	if p.score() == n {
		score = r.success.Render(fmt.Sprintf("✓ %d/%d", p.score(), n))
	}
	b.WriteString(score + r.label.Render("  ·  saved in today's log") + "\n")
	if p.saveErr != nil {
		b.WriteString(r.danger.Render("Could not save your pair results: "+p.saveErr.Error()) + "\n")
	}

	round := domain.TallyPairs(p.items, p.rights())
	b.WriteString(a.roundStatsView(round, width, func(id string) (string, string) {
		c := contrastByID(id)
		return c.Label, c.Tip
	}))
	return strings.TrimSuffix(b.String(), "\n")
}

// roundStatsView is the "This round" list of a Pairs result: each
// contrast or drill of the round with its score, then the weakest one
// with its tip. about gives the label and tip of an ID.
func (a App) roundStatsView(round []domain.PairStat, width int, about func(id string) (label, tip string)) string {
	r := a.st.main
	var b strings.Builder
	b.WriteString("\n" + r.secondary.Render("This round") + "\n")
	pad := 24 // the labels line up, at least as wide as a contrast's
	for _, st := range round {
		label, _ := about(st.Contrast)
		pad = max(pad, ansi.StringWidth(label))
	}
	var weakest *domain.PairStat
	for i, st := range round {
		label, _ := about(st.Contrast)
		mark := r.success.Render("✓")
		if st.Correct < st.Attempts {
			mark = r.danger.Render("✗")
		}
		label += strings.Repeat(" ", pad-ansi.StringWidth(label))
		fmt.Fprintf(&b, "  %s %s %s\n", mark, r.value.Render(label),
			r.label.Render(fmt.Sprintf("%d/%d", st.Correct, st.Attempts)))
		if st.Correct < st.Attempts && (weakest == nil || st.Accuracy() < weakest.Accuracy()) {
			weakest = &round[i]
		}
	}
	if weakest != nil {
		label, tip := about(weakest.Contrast)
		b.WriteString("\n" + r.secondary.Render("Weakest: "+label) + "\n")
		b.WriteString(indent(r.value.Render(ansi.Wordwrap(tip, width-2, "")), 2))
	}
	return b.String()
}

// contrastByID is the built-in contrast with id; the zero Contrast when
// there is none.
func contrastByID(id string) domain.Contrast {
	for _, c := range domain.Contrasts() {
		if c.ID == id {
			return c
		}
	}
	return domain.Contrast{}
}

// pairsHints are the keys of the Pairs screen. No text is typed here, so
// p and s are the audio keys.
func (a App) pairsHints() []string {
	if a.pairs.kind != pairsMinimal {
		return a.prosodyHints()
	}
	p := a.pairs
	switch p.phase {
	case pairsAsking:
		return []string{"1 / 2 choose", "p again", "s slower", "tab screens", "esc home"}
	case pairsAnswered:
		next := "enter next"
		if p.pos == len(p.items)-1 {
			next = "enter result"
		}
		return []string{next, "1 / 2 hear each", "p again", "s slower", "esc home"}
	case pairsResult:
		return []string{"n new round", "tab screens", "esc home"}
	}
	return a.pairsIntroHints()
}

// pairsIntroHints are the keys of an intro of any kind: `m` switches to
// the next kind.
func (a App) pairsIntroHints() []string {
	m := "m " + ((a.pairs.kind + 1) % pairsKinds).String()
	if a.pairs.kind == pairsMinimal {
		m = "m stress · intonation"
	}
	if a.speaker != nil {
		return []string{"enter start", m, "tab screens", "esc home"}
	}
	return []string{m, "tab screens", "esc home", "? help"}
}
