package srs

import (
	"strings"
	"time"

	"github.com/kevnnard/english-teacher/internal/domain"
)

// ProductionMinRepetitions is how many successful recognition reviews in a
// row a word needs before its production (typed) direction is studied:
// learners recognize a word before they can produce it.
const ProductionMinRepetitions = 2

// PlanSession picks the cards for one review sitting and returns their
// indices into cards: every reviewed card that is due, then at most maxNew
// never-reviewed cards, each group in the order given.
//
// A production card only takes part once the same word's recognition card
// has at least ProductionMinRepetitions repetitions.
func PlanSession(cards []domain.Card, now time.Time, maxNew int) []int {
	recognized := map[string]int{} // lowercased word -> recognition repetitions
	for _, c := range cards {
		if c.Direction == domain.Recognition {
			recognized[strings.ToLower(c.Word)] = c.Repetitions
		}
	}

	var due, fresh []int
	for i, c := range cards {
		switch {
		case c.Due.After(now):
			continue
		case c.Direction == domain.Production && recognized[strings.ToLower(c.Word)] < ProductionMinRepetitions:
			continue
		case c.LastReviewed == nil:
			if len(fresh) < maxNew {
				fresh = append(fresh, i)
			}
		default:
			due = append(due, i)
		}
	}
	return append(due, fresh...)
}

// NextReview finds the next day with reviews after now and how many cards are
// due that day. Cards never reviewed are not scheduled yet and are left out.
// ok is false when nothing is scheduled.
func NextReview(cards []domain.Card, now time.Time) (day time.Time, count int, ok bool) {
	for _, c := range cards {
		if c.LastReviewed == nil || !c.Due.After(now) {
			continue
		}
		d := startOfDay(c.Due.In(now.Location()))
		switch {
		case !ok || d.Before(day):
			day, count, ok = d, 1, true
		case d.Equal(day):
			count++
		}
	}
	return day, count, ok
}
