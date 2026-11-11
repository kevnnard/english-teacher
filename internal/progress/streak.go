// Package progress computes the learner's streak, week and readiness for
// the next CEFR level from study days.
// It is pure: no I/O and no clock; today is always passed in.
package progress

import (
	"slices"

	"github.com/kevnnard/english-teacher/internal/domain"
)

// Streak is the learner's run of consecutive study days.
type Streak struct {
	Current      int  // consecutive study days ending today or yesterday
	Longest      int  // the longest run ever
	StudiedToday bool // today already counts
}

// ComputeStreak counts consecutive studied days. A streak survives until
// the end of the day after the last study day, so "yesterday" still counts
// as current: the learner has until midnight to keep it.
func ComputeStreak(days []domain.Day, today domain.Date) Streak {
	var dates []domain.Date
	for _, d := range days {
		if d.Studied() && !today.Before(d.Date) {
			dates = append(dates, d.Date)
		}
	}
	slices.SortFunc(dates, func(a, b domain.Date) int {
		switch {
		case a.Before(b):
			return -1
		case b.Before(a):
			return 1
		}
		return 0
	})
	dates = slices.Compact(dates)

	var s Streak
	run := 0
	for i, d := range dates {
		if i > 0 && dates[i-1].AddDays(1) == d {
			run++
		} else {
			run = 1
		}
		s.Longest = max(s.Longest, run)
	}
	if n := len(dates); n > 0 {
		last := dates[n-1]
		s.StudiedToday = last == today
		if last == today || last == today.AddDays(-1) {
			s.Current = run
		}
	}
	return s
}

// Week returns the seven days ending today, oldest first. Days with no
// record come back empty, with only their date set.
func Week(days []domain.Day, today domain.Date) [7]domain.Day {
	var w [7]domain.Day
	for i := range w {
		w[i] = DayOf(days, today.AddDays(i-6))
	}
	return w
}

// DayOf returns the record of date, or an empty day with that date.
func DayOf(days []domain.Day, date domain.Date) domain.Day {
	for _, d := range days {
		if d.Date == date {
			return d
		}
	}
	return domain.Day{Date: date}
}

// Add folds delta into its day the way the store does: minutes and counts
// are summed but never go below zero, and new activities are appended.
// It returns the updated days; a missing day is created.
func Add(days []domain.Day, delta domain.Day) []domain.Day {
	days = slices.Clone(days)
	i := slices.IndexFunc(days, func(d domain.Day) bool { return d.Date == delta.Date })
	if i < 0 {
		days = append(days, domain.Day{Date: delta.Date})
		i = len(days) - 1
	}
	d := &days[i]
	d.Minutes = max(0, d.Minutes+delta.Minutes)
	d.Reviews = max(0, d.Reviews+delta.Reviews)
	d.Correct = max(0, d.Correct+delta.Correct)
	d.Failed = max(0, d.Failed+delta.Failed)
	d.WordsRead = max(0, d.WordsRead+delta.WordsRead)
	d.Activities = slices.Clone(d.Activities)
	for _, a := range delta.Activities {
		if !slices.Contains(d.Activities, a) {
			d.Activities = append(d.Activities, a)
		}
	}
	return days
}
