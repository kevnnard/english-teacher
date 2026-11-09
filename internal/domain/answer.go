package domain

import "fmt"

// Answer is how the learner rates a recall on the 1–4 keys. It is a simpler
// scale than SM-2's 0–5 grades; Grade converts between the two.
type Answer int

const (
	Again Answer = iota + 1 // forgot
	Hard                    // recalled with effort
	Good                    // recalled
	Easy                    // recalled instantly
)

// NewAnswer converts n to an Answer, returning an error when n is outside 1–4.
func NewAnswer(n int) (Answer, error) {
	a := Answer(n)
	if a < Again || a > Easy {
		return 0, fmt.Errorf("answer %d out of range %d-%d", n, Again, Easy)
	}
	return a, nil
}

// Grade maps the answer to the SM-2 grade the scheduler uses:
// again 1, hard 3, good 4, easy 5. Only "again" is a failed recall.
func (a Answer) Grade() Grade {
	switch a {
	case Again:
		return 1
	case Hard:
		return 3
	case Good:
		return 4
	case Easy:
		return 5
	}
	return 0
}

func (a Answer) String() string {
	switch a {
	case Again:
		return "again"
	case Hard:
		return "hard"
	case Good:
		return "good"
	case Easy:
		return "easy"
	}
	return fmt.Sprintf("Answer(%d)", int(a))
}
