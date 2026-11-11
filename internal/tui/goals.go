package tui

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/kevnnard/english-teacher/internal/domain"
	"github.com/kevnnard/english-teacher/internal/progress"
)

// The Progress screen (spec S5) shows this week against last week and the
// "can do" goals of the learner's level. It is opened with g on Home and
// is not a tab, like Level. Every number comes from the stored data; a
// week with nothing recorded says so instead of showing zeros.

// summary is this week against last week, from the data in memory.
func (a App) summary() progress.WeekSummary {
	data := progress.WeekData{Days: a.days, Cards: a.cards, Mistakes: a.mistakes}
	return progress.WeeklySummary(data, a.today())
}

// canDo is the status of every goal of the learner's level. Only the
// evidence since the level started counts.
func (a App) canDo() []progress.GoalStatus {
	data := progress.GoalData{Days: a.days, Ticks: a.goalTicks, Since: a.profile.Started}
	return progress.CanDo(a.profile.Level, data)
}

// trackedCount is how many goals of all some activity can tick.
func trackedCount(all []progress.GoalStatus) int {
	n := 0
	for _, g := range all {
		if g.Tracked() {
			n++
		}
	}
	return n
}

// goalsLine is Home's "2 of 6 A1 can-do goals   g".
func (a App) goalsLine() string {
	r := a.st.main
	all := a.canDo()
	return r.value.Render(fmt.Sprintf("%d of %d", progress.TickedCount(all), trackedCount(all))) +
		r.secondary.Render(fmt.Sprintf(" %s can-do goals", a.profile.Level)) +
		r.label.Render("   ") + r.accent.Render("g")
}

// tickGoals stores a tick for every goal of the learner's level proved by
// evidence of kind rule, which the activity log does not keep. A goal is
// ticked in memory only once it is stored.
func (a *App) tickGoals(rule domain.GoalRule) error {
	for _, g := range domain.GoalsFor(a.profile.Level) {
		if g.Rule != rule {
			continue
		}
		if _, ok := a.goalTicks[g.ID]; ok {
			continue
		}
		if a.tickGoal != nil {
			if err := a.tickGoal(g.ID, a.today()); err != nil {
				return fmt.Errorf("could not save the can-do goal: %w", err)
			}
		}
		if a.goalTicks == nil {
			a.goalTicks = map[string]domain.Date{}
		}
		a.goalTicks[g.ID] = a.today()
	}
	return nil
}

func (a App) updateProgress(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	n := len(domain.GoalsFor(a.profile.Level))
	switch msg.String() {
	case "down", "j":
		a.goalCursor = min(a.goalCursor+1, max(n-1, 0))
	case "up", "k":
		a.goalCursor = max(a.goalCursor-1, 0)
	}
	return a, nil
}

// progressHints are the keys of the Progress screen.
func (a App) progressHints() []string {
	return []string{"↑↓ choose a goal", "esc home", "? help"}
}

// Column widths of the weekly table.
const (
	weekLabelWidth = 20
	weekColWidth   = 12
)

func (a App) progressView() string {
	var b strings.Builder
	a.weekTable(&b)
	b.WriteString("\n")
	a.goalList(&b)
	return strings.TrimRight(b.String(), "\n")
}

// weekTable writes this week against last week, one row per number, then
// the top error kinds and whether they are falling.
func (a App) weekTable(b *strings.Builder) {
	r := a.st.main
	s := a.summary()
	b.WriteString(r.title.Render(fmt.Sprintf("%-*s", weekLabelWidth+2, "Week")) +
		r.label.Render(fmt.Sprintf("%-*s%s", weekColWidth, "this week", "last week")) + "\n")
	if s.Empty() {
		b.WriteString("  " + r.secondary.Render(progress.NoWeekData) + "\n")
		return
	}
	row := func(label, this, last string) {
		b.WriteString("  " + r.secondary.Render(fmt.Sprintf("%-*s", weekLabelWidth, label)) +
			r.value.Render(fmt.Sprintf("%-*s", weekColWidth, this)) + r.label.Render(last) + "\n")
	}
	row("Study time", fmt.Sprintf("%d min", s.This.Minutes), fmt.Sprintf("%d min", s.Last.Minutes))
	row("New words", fmt.Sprint(s.This.NewCards), fmt.Sprint(s.Last.NewCards))
	row("Words learned", fmt.Sprint(s.This.NewKnown), fmt.Sprint(s.Last.NewKnown))
	row("Words read", fmt.Sprint(s.This.WordsRead), fmt.Sprint(s.Last.WordsRead))
	row("Writing average", writingAverage(s.This), writingAverage(s.Last))

	b.WriteString("  " + r.secondary.Render("Top errors") + "\n")
	if len(s.Errors) == 0 {
		b.WriteString("    " + r.label.Render("No major errors logged in these two weeks.") + "\n")
	}
	for _, e := range s.Errors {
		trend := r.label.Render("= " + e.Trend())
		switch e.Trend() {
		case progress.Falling:
			trend = r.success.Render("↓ " + e.Trend())
		case progress.Rising:
			trend = r.warning.Render("↑ " + e.Trend())
		}
		b.WriteString("    " + r.secondary.Render(fmt.Sprintf("%-*s", weekLabelWidth-2, e.Kind)) +
			r.value.Render(fmt.Sprintf("%-*d", weekColWidth, e.This)) +
			r.label.Render(fmt.Sprintf("%-*d", weekColWidth, e.Last)) + trend + "\n")
	}
}

// writingAverage is "32.5/40", or "—" for a week with no texts.
func writingAverage(w progress.WeekStats) string {
	avg, ok := w.WritingAverage()
	if !ok {
		return "—"
	}
	return strings.TrimSuffix(fmt.Sprintf("%.1f", avg), ".0") + fmt.Sprintf("/%d", progress.WritingMax)
}

// goalList writes the can-do goals of the level, the selected one marked,
// and under them how the selected goal is proved.
func (a App) goalList(b *strings.Builder) {
	r := a.st.main
	all := a.canDo()
	b.WriteString(r.title.Render("Can do · "+a.profile.Level.String()) + r.label.Render("   ") +
		r.value.Render(fmt.Sprintf("%d of %d ticked", progress.TickedCount(all), trackedCount(all))) + "\n")
	if len(all) == 0 {
		return
	}
	cursor := min(a.goalCursor, len(all)-1)
	for i, g := range all {
		mark := "  "
		if i == cursor {
			mark = r.accent.Render("▸ ")
		}
		glyph, text := r.label.Render("○"), r.secondary.Render(g.Goal.Text)
		switch {
		case g.Ticked:
			glyph, text = r.success.Render("✓"), r.value.Render(g.Goal.Text)
		case !g.Tracked():
			glyph, text = r.label.Render("–"), r.label.Render(g.Goal.Text)
		}
		b.WriteString(mark + glyph + " " + text + "\n")
	}
	g := all[cursor]
	var how string
	switch {
	case g.Ticked:
		how = r.success.Render(fmt.Sprintf("Proved on %d %s", g.On.Day, g.On.Month.String()[:3]))
	case g.Tracked():
		how = r.secondary.Render("To tick it: " + g.Goal.Proof)
	default:
		how = r.label.Render("No activity in the app proves this one yet.")
	}
	b.WriteString("    " + how + "\n")
}
