// Package domain holds the pure types of the English teacher: no I/O, no UI.
package domain

import (
	"fmt"
	"time"
)

// DefaultEaseFactor is the SM-2 ease factor every new card starts with.
const DefaultEaseFactor = 2.5

// Card is one vocabulary item studied in one direction, together with the
// spaced-repetition schedule of that direction. The same word has one Card
// per direction; Word is the identity, compared case-insensitively.
type Card struct {
	Word          string
	Pronunciation string
	Meaning       string
	Example       string

	// Direction is Recognition for new cards (the zero value).
	Direction Direction
	// LastWrong is the last wrong answer the learner typed in production, or "".
	LastWrong string

	TimesCorrect int
	TimesFailed  int
	// LastReviewed is nil when the card has never been reviewed.
	LastReviewed *time.Time

	// SM-2 schedule.
	EaseFactor   float64
	IntervalDays int
	Repetitions  int
	// Due is when the card should be reviewed next. The zero time means "due now".
	Due time.Time
}

// NewCard returns a card for word with a fresh SM-2 schedule.
func NewCard(word string) Card {
	return Card{Word: word, EaseFactor: DefaultEaseFactor}
}

// Grade is how well the learner recalled a card, from 0 (blackout) to 5 (perfect).
type Grade int

// Lowest and highest valid grades.
const (
	MinGrade Grade = 0
	MaxGrade Grade = 5
)

// NewGrade converts n to a Grade, returning an error when n is outside 0–5.
func NewGrade(n int) (Grade, error) {
	g := Grade(n)
	if !g.Valid() {
		return 0, fmt.Errorf("grade %d out of range %d-%d", n, MinGrade, MaxGrade)
	}
	return g, nil
}

// Valid reports whether g is between 0 and 5.
func (g Grade) Valid() bool {
	return g >= MinGrade && g <= MaxGrade
}

// Passed reports whether g counts as a correct recall (3 or more).
func (g Grade) Passed() bool {
	return g >= 3
}
