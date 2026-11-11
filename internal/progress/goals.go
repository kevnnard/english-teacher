package progress

import (
	"slices"
	"strings"

	"github.com/kevnnard/english-teacher/internal/domain"
)

// This file ticks the "can do" goals (spec S5) of the learner's level
// from real evidence. Most rules read the daily activity log, so they are
// recomputed every time; a story question answered right is not in the
// log, so that tick is stored when it happens and passed in as Ticks.

// GoalData is the evidence the goals read.
type GoalData struct {
	Days  []domain.Day           // recorded study days with their activities, in any order
	Ticks map[string]domain.Date // stored ticks by goal id, with the day each was proved
	Since domain.Date            // the day the level started; only later evidence counts; zero means all
}

// GoalStatus is one goal and whether it is ticked.
type GoalStatus struct {
	Goal   domain.Goal
	Ticked bool
	On     domain.Date // the day the evidence was complete; zero when not ticked
}

// Tracked reports whether some activity can tick the goal.
func (g GoalStatus) Tracked() bool {
	return g.Goal.Rule != domain.NotTracked
}

// TickedCount is how many goals of all are ticked.
func TickedCount(all []GoalStatus) int {
	n := 0
	for _, g := range all {
		if g.Ticked {
			n++
		}
	}
	return n
}

// CanDo returns the status of every goal of level, in the order of
// domain.GoalsFor. It is pure.
func CanDo(level domain.Level, data GoalData) []GoalStatus {
	days := slices.Clone(data.Days)
	slices.SortStableFunc(days, func(a, b domain.Day) int { return compareDates(a.Date, b.Date) })
	goals := domain.GoalsFor(level)
	all := make([]GoalStatus, len(goals))
	for i, g := range goals {
		all[i] = GoalStatus{Goal: g}
		if g.Rule == domain.NotTracked {
			continue // a stored tick cannot tick it either
		}
		if on, ok := data.Ticks[g.ID]; ok {
			all[i].Ticked, all[i].On = true, on
			continue
		}
		all[i].On, all[i].Ticked = provedOn(g, days, data.Since)
	}
	return all
}

// provedOn is the day the log, read in date order from since, first has
// g.Times() activities that match g.
func provedOn(g domain.Goal, days []domain.Day, since domain.Date) (domain.Date, bool) {
	n := 0
	for _, d := range days {
		if d.Date.Before(since) {
			continue
		}
		for _, a := range d.Activities {
			if matches(g, a) {
				n++
			}
		}
		if n >= g.Times() {
			return d.Date, true
		}
	}
	return domain.Date{}, false
}

// matches reports whether the activity a is evidence for g.
func matches(g domain.Goal, a string) bool {
	switch g.Rule {
	case domain.ActivityDone:
		return strings.HasPrefix(a, g.Activity)
	case domain.ScoreAtLeast:
		rest, ok := strings.CutPrefix(a, g.Activity)
		if !ok {
			return false
		}
		s, ok := parseScore(rest)
		return ok && g.Passes(s.Right, s.Total)
	case domain.ChaptersRead:
		return isChapter(a)
	}
	return false // StoryAnswered and NotTracked are not in the log
}

// isChapter reports whether a is a story chapter, "Reading: <title> ch N",
// and not a short Learn text, "Reading: <title>".
func isChapter(a string) bool {
	rest, ok := strings.CutPrefix(a, domain.ReadingPrefix)
	if !ok {
		return false
	}
	i := strings.LastIndex(rest, " ch ")
	if i < 0 {
		return false
	}
	num := rest[i+len(" ch "):]
	return num != "" && strings.Trim(num, "0123456789") == ""
}
