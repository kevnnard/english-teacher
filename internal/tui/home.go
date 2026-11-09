package tui

import (
	"fmt"
	"math"
	"strings"

	"github.com/kevnnard/english-teacher/internal/domain"
)

// minutesPerCard is the time estimate behind "~N min" on Home.
const minutesPerCard = 0.5

// homeAction is the single preselected action on Home (the One Next Action Rule).
type homeAction struct {
	label  string // e.g. "Review 12 words"
	detail string // e.g. "~6 min"
	target Screen
}

// homeAction picks the next thing to do: review while anything is due,
// otherwise browse the words.
func (a App) homeAction() homeAction {
	if n := a.review.remaining(); n > 0 {
		minutes := int(math.Ceil(float64(n) * minutesPerCard))
		return homeAction{
			label:  fmt.Sprintf("Review %d %s", n, plural(n, "word", "words")),
			detail: fmt.Sprintf("~%d min", minutes),
			target: Review,
		}
	}
	n := len(a.wordCards())
	return homeAction{label: fmt.Sprintf("Browse %d %s", n, plural(n, "word", "words")), target: Words}
}

func (a App) homeView() string {
	r := a.st.main
	var b strings.Builder
	b.WriteString(r.target.Render(greeting(a.now().Hour())) + "\n\n")

	act := a.homeAction()
	if act.target != Review {
		b.WriteString(r.secondary.Render("No reviews due — come back tomorrow.") + "\n\n")
	}
	b.WriteString(r.accent.Render("▸ ") + r.target.Render(fmt.Sprintf("%-24s", act.label)) +
		r.label.Render(fmt.Sprintf("%-12s", act.detail)) + r.accent.Render("enter") + "\n\n")

	b.WriteString(r.label.Render("Words") + "  " + a.statusCounts())
	return b.String()
}

// statusCounts is "● 3 due   ◐ 1 learning   ✓ 1 known   ○ 2 new".
func (a App) statusCounts() string {
	counts := a.countByStatus()
	parts := make([]string, 0, len(statusOrder))
	for _, s := range statusOrder {
		parts = append(parts, a.glyph(s)+fmt.Sprintf(" %d %s", counts[s], s))
	}
	return strings.Join(parts, "   ")
}

// statusOrder is how statuses are listed: what needs attention first.
var statusOrder = []domain.Status{domain.StatusDue, domain.StatusLearning, domain.StatusKnown, domain.StatusNew}

func (a App) countByStatus() map[domain.Status]int {
	counts := map[domain.Status]int{}
	now := a.now()
	for _, c := range a.wordCards() {
		counts[c.Status(now)]++
	}
	return counts
}

// glyph is the colored status symbol. Color is never the only signal: the
// glyph's shape and the status word next to it carry the same meaning.
func (a App) glyph(s domain.Status) string {
	r := a.st.main
	switch s {
	case domain.StatusDue:
		return r.accent.Render("●")
	case domain.StatusLearning:
		return r.warning.Render("◐")
	case domain.StatusKnown:
		return r.success.Render("✓")
	default:
		return r.label.Render("○")
	}
}

func greeting(hour int) string {
	switch {
	case hour < 12:
		return "Good morning."
	case hour < 18:
		return "Good afternoon."
	default:
		return "Good evening."
	}
}

func plural(n int, one, many string) string {
	if n == 1 {
		return one
	}
	return many
}
