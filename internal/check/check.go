// Package check compares a typed answer with the expected English and
// explains the difference letter by letter. It is pure: no I/O, no UI.
package check

import (
	"slices"
	"strings"
	"unicode"
)

// AlmostDistance is the most letters an answer may be off and still count as
// "almost": close enough to show a diff instead of only the right answer.
const AlmostDistance = 2

// Kind is the verdict on a typed answer.
type Kind int

const (
	Exact  Kind = iota // same text once normalized
	Almost             // at most AlmostDistance letters off
	Wrong              // anything else, including an empty answer
)

// OpKind says what one piece of a diff is.
type OpKind int

const (
	Equal  OpKind = iota // typed and expected agree
	Insert               // missing from the typed answer
	Delete               // typed but not expected
)

// Op is one run of the letter diff from the typed answer to the expected one.
type Op struct {
	Kind OpKind
	Text string
}

// Result is the outcome of Answer.
type Result struct {
	Kind     Kind
	Distance int  // letters inserted, deleted or substituted (Levenshtein)
	Diff     []Op // how to turn the normalized typed answer into the expected one
}

// Normalize lowercases s and drops surrounding spaces and trailing
// punctuation, the differences that never make an answer wrong.
func Normalize(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	s = strings.TrimRightFunc(s, unicode.IsPunct)
	return strings.TrimSpace(s)
}

// Answer checks typed against expected after normalizing both.
func Answer(typed, expected string) Result {
	from, to := []rune(Normalize(typed)), []rune(Normalize(expected))
	diff, dist := levenshtein(from, to)

	r := Result{Distance: dist, Diff: diff}
	switch {
	case dist == 0:
		r.Kind = Exact
	case len(from) > 0 && dist <= AlmostDistance:
		r.Kind = Almost
	default:
		r.Kind = Wrong
	}
	return r
}

// levenshtein returns the edit distance from a to b and the edits themselves.
func levenshtein(a, b []rune) ([]Op, int) {
	// d[i][j] is the distance between a[:i] and b[:j].
	d := make([][]int, len(a)+1)
	for i := range d {
		d[i] = make([]int, len(b)+1)
		d[i][0] = i
	}
	for j := range d[0] {
		d[0][j] = j
	}
	for i := 1; i <= len(a); i++ {
		for j := 1; j <= len(b); j++ {
			cost := 1
			if a[i-1] == b[j-1] {
				cost = 0
			}
			d[i][j] = min(d[i-1][j]+1, d[i][j-1]+1, d[i-1][j-1]+cost)
		}
	}

	// Walk back from the end to recover the edits, then reverse them.
	var ops []Op
	add := func(k OpKind, r rune) {
		if n := len(ops); n > 0 && ops[n-1].Kind == k {
			ops[n-1].Text = string(r) + ops[n-1].Text
			return
		}
		ops = append(ops, Op{Kind: k, Text: string(r)})
	}
	i, j := len(a), len(b)
	for i > 0 || j > 0 {
		switch {
		case i > 0 && j > 0 && a[i-1] == b[j-1] && d[i][j] == d[i-1][j-1]:
			add(Equal, a[i-1])
			i, j = i-1, j-1
		case i > 0 && j > 0 && d[i][j] == d[i-1][j-1]+1: // substitution
			add(Insert, b[j-1])
			add(Delete, a[i-1])
			i, j = i-1, j-1
		case i > 0 && d[i][j] == d[i-1][j]+1:
			add(Delete, a[i-1])
			i--
		default:
			add(Insert, b[j-1])
			j--
		}
	}
	slices.Reverse(ops)
	return ops, d[len(a)][len(b)]
}

// Plain writes diff as text, each change in brackets as [typed→expected]:
// "nice to me[a→e]t you".
func Plain(diff []Op) string {
	var b strings.Builder
	for _, g := range Group(diff) {
		if g.Equal != "" {
			b.WriteString(g.Equal)
			continue
		}
		b.WriteString("[" + g.Deleted + "→" + g.Inserted + "]")
	}
	return b.String()
}

// Change is one stretch of a diff: either equal text, or what was typed
// instead of what was expected.
type Change struct {
	Equal             string
	Deleted, Inserted string
}

// Group merges neighbouring deletes and inserts into one Change, so a
// substitution reads as a single [a→e].
func Group(diff []Op) []Change {
	var out []Change
	for _, op := range diff {
		if op.Kind == Equal {
			out = append(out, Change{Equal: op.Text})
			continue
		}
		if n := len(out); n == 0 || out[n-1].Equal != "" {
			out = append(out, Change{})
		}
		c := &out[len(out)-1]
		if op.Kind == Delete {
			c.Deleted += op.Text
		} else {
			c.Inserted += op.Text
		}
	}
	return out
}

// Gap hides word inside sentence with underscores, for a production prompt:
// Gap("Nice to meet you, I'm Kevin.", "nice to meet you") is
// "________________, I'm Kevin.". It matches whole words only, ignoring
// case, and reports false when the sentence does not contain the word.
func Gap(sentence, word string) (string, bool) {
	s, w := []rune(sentence), []rune(strings.TrimSpace(word))
	if len(w) == 0 {
		return sentence, false
	}
	for i := 0; i+len(w) <= len(s); i++ {
		if !equalFold(s[i:i+len(w)], w) || isLetter(s, i-1) || isLetter(s, i+len(w)) {
			continue
		}
		out := slices.Clone(s)
		for k := i; k < i+len(w); k++ {
			out[k] = '_'
		}
		return string(out), true
	}
	return sentence, false
}

func equalFold(a, b []rune) bool {
	for i := range a {
		if unicode.ToLower(a[i]) != unicode.ToLower(b[i]) {
			return false
		}
	}
	return true
}

// isLetter reports whether s[i] exists and is part of a word.
func isLetter(s []rune, i int) bool {
	return i >= 0 && i < len(s) && (unicode.IsLetter(s[i]) || unicode.IsDigit(s[i]))
}
