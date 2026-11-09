package tui

import (
	"errors"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/kevnnard/english-teacher/internal/domain"
)

var (
	today   = domain.DateOf(fixedNow)
	errTest = errors.New("disk full")
)

// studyLog is a fake Record function that remembers every delta.
type studyLog struct {
	deltas []domain.Day
	err    error
}

func (l *studyLog) record(d domain.Day) error {
	l.deltas = append(l.deltas, d)
	return l.err
}

// newProgressApp builds an app with a profile, past days and a study log.
func newProgressApp(t *testing.T, p domain.Profile, days []domain.Day, log *studyLog, start Screen) (App, *testClock) {
	t.Helper()
	clock := &testClock{t: fixedNow}
	a := NewApp(Config{
		Cards: deck(), Save: (&recorder{}).save, Now: clock.now, Start: start, MaxNew: DefaultMaxNew,
		Profile: p, Days: days, Record: log.record,
	})
	return send(t, a, tea.WindowSizeMsg{Width: 120, Height: 30}), clock
}

func studiedDay(offset int) domain.Day {
	return domain.Day{Date: today.AddDays(offset), Reviews: 3}
}

func TestStatusBarShowsTheProfileLevel(t *testing.T) {
	a, _ := newProgressApp(t, domain.Profile{Level: domain.B1, DailyGoalMinutes: 20}, nil, &studyLog{}, Home)

	mustContain(t, statusBar(a), "B1")
	mustNotContain(t, statusBar(a), "A1")
}

func TestZeroProfileMeansANewA1Learner(t *testing.T) {
	a, _ := newProgressApp(t, domain.Profile{}, nil, &studyLog{}, Home)

	mustContain(t, statusBar(a), "A1")
	mustContain(t, pageText(a), "0 of 20 min")
}

func TestStatusBarShowsTheStreak(t *testing.T) {
	a, _ := newProgressApp(t, domain.NewProfile(), []domain.Day{studiedDay(-1), studiedDay(-2)}, &studyLog{}, Home)
	mustContain(t, statusBar(a), "2-day streak")

	b, _ := newProgressApp(t, domain.NewProfile(), nil, &studyLog{}, Home)
	mustNotContain(t, statusBar(b), "streak")
}

func TestGradingRecordsTheDay(t *testing.T) {
	log := &studyLog{}
	a, _ := newProgressApp(t, domain.NewProfile(), nil, log, Review)

	a, _ = press(t, a, "space", "3") // hello: good
	a, _ = press(t, a, "space", "1") // hi: forgot

	if len(log.deltas) != 2 {
		t.Fatalf("recorded %d deltas, want 2: %+v", len(log.deltas), log.deltas)
	}
	if d := log.deltas[0]; d.Date != today || d.Reviews != 1 || d.Correct != 1 || d.Failed != 0 {
		t.Errorf("first delta = %+v", d)
	}
	if d := log.deltas[1]; d.Reviews != 1 || d.Correct != 0 || d.Failed != 1 {
		t.Errorf("second delta = %+v", d)
	}
	mustContain(t, statusBar(a), "1-day streak")
}

func TestUndoTakesTheGradeBackFromTheDay(t *testing.T) {
	log := &studyLog{}
	a, _ := newProgressApp(t, domain.NewProfile(), nil, log, Review)

	a, _ = press(t, a, "space", "1", "u")

	last := log.deltas[len(log.deltas)-1]
	if last.Reviews != -1 || last.Failed != -1 || last.Correct != 0 {
		t.Errorf("undo delta = %+v, want reviews -1 failed -1", last)
	}
	mustNotContain(t, statusBar(a), "streak")
}

func TestPracticeRoundRecordsNothing(t *testing.T) {
	log := &studyLog{}
	a, _ := newProgressApp(t, domain.NewProfile(), nil, log, Review)
	for range 5 { // finish the session, failing every card
		a, _ = press(t, a, "space", "1")
	}
	n := len(log.deltas)

	a, _ = press(t, a, "r", "space", "3")

	if len(log.deltas) != n {
		t.Errorf("the practice round recorded %+v", log.deltas[n:])
	}
}

func TestStudyMinutesAreCountedWithAnIdleCap(t *testing.T) {
	log := &studyLog{}
	a, clock := newProgressApp(t, domain.NewProfile(), nil, log, Review)

	clock.t = clock.t.Add(40 * time.Second)
	a, _ = press(t, a, "space", "3")
	clock.t = clock.t.Add(40 * time.Second)
	a, _ = press(t, a, "space", "3")
	clock.t = clock.t.Add(30 * time.Minute) // walked away
	a, _ = press(t, a, "space", "3")

	minutes := 0
	for _, d := range log.deltas {
		minutes += d.Minutes
	}
	if log.deltas[0].Minutes != 0 || log.deltas[1].Minutes != 1 {
		t.Errorf("deltas = %+v, want the first minute on the second grade", log.deltas)
	}
	if minutes != 3 { // 40 s + 40 s + at most 2 min
		t.Errorf("recorded %d minutes, want 3", minutes)
	}
	mustContain(t, pageText(send(t, a, key("esc"))), "3 of 20 min")
}

func TestRecordErrorStopsTheApp(t *testing.T) {
	log := &studyLog{err: errTest}
	a, _ := newProgressApp(t, domain.NewProfile(), nil, log, Review)

	a, _ = press(t, a, "space", "3")

	if a.Err() != errTest {
		t.Errorf("Err = %v, want the record error", a.Err())
	}
}

func TestHomeGreetsByNameOnlyWhenSet(t *testing.T) {
	named, _ := newProgressApp(t, domain.Profile{Name: "Kevin", Level: domain.A1, DailyGoalMinutes: 20}, nil, &studyLog{}, Home)
	mustContain(t, pageText(named), "Good evening, Kevin.")

	anon, _ := newProgressApp(t, domain.NewProfile(), nil, &studyLog{}, Home)
	mustContain(t, pageText(anon), "Good evening.")
}

func TestHomeShowsTodayGoalAndWeek(t *testing.T) {
	days := []domain.Day{{Date: today, Minutes: 12, Reviews: 4}, studiedDay(-2)}
	a, _ := newProgressApp(t, domain.Profile{Level: domain.A1, DailyGoalMinutes: 25}, days, &studyLog{}, Home)

	page := pageText(a)

	mustContain(t, page, "12 of 25 min", "Tu ○  We ○  Th ○  Fr ○  Sa ●  Su ○  Mo ●")
}

func TestStatsShowsStreakAndTheLastSevenDays(t *testing.T) {
	days := []domain.Day{
		{Date: today, Minutes: 5, Reviews: 8, Correct: 6, Failed: 2},
		studiedDay(-1),
		studiedDay(-10), studiedDay(-11), studiedDay(-12),
	}
	a, _ := newProgressApp(t, domain.NewProfile(), days, &studyLog{}, Stats)

	page := pageText(a)

	mustContain(t, page, "Streak", "2 days", "Longest", "3 days", "Last 7 days", "Mo 9 Nov", "5 min", "8 cards", "✓ 6", "✗ 2")
	if strings.Count(page, "Nov") < 7 {
		t.Errorf("want one row per day of the week:\n%s", page)
	}
}

func TestUndoAfterMidnightTakesBackTheDayOfTheGrade(t *testing.T) {
	log := &studyLog{}
	a, clock := newProgressApp(t, domain.NewProfile(), nil, log, Review)
	a, _ = press(t, a, "space", "3")

	clock.t = clock.t.Add(3 * time.Hour) // 00:30 the next day
	a, _ = press(t, a, "u")

	if last := log.deltas[len(log.deltas)-1]; last.Date != today || last.Reviews != -1 {
		t.Errorf("undo delta = %+v, want reviews -1 on %v", last, today)
	}
	mustNotContain(t, statusBar(a), "streak")
}

func TestHomeStreakLineGuidesEveryState(t *testing.T) {
	old := domain.Day{Date: domain.Date{Year: 2026, Month: time.August, Day: 14}, Activities: []string{"Lesson 1"}}
	tests := []struct {
		name string
		days []domain.Day
		want string
	}{
		{"never studied", nil, "Streak  0 days — review today to start one"},
		{"broken streak", []domain.Day{old}, "Streak  0 days · last studied 14 Aug — review today to start one"},
		{"alive from yesterday", []domain.Day{studiedDay(-1), studiedDay(-2)}, "Streak  2 days — review today to keep it"},
		{"done today", []domain.Day{studiedDay(0), studiedDay(-1), studiedDay(-5), studiedDay(-6), studiedDay(-7)}, "Streak  2 days ✓ today · best 3"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			a, _ := newProgressApp(t, domain.NewProfile(), tt.days, &studyLog{}, Home)
			mustContain(t, pageText(a), tt.want)
		})
	}
}
