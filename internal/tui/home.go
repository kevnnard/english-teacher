package tui

import (
	"fmt"
	"math"
	"strconv"
	"strings"

	"github.com/kevnnard/english-teacher/internal/domain"
	"github.com/kevnnard/english-teacher/internal/progress"
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
	b.WriteString(r.target.Render(greeting(a.now().Hour(), a.profile.Name)) + "\n\n")

	act := a.homeAction()
	if act.target != Review {
		b.WriteString(r.secondary.Render("No reviews due — come back tomorrow.") + "\n\n")
	}
	b.WriteString(r.accent.Render("▸ ") + r.target.Render(fmt.Sprintf("%-24s", act.label)) +
		r.label.Render(fmt.Sprintf("%-12s", act.detail)) + r.accent.Render("enter") + "\n\n")

	b.WriteString(r.label.Render(fmt.Sprintf("%-8s", "Words")) + a.statusCounts() + "\n\n")
	b.WriteString(r.label.Render(fmt.Sprintf("%-8s", "Streak")) + a.streakLine() + "\n")
	b.WriteString(r.label.Render(fmt.Sprintf("%-8s", "Today")) + a.goalLine() + "\n")
	b.WriteString(r.label.Render(fmt.Sprintf("%-8s", "Week")) + a.weekRow() + "\n")
	b.WriteString(r.label.Render(fmt.Sprintf("%-8s", "Read")) + r.value.Render(strconv.Itoa(a.wordsReadThisWeek())) +
		r.secondary.Render(" words this week") + "\n\n")
	b.WriteString(r.label.Render(fmt.Sprintf("%-8s", "Level")) + a.levelLine() + "\n")
	b.WriteString(r.label.Render(fmt.Sprintf("%-8s", "Goals")) + a.goalsLine())
	return b.String()
}

// streakLine says where the streak stands and what to do about it, so a
// zero streak still points at the next step instead of hiding.
func (a App) streakLine() string {
	r := a.st.main
	st := a.streak()
	count := r.value.Render(days(st.Current))
	switch {
	case st.StudiedToday:
		line := count + " " + r.success.Render("✓ today")
		if st.Longest > st.Current {
			line += r.label.Render(" · best " + strconv.Itoa(st.Longest))
		}
		return line
	case st.Current > 0:
		return count + r.secondary.Render(" — review today to keep it")
	}
	if last, ok := a.lastStudied(); ok {
		count += r.label.Render(fmt.Sprintf(" · last studied %d %s", last.Day, last.Month.String()[:3]))
	}
	return count + r.secondary.Render(" — review today to start one")
}

// lastStudied is the most recent day that counts for the streak.
func (a App) lastStudied() (domain.Date, bool) {
	var last domain.Date
	found := false
	for _, d := range a.days {
		if d.Studied() && (!found || last.Before(d.Date)) {
			last, found = d.Date, true
		}
	}
	return last, found
}

// goalBarWidth is how many cells the daily goal bar takes.
const goalBarWidth = 10

// goalLine is "12 of 25 min  ▰▰▰▰▱▱▱▱▱▱": today's study time against the goal.
func (a App) goalLine() string {
	r := a.st.main
	done := progress.DayOf(a.days, a.today()).Minutes
	goal := a.profile.DailyGoalMinutes
	filled := min(done*goalBarWidth/goal, goalBarWidth)
	bar := r.success.Render(strings.Repeat("▰", filled)) + r.label.Render(strings.Repeat("▱", goalBarWidth-filled))
	text := r.value.Render(fmt.Sprintf("%d of %d min", done, goal))
	if done >= goal {
		text += r.success.Render("  goal met")
	}
	return text + "  " + bar
}

// weekRow is "Tu ○  We ●  …  Mo ●": the last seven days, today last. The
// filled dot means the day counts for the streak.
func (a App) weekRow() string {
	r := a.st.main
	week := progress.Week(a.days, a.today())
	parts := make([]string, len(week))
	for i, d := range week {
		dot := r.label.Render("○")
		if d.Studied() {
			dot = r.success.Render("●")
		}
		parts[i] = r.secondary.Render(weekday(d.Date)) + " " + dot
	}
	return strings.Join(parts, "  ")
}

// weekday is the two-letter name of the day d falls on, like "Mo".
func weekday(d domain.Date) string {
	return d.Weekday().String()[:2]
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

// greeting is "Good evening." or, when the learner set a name,
// "Good evening, Kevin.".
func greeting(hour int, name string) string {
	var g string
	switch {
	case hour < 12:
		g = "Good morning"
	case hour < 18:
		g = "Good afternoon"
	default:
		g = "Good evening"
	}
	if name != "" {
		g += ", " + name
	}
	return g + "."
}

func plural(n int, one, many string) string {
	if n == 1 {
		return one
	}
	return many
}
