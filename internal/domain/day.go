package domain

import (
	"fmt"
	"strings"
	"time"
)

// Date is a calendar day with no time of day and no time zone. Study days
// are counted in the learner's local calendar, so a session at 11 pm and
// one at 1 am the next morning are two different days.
type Date struct {
	Year  int
	Month time.Month
	Day   int
}

// DateOf returns the calendar day of t in t's own location.
func DateOf(t time.Time) Date {
	y, m, d := t.Date()
	return Date{y, m, d}
}

// ParseDate reads a YYYY-MM-DD date.
func ParseDate(s string) (Date, error) {
	t, err := time.Parse(time.DateOnly, s)
	if err != nil {
		return Date{}, fmt.Errorf("date %q is not YYYY-MM-DD", s)
	}
	return DateOf(t), nil
}

// String formats d as YYYY-MM-DD.
func (d Date) String() string {
	return fmt.Sprintf("%04d-%02d-%02d", d.Year, d.Month, d.Day)
}

// AddDays returns the day n days after d (before it when n is negative).
func (d Date) AddDays(n int) Date {
	return DateOf(d.time().AddDate(0, 0, n))
}

// Before reports whether d comes before o.
func (d Date) Before(o Date) bool {
	return d.time().Before(o.time())
}

// time is midnight UTC of d, only used for calendar arithmetic.
func (d Date) time() time.Time {
	return time.Date(d.Year, d.Month, d.Day, 0, 0, 0, 0, time.UTC)
}

// Day is what the learner did on one calendar day.
type Day struct {
	Date       Date
	Minutes    int      // time spent studying
	Reviews    int      // cards graded
	Correct    int      // graded cards recalled (grade 3 or more)
	Failed     int      // graded cards forgotten
	Activities []string // finished activities, like "Lesson 2" or "Reading"
}

// Studied reports whether the day counts for the streak: at least one
// graded card or one finished activity. Time alone does not count, so
// opening the app and leaving it open does not keep a streak alive.
func (d Day) Studied() bool {
	return d.Reviews > 0 || len(d.Activities) > 0
}

// Level is a CEFR level, from A1 to C2.
type Level string

// The six CEFR levels.
const (
	A1 Level = "A1"
	A2 Level = "A2"
	B1 Level = "B1"
	B2 Level = "B2"
	C1 Level = "C1"
	C2 Level = "C2"
)

var levelNames = map[Level]string{
	A1: "Beginner", A2: "Elementary", B1: "Intermediate",
	B2: "Upper Intermediate", C1: "Advanced", C2: "Proficiency",
}

// ParseLevel reads a CEFR level like "A1", ignoring case and spaces.
func ParseLevel(s string) (Level, error) {
	l := Level(strings.ToUpper(strings.TrimSpace(s)))
	if _, ok := levelNames[l]; !ok {
		return "", fmt.Errorf("level %q is not a CEFR level (A1–C2)", s)
	}
	return l, nil
}

func (l Level) String() string { return string(l) }

// Name is the level's common name, like "Beginner" for A1.
func (l Level) Name() string { return levelNames[l] }

// DefaultDailyGoalMinutes is the daily study goal until the learner sets one.
const DefaultDailyGoalMinutes = 20

// Profile is who the learner is and what they aim for.
type Profile struct {
	Name             string // "" until the learner sets it
	Level            Level
	Started          Date // zero until known
	DailyGoalMinutes int
}

// NewProfile returns the profile of a new A1 learner with the default goal.
func NewProfile() Profile {
	return Profile{Level: A1, DailyGoalMinutes: DefaultDailyGoalMinutes}
}
