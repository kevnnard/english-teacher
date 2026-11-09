package tui

import (
	"fmt"
	"strings"
	"time"
)

func (a App) statsView() string {
	r := a.st.main
	counts := a.countByStatus()

	var b strings.Builder
	b.WriteString(r.title.Render("Words by status") + "\n")
	for _, s := range statusOrder {
		fmt.Fprintf(&b, "  %s %-12s%d\n", a.glyph(s), s, counts[s])
	}
	b.WriteString("\n" + r.label.Render("Reviewed today") + fmt.Sprintf("  %d", a.reviewedToday()))
	return b.String()
}

// reviewedToday counts the cards whose last review fell on today's date.
func (a App) reviewedToday() int {
	now := a.now()
	n := 0
	for _, c := range a.cards {
		if c.LastReviewed != nil && sameDay(*c.LastReviewed, now) {
			n++
		}
	}
	return n
}

func sameDay(t, now time.Time) bool {
	t = t.In(now.Location())
	y1, m1, d1 := t.Date()
	y2, m2, d2 := now.Date()
	return y1 == y2 && m1 == m2 && d1 == d2
}
