package domain

import "slices"

// Minimal pairs (spec S5): two real English words that differ in one
// sound, such as ship and sheep. Spanish has no such difference, so a
// Spanish speaker hears both words as one until the ear learns it.
// Hearing one word and choosing which it was trains that ear.

// MinimalPair is two words that differ only in the sound of their
// contrast; the learner presses 1 for the first and 2 for the second.
type MinimalPair [2]string

// Contrast is one difference of sounds hard for Spanish speakers, with
// the pairs that train it.
type Contrast struct {
	ID    string        // stable key, stored with the learner's results
	Label string        // the two sounds, in Spanish, for the learner
	Tip   string        // how to make the difference, in Spanish
	Pairs []MinimalPair // at least three real-word pairs
}

// contrasts are the built-in contrasts, in the order a round uses to
// break ties. Every word is a real English word.
var contrasts = []Contrast{
	{"i-ee", "i corta / i larga",
		"ship: i corta y relajada, casi una e. sheep: i larga y tensa, como sonriendo.",
		[]MinimalPair{{"ship", "sheep"}, {"live", "leave"}, {"sit", "seat"}, {"fill", "feel"}}},
	{"u-oo", "u corta / u larga",
		"full: u corta y floja, labios poco redondos. fool: u larga, labios redondos y tensos.",
		[]MinimalPair{{"full", "fool"}, {"pull", "pool"}, {"look", "Luke"}}},
	{"a-u", "a de cat / a de cut",
		"cat: abre mucho la boca, entre a y e. cut: una a corta y floja, en el centro de la boca.",
		[]MinimalPair{{"cat", "cut"}, {"hat", "hut"}, {"bag", "bug"}, {"cap", "cup"}}},
	{"b-v", "b / v",
		"b: junta los dos labios. v: los dientes de arriba tocan el labio de abajo y el aire vibra.",
		[]MinimalPair{{"bat", "vat"}, {"berry", "very"}, {"ban", "van"}, {"best", "vest"}}},
	{"th-s", "th / s",
		"th de think: la punta de la lengua sale entre los dientes y sopla. s: la lengua queda detrás de los dientes.",
		[]MinimalPair{{"think", "sink"}, {"thick", "sick"}, {"path", "pass"}, {"mouth", "mouse"}}},
	{"th-d", "th suave / d",
		"th de they: la lengua entre los dientes, suave y con voz. d: la lengua toca detrás de los dientes de arriba.",
		[]MinimalPair{{"they", "day"}, {"then", "den"}, {"though", "dough"}}},
	{"ed", "verbo / verbo + -ed",
		"Escucha el final: -ed suena t (worked), d (played) o id tras t y d (wanted). Nunca \"e-d\".",
		[]MinimalPair{{"work", "worked"}, {"walk", "walked"}, {"play", "played"}, {"open", "opened"}}},
	{"s-cluster", "s + consonante / e + s",
		"state empieza con la s, sin e delante: \"sss-tate\". estate sí empieza con e.",
		[]MinimalPair{{"state", "estate"}, {"steam", "esteem"}, {"special", "especial"}}},
}

// Contrasts returns the built-in contrasts. It is a copy, so a caller
// may change it freely.
func Contrasts() []Contrast {
	out := make([]Contrast, len(contrasts))
	for i, c := range contrasts {
		c.Pairs = slices.Clone(c.Pairs)
		out[i] = c
	}
	return out
}

// PairStat is how the learner did on one contrast: in total, as stored,
// or in one round.
type PairStat struct {
	Contrast string // the Contrast ID
	Attempts int    // words heard
	Correct  int    // words chosen right
}

// Accuracy is the share of right answers, from 0 to 1; 0 when unseen.
func (s PairStat) Accuracy() float64 {
	if s.Attempts == 0 {
		return 0
	}
	return float64(s.Correct) / float64(s.Attempts)
}

// PairWeight is how much practice a contrast needs, from 1 to 10:
//
//	weight = 1 + 9 × (attempts − correct + 1) / (attempts + 2)
//
// The fraction is the error rate with one wrong and one right answer
// added, so an unseen contrast counts as half wrong (weight 5.5) and two
// lucky answers do not make a contrast look mastered.
func PairWeight(s PairStat) float64 {
	wrong := max(s.Attempts-s.Correct, 0)
	return 1 + 9*float64(wrong+1)/float64(s.Attempts+2)
}

// PairItem is one item of a round: the word played and the pair it
// belongs to.
type PairItem struct {
	Contrast string      // the Contrast ID
	Pair     MinimalPair // the two words on screen, as 1 and 2
	Played   int         // which word is played: 0 for 1, 1 for 2
}

// Word is the word played.
func (it PairItem) Word() string { return it.Pair[it.Played] }

// minPairContrasts is the fewest contrasts a round mixes, when there are
// that many.
const minPairContrasts = 3

// PairRound picks n items for a round. It is pure: intn(k) must return
// a number in [0, k), and the same stats and intn give the same round.
//
// The weakest contrasts come first and get the most items:
//
//  1. Contrasts are ordered by PairWeight, highest (weakest) first; ties
//     keep the order of contrasts.
//  2. The three weakest get one item each, so every round mixes at least
//     three contrasts.
//  3. The other items are shared by all contrasts in proportion to their
//     weight, by largest remainder (ties to the weaker contrast).
//  4. Items go round robin in that order: one of the weakest, one of the
//     next, and so on, skipping contrasts whose share is used up.
//
// Each contrast's pairs come in a shuffled order, and intn chooses which
// word of the pair plays.
func PairRound(contrasts []Contrast, stats map[string]PairStat, n int, intn func(int) int) []PairItem {
	if len(contrasts) == 0 || n <= 0 {
		return nil
	}
	order := WeakestFirst(contrasts, stats)
	weight := func(c Contrast) float64 { return PairWeight(stats[c.ID]) }

	shares := pairShares(order, weight, n)
	pairOrder := make([][]int, len(order)) // each contrast's shuffled pair indexes
	for i, c := range order {
		pairOrder[i] = shuffled(len(c.Pairs), intn)
	}

	var items []PairItem
	for round := 0; len(items) < n; round++ {
		for i, c := range order {
			if round >= shares[i] {
				continue
			}
			p := pairOrder[i][round%len(pairOrder[i])]
			items = append(items, PairItem{Contrast: c.ID, Pair: c.Pairs[p], Played: intn(2)})
		}
	}
	return items
}

// WeakestFirst returns contrasts ordered by PairWeight, highest (the
// weakest) first; ties keep the order of contrasts. Step 1 of PairRound.
func WeakestFirst(contrasts []Contrast, stats map[string]PairStat) []Contrast {
	order := slices.Clone(contrasts)
	weight := func(c Contrast) float64 { return PairWeight(stats[c.ID]) }
	slices.SortStableFunc(order, func(a, b Contrast) int { return compareDesc(weight(a), weight(b)) })
	return order
}

// pairShares is how many of n items each contrast of order gets: steps
// 2 and 3 of PairRound. The shares add up to n.
func pairShares(order []Contrast, weight func(Contrast) float64, n int) []int {
	shares := make([]int, len(order))
	first := min(minPairContrasts, len(order), n)
	for i := range first {
		shares[i] = 1
	}
	rest := n - first
	total := 0.0
	for _, c := range order {
		total += weight(c)
	}
	remainders := make([]float64, len(order))
	given := 0
	for i, c := range order {
		quota := float64(rest) * weight(c) / total
		shares[i] += int(quota)
		given += int(quota)
		remainders[i] = quota - float64(int(quota))
	}
	byRemainder := make([]int, len(order))
	for i := range byRemainder {
		byRemainder[i] = i
	}
	slices.SortStableFunc(byRemainder, func(a, b int) int { return compareDesc(remainders[a], remainders[b]) })
	for _, i := range byRemainder[:rest-given] {
		shares[i]++
	}
	return shares
}

// compareDesc orders larger numbers first, for a stable sort.
func compareDesc(a, b float64) int {
	switch {
	case a > b:
		return -1
	case a < b:
		return 1
	}
	return 0
}

// shuffled is 0..n-1 in an order chosen by intn (Fisher–Yates).
func shuffled(n int, intn func(int) int) []int {
	idx := make([]int, n)
	for i := range idx {
		idx[i] = i
	}
	for i := n - 1; i > 0; i-- {
		j := intn(i + 1)
		idx[i], idx[j] = idx[j], idx[i]
	}
	return idx
}

// TallyPairs counts the answers of a round per contrast, in the order
// the contrasts first came up. right[i] is whether items[i] was answered
// right; items without an answer are not counted.
func TallyPairs(items []PairItem, right []bool) []PairStat {
	var out []PairStat
	at := map[string]int{}
	for i, ok := range right[:min(len(right), len(items))] {
		id := items[i].Contrast
		j, seen := at[id]
		if !seen {
			j = len(out)
			at[id] = j
			out = append(out, PairStat{Contrast: id})
		}
		out[j].Attempts++
		if ok {
			out[j].Correct++
		}
	}
	return out
}
