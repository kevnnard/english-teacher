package progress

import (
	"testing"

	"github.com/kevnnard/english-teacher/internal/domain"
)

func TestCountWords(t *testing.T) {
	tests := []struct {
		text string
		want int
	}{
		{"", 0},
		{"   \n\t ", 0},
		{"Hello!", 1},
		{"Ana is a teacher. She lives in Rosario.", 8},
		{"I don't know — maybe a well-known café?", 7}, // the dash alone is not a word
		{"She has 2 cats\nand 1 dog.", 7},
	}
	for _, tt := range tests {
		if got := CountWords(tt.text); got != tt.want {
			t.Errorf("CountWords(%q) = %d, want %d", tt.text, got, tt.want)
		}
	}
}

func TestWeekStartIsMonday(t *testing.T) {
	// today (2026-11-09) is a Monday.
	monday := today
	for i := range 7 {
		d := monday.AddDays(i)
		if got := WeekStart(d); got != monday {
			t.Errorf("WeekStart(%s, %s) = %s, want %s", d, d.Weekday(), got, monday)
		}
	}
	if got := WeekStart(monday.AddDays(-1)); got != monday.AddDays(-7) {
		t.Errorf("WeekStart(Sunday before) = %s, want the Monday before", got)
	}
}

func TestWordsReadThisWeekRunsMondayToSunday(t *testing.T) {
	monday := today
	sunday := monday.AddDays(6)
	days := []domain.Day{
		{Date: monday.AddDays(-1), WordsRead: 1000}, // the Sunday before: last week
		{Date: monday, WordsRead: 300},
		{Date: monday.AddDays(3), WordsRead: 250, Reviews: 9},
		{Date: sunday, WordsRead: 40},
		{Date: sunday.AddDays(1), WordsRead: 7}, // next Monday
	}
	tests := []struct {
		name  string
		today domain.Date
		want  int
	}{
		{"on Monday only Monday counts", monday, 300},
		{"on Sunday the whole week counts", sunday, 590},
		{"days after today do not count", monday.AddDays(3), 550},
		{"the next Monday starts again", sunday.AddDays(1), 7},
		{"the Sunday before is last week", monday.AddDays(-1), 1000},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := WordsReadThisWeek(days, tt.today); got != tt.want {
				t.Errorf("WordsReadThisWeek(%s) = %d, want %d", tt.today, got, tt.want)
			}
		})
	}
	if got := WordsReadThisWeek(nil, monday); got != 0 {
		t.Errorf("no days: %d, want 0", got)
	}
}

func TestAddSumsWordsRead(t *testing.T) {
	days := Add(nil, domain.Day{Date: today, WordsRead: 300})
	days = Add(days, domain.Day{Date: today, WordsRead: 250})
	if got := DayOf(days, today).WordsRead; got != 550 {
		t.Errorf("WordsRead = %d, want 550", got)
	}
}
