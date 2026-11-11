package progress

import (
	"slices"
	"strings"

	"github.com/kevnnard/english-teacher/internal/domain"
)

// This file is the weekly summary (spec S5): this week against last week.
// Weeks run Monday to Sunday, like WordsReadThisWeek; "this week" ends
// today, and days after today are ignored.

// Trend words of an error kind, this week against last week.
const (
	Falling = "falling" // fewer errors this week
	Rising  = "rising"  // more errors this week
	Same    = "same"    // as many as last week
)

// TopErrors is how many error kinds the summary lists.
const TopErrors = 3

// NoWeekData is what the summary says when neither week has anything.
const NoWeekData = "Nothing recorded this week or last week yet."

// WeekData is the stored data the weekly summary reads.
type WeekData struct {
	Days     []domain.Day     // recorded study days, in any order
	Cards    []domain.Card    // every card; a word counts once, by its recognition card
	Mistakes []domain.Mistake // every logged major error, with its date
}

// WeekStats are the numbers of one week, from Start to End, both included.
type WeekStats struct {
	Start, End  domain.Date
	Minutes     int // study time
	DaysStudied int // days that count for the streak (domain.Day.Studied)
	NewCards    int // words added to the deck (only words with a stored date)
	NewKnown    int // words that became known (only words with a stored date)
	WordsRead   int // words of the story chapters finished
	Texts       int // Write rubrics recorded
	WritingSum  int // the sum of those rubrics, out of WritingMax each
}

// WritingAverage is the average Write rubric of the week, out of
// WritingMax; ok is false when the week has no texts.
func (w WeekStats) WritingAverage() (avg float64, ok bool) {
	if w.Texts == 0 {
		return 0, false
	}
	return float64(w.WritingSum) / float64(w.Texts), true
}

// empty reports whether the week has no numbers at all.
func (w WeekStats) empty() bool {
	return w.Minutes == 0 && w.DaysStudied == 0 && w.NewCards == 0 && w.NewKnown == 0 &&
		w.WordsRead == 0 && w.Texts == 0
}

// ErrorTrend is how many errors of one kind were logged this week and
// last week.
type ErrorTrend struct {
	Kind       string
	This, Last int
}

// Trend is Falling, Rising or Same.
func (e ErrorTrend) Trend() string {
	switch {
	case e.This < e.Last:
		return Falling
	case e.This > e.Last:
		return Rising
	}
	return Same
}

// WeekSummary is this week against last week.
type WeekSummary struct {
	This, Last WeekStats
	// Errors are up to TopErrors kinds with the most errors over both
	// weeks together, most first; ties go by name.
	Errors []ErrorTrend
}

// Empty reports whether neither week has any data: then the summary says
// NoWeekData instead of showing zeros.
func (s WeekSummary) Empty() bool {
	return s.This.empty() && s.Last.empty() && len(s.Errors) == 0
}

// WeeklySummary computes the summary of today's week against the week
// before. It is pure: today is passed in.
func WeeklySummary(data WeekData, today domain.Date) WeekSummary {
	monday := WeekStart(today)
	return WeekSummary{
		This:   weekStats(data, monday, today),
		Last:   weekStats(data, monday.AddDays(-7), monday.AddDays(-1)),
		Errors: errorTrends(data.Mistakes, monday, today),
	}
}

// within reports whether d is between from and to, both included.
func within(d, from, to domain.Date) bool {
	return !d.Before(from) && !to.Before(d)
}

func weekStats(data WeekData, from, to domain.Date) WeekStats {
	w := WeekStats{Start: from, End: to}
	var week []domain.Day
	for _, d := range data.Days {
		if !within(d.Date, from, to) {
			continue
		}
		week = append(week, d)
		w.Minutes += d.Minutes
		w.WordsRead += d.WordsRead
		if d.Studied() {
			w.DaysStudied++
		}
	}
	for _, s := range ScoresOf(week, WritingPrefix) {
		w.Texts++
		w.WritingSum += s.Right
	}
	added, known := map[string]bool{}, map[string]bool{}
	for _, c := range data.Cards {
		if c.Direction != domain.Recognition {
			continue
		}
		word := strings.ToLower(c.Word)
		if c.Added != (domain.Date{}) && within(c.Added, from, to) {
			added[word] = true
		}
		if c.Known() && c.KnownSince != (domain.Date{}) && within(c.KnownSince, from, to) {
			known[word] = true
		}
	}
	w.NewCards, w.NewKnown = len(added), len(known)
	return w
}

// errorTrends counts the mistakes of each kind in the week from monday to
// today and in the week before, and keeps the TopErrors biggest kinds.
func errorTrends(mistakes []domain.Mistake, monday, today domain.Date) []ErrorTrend {
	byKind := map[string]*ErrorTrend{}
	for _, m := range mistakes {
		var count *int
		e := byKind[m.Kind]
		if e == nil {
			e = &ErrorTrend{Kind: m.Kind}
		}
		switch {
		case within(m.Date, monday, today):
			count = &e.This
		case within(m.Date, monday.AddDays(-7), monday.AddDays(-1)):
			count = &e.Last
		default:
			continue
		}
		*count++
		byKind[m.Kind] = e
	}
	trends := make([]ErrorTrend, 0, len(byKind))
	for _, e := range byKind {
		trends = append(trends, *e)
	}
	slices.SortFunc(trends, func(a, b ErrorTrend) int {
		if d := (b.This + b.Last) - (a.This + a.Last); d != 0 {
			return d
		}
		return strings.Compare(a.Kind, b.Kind)
	})
	return trends[:min(len(trends), TopErrors)]
}
