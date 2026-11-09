package tui

import (
	"time"

	"github.com/kevnnard/english-teacher/internal/domain"
	"github.com/kevnnard/english-teacher/internal/progress"
)

// maxStudyGap is the most study time one grade can add. Longer gaps mean
// the learner walked away, and idle time must not count as study.
const maxStudyGap = 2 * time.Minute

// today is the learner's local calendar day.
func (a App) today() domain.Date {
	return domain.DateOf(a.now())
}

// streak is the learner's streak, counting today's grades so far.
func (a App) streak() progress.Streak {
	return progress.ComputeStreak(a.days, a.today())
}

// recordGrade adds one graded card, and the time spent on it, to today.
func (a *App) recordGrade(passed bool) {
	delta := domain.Day{Reviews: 1, Minutes: a.takeStudyMinutes()}
	if passed {
		delta.Correct = 1
	} else {
		delta.Failed = 1
	}
	a.addToToday(delta)
}

// recordUndo takes an undone grade back from date, the day it was
// recorded on, even when midnight has passed since. The time spent stays:
// it was still study.
func (a *App) recordUndo(date domain.Date, failed bool) {
	delta := domain.Day{Date: date, Reviews: -1}
	if failed {
		delta.Failed = -1
	} else {
		delta.Correct = -1
	}
	a.addToDay(delta)
}

// takeStudyMinutes adds the time since the last grade (capped by
// maxStudyGap) and returns the whole minutes ready to record; the rest
// waits for the next grade.
func (a *App) takeStudyMinutes() int {
	now := a.now()
	a.studied += min(max(now.Sub(a.lastStudy), 0), maxStudyGap)
	a.lastStudy = now
	minutes := int(a.studied / time.Minute)
	a.studied -= time.Duration(minutes) * time.Minute
	return minutes
}

// addToToday records delta on today's date.
func (a *App) addToToday(delta domain.Day) {
	delta.Date = a.today()
	a.addToDay(delta)
}

// addToDay records delta on its date, in the store and in memory.
// A failed record stops the app like a failed save.
func (a *App) addToDay(delta domain.Day) {
	if a.record != nil {
		if err := a.record(delta); err != nil {
			a.err = err
			return
		}
	}
	a.days = progress.Add(a.days, delta)
}
