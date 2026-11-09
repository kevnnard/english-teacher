package domain

import "fmt"

// Direction says which way a word is studied. Each direction has its own
// schedule, because recognizing a word is easier than producing it.
type Direction int

const (
	Recognition Direction = iota // English shown, recall the Spanish meaning
	Production                   // Spanish shown, type the English
)

func (d Direction) String() string {
	switch d {
	case Recognition:
		return "recognition"
	case Production:
		return "production"
	}
	return fmt.Sprintf("Direction(%d)", int(d))
}

// ParseDirection is the inverse of Direction.String.
func ParseDirection(s string) (Direction, error) {
	switch s {
	case "recognition":
		return Recognition, nil
	case "production":
		return Production, nil
	}
	return 0, fmt.Errorf("unknown direction %q", s)
}
