package progress

import (
	"reflect"
	"testing"
	"time"

	"github.com/kevnnard/english-teacher/internal/domain"
)

// Weeks of the fixed data: today is Wednesday 2026-11-11, so this week
// runs from Monday 9 and last week from Monday 2 to Sunday 8.
var (
	wed11 = domain.Date{Year: 2026, Month: time.November, Day: 11}
	mon9  = domain.Date{Year: 2026, Month: time.November, Day: 9}
	sun8  = domain.Date{Year: 2026, Month: time.November, Day: 8}
	mon2  = domain.Date{Year: 2026, Month: time.November, Day: 2}
	sun1  = domain.Date{Year: 2026, Month: time.November, Day: 1}
	thu12 = domain.Date{Year: 2026, Month: time.November, Day: 12}
)

func weekCard(word string, dir domain.Direction, added, known domain.Date, interval int) domain.Card {
	return domain.Card{Word: word, Direction: dir, Added: added, KnownSince: known, IntervalDays: interval}
}

func weekData() WeekData {
	return WeekData{
		Days: []domain.Day{
			{Date: sun1, Minutes: 99, Reviews: 9, WordsRead: 999, Activities: []string{"Writing: 10/40"}}, // two weeks ago
			{Date: mon2, Minutes: 10, Reviews: 4},
			{Date: sun8, Minutes: 20, WordsRead: 300, Activities: []string{"Writing: 30/40"}},
			{Date: mon9, Minutes: 15, Reviews: 6, WordsRead: 250, Activities: []string{"Writing: 33/40", "Talk: Order a coffee"}},
			{Date: wed11, Minutes: 25, WordsRead: 400, Activities: []string{"Writing: 36/40"}},
			{Date: mon9.AddDays(1), Minutes: 5},    // time only: not a study day
			{Date: thu12, Minutes: 50, Reviews: 3}, // after today: ignored
		},
		Cards: []domain.Card{
			weekCard("look for", domain.Recognition, mon9, domain.Date{}, 1),
			weekCard("look for", domain.Production, mon9, domain.Date{}, 1), // the same word
			weekCard("apple", domain.Recognition, wed11, domain.Date{}, 0),
			weekCard("house", domain.Recognition, sun8, mon9, 25),
			weekCard("dog", domain.Recognition, mon2, sun8, 22),
			weekCard("cat", domain.Production, mon2, mon9, 30),                    // production only: not counted
			weekCard("old", domain.Recognition, domain.Date{}, domain.Date{}, 40), // no dates
		},
		Mistakes: []domain.Mistake{
			{Date: mon9, Kind: "article"}, {Date: mon9, Kind: "article"}, {Date: wed11, Kind: "article"},
			{Date: sun8, Kind: "article"}, {Date: mon2, Kind: "article"}, {Date: mon2, Kind: "article"},
			{Date: mon2, Kind: "article"}, {Date: mon2, Kind: "article"},
			{Date: wed11, Kind: "be"}, {Date: mon9, Kind: "be"}, {Date: sun8, Kind: "be"},
			{Date: mon9, Kind: "tense"}, {Date: sun8, Kind: "tense"},
			{Date: mon9, Kind: "spelling"},
			{Date: sun1, Kind: "word order"}, {Date: sun1, Kind: "word order"}, {Date: sun1, Kind: "word order"},
			{Date: thu12, Kind: "be"},
		},
	}
}

func TestWeeklySummaryNumbers(t *testing.T) {
	s := WeeklySummary(weekData(), wed11)
	want := struct{ this, last WeekStats }{
		this: WeekStats{Start: mon9, End: wed11, Minutes: 45, DaysStudied: 2, NewCards: 2, NewKnown: 1,
			WordsRead: 650, Texts: 2, WritingSum: 69},
		last: WeekStats{Start: mon2, End: sun8, Minutes: 30, DaysStudied: 2, NewCards: 2, NewKnown: 1,
			WordsRead: 300, Texts: 1, WritingSum: 30},
	}
	if s.This != want.this {
		t.Errorf("This = %+v\nwant   %+v", s.This, want.this)
	}
	if s.Last != want.last {
		t.Errorf("Last = %+v\nwant   %+v", s.Last, want.last)
	}
	if avg, ok := s.This.WritingAverage(); !ok || avg != 34.5 {
		t.Errorf("this week's Write average = %v, %v; want 34.5", avg, ok)
	}
	if _, ok := (WeekStats{}).WritingAverage(); ok {
		t.Error("a week with no texts has a Write average")
	}
	if s.Empty() {
		t.Error("Empty with data")
	}
}

func TestWeeklySummaryOnASundayAndAMonday(t *testing.T) {
	data := weekData()
	// On Sunday 8 the week is Monday 2 to Sunday 8 and today is included.
	if s := WeeklySummary(data, sun8); s.This.Start != mon2 || s.This.Minutes != 30 || s.Last.Minutes != 99 {
		t.Errorf("on Sunday: This %+v, Last %+v", s.This, s.Last)
	}
	// On Monday 9 a new week starts: only that day is this week.
	if s := WeeklySummary(data, mon9); s.This.Minutes != 15 || s.Last.Minutes != 30 {
		t.Errorf("on Monday: This %+v, Last %+v", s.This, s.Last)
	}
}

func TestWeeklySummaryTopErrors(t *testing.T) {
	s := WeeklySummary(weekData(), wed11)
	want := []ErrorTrend{
		{Kind: "article", This: 3, Last: 5},
		{Kind: "be", This: 2, Last: 1},
		{Kind: "tense", This: 1, Last: 1},
	}
	if !reflect.DeepEqual(s.Errors, want) {
		t.Errorf("Errors = %+v, want %+v", s.Errors, want)
	}
	for i, trend := range []string{Falling, Rising, Same} {
		if got := s.Errors[i].Trend(); got != trend {
			t.Errorf("%s: Trend = %q, want %q", s.Errors[i].Kind, got, trend)
		}
	}
}

func TestWeeklySummaryWithNoData(t *testing.T) {
	data := WeekData{
		Days:  []domain.Day{{Date: sun1, Minutes: 40, Reviews: 3}}, // two weeks ago only
		Cards: []domain.Card{weekCard("old", domain.Recognition, domain.Date{}, domain.Date{}, 40)},
	}
	s := WeeklySummary(data, wed11)
	if !s.Empty() {
		t.Errorf("Empty = false for %+v", s)
	}
	if s.Errors != nil && len(s.Errors) != 0 {
		t.Errorf("Errors = %v, want none", s.Errors)
	}
	if NoWeekData != "Nothing recorded this week or last week yet." {
		t.Errorf("NoWeekData = %q", NoWeekData)
	}
}
