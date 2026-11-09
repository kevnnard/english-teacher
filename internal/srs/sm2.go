// Package srs implements the SM-2 spaced-repetition algorithm.
//
// Everything here is pure: the current time is always passed in.
package srs

import (
	"math"
	"time"

	"github.com/kevnnard/english-teacher/internal/domain"
)

// MinEaseFactor is the lowest ease factor SM-2 allows.
const MinEaseFactor = 1.3

// Review returns c rescheduled after the learner answered with grade g at now.
// The input card is not modified. Grades outside 0–5 are clamped.
//
// The next due date is the start of now's day plus the new interval, so a card
// scheduled for "1 day" is due at any time tomorrow.
func Review(c domain.Card, g domain.Grade, now time.Time) domain.Card {
	g = clamp(g)

	if g.Passed() {
		switch c.Repetitions {
		case 0:
			c.IntervalDays = 1
		case 1:
			c.IntervalDays = 6
		default:
			// Uses the ease factor from before this review, as in the original SM-2.
			c.IntervalDays = int(math.Round(float64(c.IntervalDays) * c.EaseFactor))
		}
		c.Repetitions++
		c.TimesCorrect++
	} else {
		c.Repetitions = 0
		c.IntervalDays = 1
		c.TimesFailed++
	}

	c.EaseFactor = nextEase(c.EaseFactor, g)

	reviewed := now
	c.LastReviewed = &reviewed
	c.Due = startOfDay(now).AddDate(0, 0, c.IntervalDays)
	return c
}

// Due returns the cards whose due date is at or before now, keeping their order.
func Due(cards []domain.Card, now time.Time) []domain.Card {
	var due []domain.Card
	for _, c := range cards {
		if !c.Due.After(now) {
			due = append(due, c)
		}
	}
	return due
}

// nextEase applies EF' = EF + (0.1 − (5−g)(0.08 + (5−g)·0.02)), never below MinEaseFactor.
func nextEase(ef float64, g domain.Grade) float64 {
	q := float64(5 - g)
	ef += 0.1 - q*(0.08+q*0.02)
	return math.Max(ef, MinEaseFactor)
}

func clamp(g domain.Grade) domain.Grade {
	return min(max(g, domain.MinGrade), domain.MaxGrade)
}

func startOfDay(t time.Time) time.Time {
	y, m, d := t.Date()
	return time.Date(y, m, d, 0, 0, 0, 0, t.Location())
}
