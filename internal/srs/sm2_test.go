package srs

import (
	"math"
	"testing"
	"time"

	"github.com/kevnnard/english-teacher/internal/domain"
)

// now is a fixed review time; tests never depend on the real clock.
var now = time.Date(2026, 11, 9, 21, 30, 0, 0, time.UTC)

// today is the start of the day of now.
var today = time.Date(2026, 11, 9, 0, 0, 0, 0, time.UTC)

func approx(a, b float64) bool { return math.Abs(a-b) < 1e-9 }

func TestReviewIntervals(t *testing.T) {
	tests := []struct {
		name         string
		reps         int
		interval     int
		ef           float64
		grade        domain.Grade
		wantReps     int
		wantInterval int
	}{
		{name: "first success is 1 day", reps: 0, interval: 0, ef: 2.5, grade: 4, wantReps: 1, wantInterval: 1},
		{name: "second success is 6 days", reps: 1, interval: 1, ef: 2.5, grade: 4, wantReps: 2, wantInterval: 6},
		{name: "third success is interval*EF", reps: 2, interval: 6, ef: 2.5, grade: 4, wantReps: 3, wantInterval: 15},
		{name: "interval*EF is rounded", reps: 3, interval: 15, ef: 2.36, grade: 5, wantReps: 4, wantInterval: 35}, // 15*2.36 = 35.4
		{name: "rounding goes up at .5", reps: 3, interval: 5, ef: 1.3, grade: 3, wantReps: 4, wantInterval: 7},    // 5*1.3 = 6.5
		{name: "grade 3 still passes", reps: 0, interval: 0, ef: 2.5, grade: 3, wantReps: 1, wantInterval: 1},
		{name: "grade 2 resets", reps: 5, interval: 40, ef: 2.5, grade: 2, wantReps: 0, wantInterval: 1},
		{name: "grade 0 resets", reps: 2, interval: 6, ef: 2.5, grade: 0, wantReps: 0, wantInterval: 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := domain.NewCard("word")
			c.Repetitions, c.IntervalDays, c.EaseFactor = tt.reps, tt.interval, tt.ef

			got := Review(c, tt.grade, now)

			if got.Repetitions != tt.wantReps {
				t.Errorf("Repetitions = %d, want %d", got.Repetitions, tt.wantReps)
			}
			if got.IntervalDays != tt.wantInterval {
				t.Errorf("IntervalDays = %d, want %d", got.IntervalDays, tt.wantInterval)
			}
			wantDue := today.AddDate(0, 0, tt.wantInterval)
			if !got.Due.Equal(wantDue) {
				t.Errorf("Due = %v, want %v", got.Due, wantDue)
			}
		})
	}
}

func TestReviewEaseFactor(t *testing.T) {
	tests := []struct {
		grade domain.Grade
		ef    float64
		want  float64
	}{
		{grade: 5, ef: 2.5, want: 2.6},
		{grade: 4, ef: 2.5, want: 2.5},
		{grade: 3, ef: 2.5, want: 2.36},
		{grade: 2, ef: 2.5, want: 2.18},
		{grade: 1, ef: 2.5, want: 1.96},
		{grade: 0, ef: 2.5, want: 1.7},
		{grade: 0, ef: 1.4, want: 1.3}, // 1.4 - 0.8 = 0.6, clamped
		{grade: 3, ef: 1.3, want: 1.3}, // 1.3 - 0.14 = 1.16, clamped
	}
	for _, tt := range tests {
		c := domain.NewCard("word")
		c.EaseFactor = tt.ef

		got := Review(c, tt.grade, now)

		if !approx(got.EaseFactor, tt.want) {
			t.Errorf("grade %d, EF %v: EaseFactor = %v, want %v", tt.grade, tt.ef, got.EaseFactor, tt.want)
		}
	}
}

func TestReviewNeverBelowMinEase(t *testing.T) {
	c := domain.NewCard("word")
	for i := 0; i < 20; i++ {
		c = Review(c, 0, now)
		if c.EaseFactor < MinEaseFactor {
			t.Fatalf("after %d failures EaseFactor = %v, below %v", i+1, c.EaseFactor, MinEaseFactor)
		}
	}
}

func TestReviewCounters(t *testing.T) {
	tests := []struct {
		grade       domain.Grade
		wantCorrect int
		wantFailed  int
	}{
		{grade: 0, wantCorrect: 3, wantFailed: 2},
		{grade: 2, wantCorrect: 3, wantFailed: 2},
		{grade: 3, wantCorrect: 4, wantFailed: 1},
		{grade: 5, wantCorrect: 4, wantFailed: 1},
	}
	for _, tt := range tests {
		c := domain.NewCard("word")
		c.TimesCorrect, c.TimesFailed = 3, 1

		got := Review(c, tt.grade, now)

		if got.TimesCorrect != tt.wantCorrect || got.TimesFailed != tt.wantFailed {
			t.Errorf("grade %d: correct=%d failed=%d, want correct=%d failed=%d",
				tt.grade, got.TimesCorrect, got.TimesFailed, tt.wantCorrect, tt.wantFailed)
		}
	}
}

func TestReviewSetsLastReviewed(t *testing.T) {
	got := Review(domain.NewCard("word"), 4, now)

	if got.LastReviewed == nil || !got.LastReviewed.Equal(now) {
		t.Errorf("LastReviewed = %v, want %v", got.LastReviewed, now)
	}
}

func TestReviewDoesNotMutateInput(t *testing.T) {
	c := domain.NewCard("word")

	_ = Review(c, 5, now)

	if c.Repetitions != 0 || c.TimesCorrect != 0 || c.LastReviewed != nil {
		t.Errorf("input card was mutated: %+v", c)
	}
}

func TestReviewClampsInvalidGrade(t *testing.T) {
	tests := []struct {
		in   domain.Grade
		same domain.Grade
	}{
		{in: -3, same: 0},
		{in: 9, same: 5},
	}
	for _, tt := range tests {
		c := domain.NewCard("word")
		got, want := Review(c, tt.in, now), Review(c, tt.same, now)
		if got.EaseFactor != want.EaseFactor || got.IntervalDays != want.IntervalDays {
			t.Errorf("Review with grade %d = %+v, want same as grade %d = %+v", tt.in, got, tt.same, want)
		}
	}
}

func TestDue(t *testing.T) {
	cards := []domain.Card{
		{Word: "never-scheduled"},                         // zero Due: due now
		{Word: "yesterday", Due: today.AddDate(0, 0, -1)}, // overdue
		{Word: "today", Due: today},                       // due today
		{Word: "tomorrow", Due: today.AddDate(0, 0, 1)},   // not yet
	}

	got := Due(cards, now)

	want := []string{"never-scheduled", "yesterday", "today"}
	if len(got) != len(want) {
		t.Fatalf("Due returned %d cards, want %d: %+v", len(got), len(want), got)
	}
	for i, w := range want {
		if got[i].Word != w {
			t.Errorf("Due[%d] = %q, want %q", i, got[i].Word, w)
		}
	}
}

func TestDueEmpty(t *testing.T) {
	if got := Due(nil, now); len(got) != 0 {
		t.Errorf("Due(nil) = %v, want empty", got)
	}
}
