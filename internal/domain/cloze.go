package domain

import (
	"fmt"
	"strings"
)

// Grammar in context (spec S4): a cloze item is a sentence with a gap the
// learner fills by typing, and a round mixes several grammar points.
// Mixing them (interleaving) makes the learner decide which rule applies,
// instead of repeating the same rule ten times in a row.

// MinClozePoints is the fewest different grammar points a cloze round mixes.
const MinClozePoints = 3

// mistakeWeight is how many items a point the learner gets wrong in Write
// is worth, against 1 for every other point.
const mistakeWeight = 2

// ClozeItem is one gap-fill item: a sentence with one word or a few words
// hidden, the grammar point it practices and the rule behind it.
type ClozeItem struct {
	Sentence string // the whole sentence, with the answer in place
	Answer   string // the hidden word or words, as they appear in Sentence
	Hint     string // a short clue shown with the gap, such as the base verb "go"; may be ""
	Point    string // the grammar point, such as "past simple"; shown only after the answer
	Rule     string // the rule in one line, shown after the answer
	Chunk    Chunk  // a useful fixed phrase in the sentence; the zero Chunk means none
	Yours    bool   // the sentence comes from the learner's own cards or stories
}

// Chunk is a fixed phrase worth learning whole, such as "I'm looking for…"
// or "Could you…?". It becomes an ordinary Review card.
type Chunk struct {
	Phrase  string // the phrase, in English
	Meaning string // what it means, in Spanish
	Use     string // when to use it, in a few words
	Example string // one sentence with the phrase
}

// Valid reports whether c has a phrase and a meaning, the least a card needs.
func (c Chunk) Valid() bool {
	return strings.TrimSpace(c.Phrase) != "" && strings.TrimSpace(c.Meaning) != ""
}

// Card is c as a new card: the phrase on the front, and on the back its
// meaning and use, with the example.
func (c Chunk) Card() Card {
	card := NewCard(strings.TrimSpace(c.Phrase))
	card.Meaning = strings.TrimSpace(c.Meaning)
	if use := strings.TrimSpace(c.Use); use != "" {
		card.Meaning += " — " + use
	}
	card.Example = strings.TrimSpace(c.Example)
	return card
}

// Interleave chooses and orders up to n items for a round. It is pure.
//
// Items of the same point keep their order. Each next item comes from the
// point that has been used least so far, never from the point just asked,
// so the same point is never twice in a row. A point in mistakes, the
// labels of the learner's top Write errors, counts as mistakeWeight
// points: it is asked first and about twice as often. Ties go to mistakes
// first, in their order, then to the other points in the order they appear.
//
// Points are compared ignoring case and spaces around them; items with no
// point or no answer are dropped. The round stops early rather than repeat
// a point, and fewer than MinClozePoints different points is an error.
func Interleave(items []ClozeItem, mistakes []string, n int) ([]ClozeItem, error) {
	// groups holds the items of each point, in the order points come first.
	type group struct {
		items  []ClozeItem
		weight int
		used   int
	}
	var groups []*group
	byKey := map[string]*group{}
	for _, m := range mistakes {
		k := pointKey(m)
		if k != "" && byKey[k] == nil {
			byKey[k] = &group{weight: mistakeWeight}
			groups = append(groups, byKey[k])
		}
	}
	for _, it := range items {
		k := pointKey(it.Point)
		if k == "" || strings.TrimSpace(it.Answer) == "" {
			continue
		}
		if byKey[k] == nil {
			byKey[k] = &group{weight: 1}
			groups = append(groups, byKey[k])
		}
		byKey[k].items = append(byKey[k].items, it)
	}

	var round []ClozeItem
	var last *group
	for len(round) < n {
		var next *group
		for _, g := range groups {
			if g == last || g.used == len(g.items) {
				continue
			}
			// g is used less than next, relative to its weight.
			if next == nil || g.used*next.weight < next.used*g.weight {
				next = g
			}
		}
		if next == nil {
			break
		}
		round = append(round, next.items[next.used])
		next.used++
		last = next
	}

	mixed := 0
	for _, g := range groups {
		if g.used > 0 {
			mixed++
		}
	}
	if mixed < MinClozePoints {
		points := "points"
		if mixed == 1 {
			points = "point"
		}
		return nil, fmt.Errorf("the items cover only %d grammar %s; a round mixes at least %d", mixed, points, MinClozePoints)
	}
	return round, nil
}

// pointKey is how two points are compared: lowercase, without spaces around.
func pointKey(point string) string {
	return strings.ToLower(strings.TrimSpace(point))
}
