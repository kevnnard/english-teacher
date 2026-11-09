package progress

import (
	"testing"
	"time"

	"github.com/kevnnard/english-teacher/internal/domain"
)

var today = domain.Date{Year: 2026, Month: time.November, Day: 9}

// studied returns a studied day n days before today.
func studied(n int) domain.Day {
	return domain.Day{Date: today.AddDays(-n), Reviews: 1}
}

func TestStreak(t *testing.T) {
	tests := []struct {
		name             string
		days             []domain.Day
		current, longest int
	}{
		{"no days", nil, 0, 0},
		{"only today", []domain.Day{studied(0)}, 1, 1},
		{"yesterday keeps the streak alive", []domain.Day{studied(1), studied(2)}, 2, 2},
		{"a gap of two days breaks it", []domain.Day{studied(2), studied(3), studied(4)}, 0, 3},
		{"current and an older longer run", []domain.Day{studied(0), studied(1), studied(5), studied(6), studied(7)}, 2, 3},
		{"order does not matter", []domain.Day{studied(1), studied(0), studied(2)}, 3, 3},
		{"days without study do not count", []domain.Day{studied(0), {Date: today.AddDays(-1), Minutes: 5}, studied(2)}, 1, 1},
		{"future days are ignored", []domain.Day{{Date: today.AddDays(1), Reviews: 1}, studied(0)}, 1, 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := ComputeStreak(tt.days, today)
			if s.Current != tt.current || s.Longest != tt.longest {
				t.Errorf("ComputeStreak = %+v, want current %d longest %d", s, tt.current, tt.longest)
			}
		})
	}
}

func TestStreakStudiedToday(t *testing.T) {
	if ComputeStreak([]domain.Day{studied(1)}, today).StudiedToday {
		t.Error("yesterday only: StudiedToday should be false")
	}
	if !ComputeStreak([]domain.Day{studied(0)}, today).StudiedToday {
		t.Error("StudiedToday should be true")
	}
}

func TestWeekIsTheLastSevenDaysEndingToday(t *testing.T) {
	days := []domain.Day{studied(0), {Date: today.AddDays(-3), Minutes: 12, Reviews: 4}, studied(9)}

	w := Week(days, today)

	if w[6].Date != today || w[0].Date != today.AddDays(-6) {
		t.Fatalf("week runs %v..%v, want %v..%v", w[0].Date, w[6].Date, today.AddDays(-6), today)
	}
	if w[3].Minutes != 12 || w[3].Reviews != 4 {
		t.Errorf("w[3] = %+v, want the day three days ago", w[3])
	}
	if w[1].Studied() {
		t.Errorf("a missing day should be empty, got %+v", w[1])
	}
}

func TestDayOfFindsOrCreatesToday(t *testing.T) {
	days := []domain.Day{{Date: today, Minutes: 7}}

	if d := DayOf(days, today); d.Minutes != 7 {
		t.Errorf("DayOf = %+v", d)
	}
	if d := DayOf(nil, today); d.Date != today || d.Studied() {
		t.Errorf("DayOf(nil) = %+v, want an empty today", d)
	}
}
