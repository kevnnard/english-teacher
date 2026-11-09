package tui

import (
	"fmt"
	"strings"

	"github.com/kevnnard/english-teacher/internal/progress"
)

func (a App) statsView() string {
	r := a.st.main
	counts := a.countByStatus()

	var b strings.Builder
	b.WriteString(r.title.Render("Words by status") + "\n")
	for _, s := range statusOrder {
		fmt.Fprintf(&b, "  %s %-12s%d\n", a.glyph(s), s, counts[s])
	}

	streak := a.streak()
	b.WriteString("\n" + r.label.Render(fmt.Sprintf("%-10s", "Streak")) + r.value.Render(days(streak.Current)) +
		"    " + r.label.Render("Longest  ") + r.value.Render(days(streak.Longest)) + "\n")

	b.WriteString("\n" + r.title.Render("Last 7 days") + "\n")
	week := progress.Week(a.days, a.today())
	for i := len(week) - 1; i >= 0; i-- { // today first
		d := week[i]
		label := fmt.Sprintf("  %s %d %s", weekday(d.Date), d.Date.Day, d.Date.Month.String()[:3])
		b.WriteString(r.secondary.Render(fmt.Sprintf("%-14s", label)))
		if !d.Studied() && d.Minutes == 0 {
			b.WriteString(r.label.Render("—") + "\n")
			continue
		}
		b.WriteString(r.value.Render(fmt.Sprintf("%3d min   %3d %s", d.Minutes, d.Reviews, plural(d.Reviews, "card ", "cards"))))
		if d.Reviews > 0 {
			b.WriteString("   " + r.success.Render(fmt.Sprintf("✓ %d", d.Correct)) + "  " + r.danger.Render(fmt.Sprintf("✗ %d", d.Failed)))
		}
		if len(d.Activities) > 0 {
			b.WriteString("   " + r.label.Render(strings.Join(d.Activities, ", ")))
		}
		b.WriteString("\n")
	}
	return strings.TrimRight(b.String(), "\n")
}

// days is "1 day" or "N days".
func days(n int) string {
	return fmt.Sprintf("%d %s", n, plural(n, "day", "days"))
}
