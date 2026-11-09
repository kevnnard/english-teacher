package domain

import "time"

// Status is where a card stands in the learner's study, for lists and counts.
type Status int

const (
	StatusNew      Status = iota // never reviewed
	StatusDue                    // reviewed before and due now
	StatusLearning               // scheduled, with a short interval
	StatusKnown                  // scheduled, with an interval of KnownIntervalDays or more
)

// KnownIntervalDays is the interval from which a card counts as known.
// Anki calls cards with an interval of 21 days or more "mature".
const KnownIntervalDays = 21

func (s Status) String() string {
	switch s {
	case StatusNew:
		return "new"
	case StatusDue:
		return "due"
	case StatusLearning:
		return "learning"
	case StatusKnown:
		return "known"
	}
	return "unknown"
}

// Status reports where c stands at now. A due card is "due" even when its
// interval is long: what matters is that it needs a review.
func (c Card) Status(now time.Time) Status {
	switch {
	case c.LastReviewed == nil:
		return StatusNew
	case !c.Due.After(now):
		return StatusDue
	case c.IntervalDays >= KnownIntervalDays:
		return StatusKnown
	default:
		return StatusLearning
	}
}
