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

	// Added is the day the word was added to the deck. It is the same in
	// both directions, and zero when unknown: words imported or added
	// before the app kept the date.
	Added Date
	// KnownSince is the day this direction's interval last reached
	// KnownIntervalDays, so the word became known. It is zero while the
	// card is not known, and also when it became known before the app
	// kept the date.
	KnownSince Date
}

// Known reports whether c's interval is long enough for the word to count
// as known: KnownIntervalDays (21) days or more.
func (c Card) Known() bool {
	return c.IntervalDays >= KnownIntervalDays
}

// TrackKnown returns after, the card as a review on day on left it, with
// KnownSince kept up to date against before, the card as it was:
//
//   - not known before and known after: known since on;
//   - known before and after: the first date stays (zero stays zero);
//   - not known after: zero, the word is no longer known.
func TrackKnown(before, after Card, on Date) Card {
	switch {
	case !after.Known():
		after.KnownSince = Date{}
	case !before.Known():
		after.KnownSince = on
	default:
		after.KnownSince = before.KnownSince
	}
	return after
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
