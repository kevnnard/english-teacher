package check

import (
	"strings"
	"testing"
)

func TestNormalize(t *testing.T) {
	tests := map[string]string{
		"  Nice to meet you!  ": "nice to meet you",
		"Hello.":                "hello",
		"what?!":                "what",
		"See you later…":        "see you later",
		"it's":                  "it's",
		"good  morning":         "good  morning", // inner spaces are the learner's answer
	}
	for in, want := range tests {
		if got := Normalize(in); got != want {
			t.Errorf("Normalize(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestAnswerExactIgnoresCaseSpacesAndTrailingPunctuation(t *testing.T) {
	for _, typed := range []string{"nice to meet you", "  Nice to meet you. ", "NICE TO MEET YOU!"} {
		r := Answer(typed, "Nice to meet you")
		if r.Kind != Exact || r.Distance != 0 {
			t.Errorf("Answer(%q) = %+v, want exact", typed, r)
		}
	}
}

func TestAnswerAlmostShowsLetterDiff(t *testing.T) {
	r := Answer("nice to meat you", "nice to meet you")

	if r.Kind != Almost || r.Distance != 1 {
		t.Fatalf("Answer = %+v, want almost with distance 1", r)
	}
	if got := Plain(r.Diff); got != "nice to me[a→e]t you" {
		t.Errorf("Plain(diff) = %q", got)
	}
}

func TestAnswerAlmostUpToTwoLetters(t *testing.T) {
	tests := []struct {
		typed, expected string
		distance        int
	}{
		{"developper", "developer", 1},      // one extra letter
		{"argentinan", "argentinian", 1},    // one missing letter
		{"see yu latr", "see you later", 2}, // two missing letters
		{"thnak you", "thank you", 2},       // swapped letters count as two
	}
	for _, tt := range tests {
		r := Answer(tt.typed, tt.expected)
		if r.Kind != Almost || r.Distance != tt.distance {
			t.Errorf("Answer(%q, %q) = %+v, want almost with distance %d", tt.typed, tt.expected, r, tt.distance)
		}
		checkDiffRebuilds(t, r.Diff, tt.typed, tt.expected)
	}
}

// checkDiffRebuilds verifies the diff is faithful: equal + deleted text is
// what was typed, equal + inserted text is what was expected.
func checkDiffRebuilds(t *testing.T, diff []Op, typed, expected string) {
	t.Helper()
	var from, to strings.Builder
	edits := 0
	for _, op := range diff {
		switch op.Kind {
		case Equal:
			from.WriteString(op.Text)
			to.WriteString(op.Text)
		case Delete:
			from.WriteString(op.Text)
			edits++
		case Insert:
			to.WriteString(op.Text)
			edits++
		}
	}
	if from.String() != typed || to.String() != expected {
		t.Errorf("diff %v rebuilds %q → %q, want %q → %q", diff, from.String(), to.String(), typed, expected)
	}
}

func TestAnswerWrong(t *testing.T) {
	tests := []struct{ typed, expected string }{
		{"goodbye", "nice to meet you"},
		{"hallo wrd", "hello world"}, // three letters off
		{"", "hi"},                   // an empty answer is never "almost"
		{"   ", "hi"},
	}
	for _, tt := range tests {
		if r := Answer(tt.typed, tt.expected); r.Kind != Wrong {
			t.Errorf("Answer(%q, %q) = %+v, want wrong", tt.typed, tt.expected, r)
		}
	}
}

func TestGap(t *testing.T) {
	got, ok := Gap("Nice to meet you, I'm Kevin.", "nice to meet you")
	if !ok || got != "________________, I'm Kevin." {
		t.Errorf("Gap = %q, %v", got, ok)
	}

	if _, ok := Gap("I love Japanese food.", "Japan"); ok {
		t.Error("Gap should not cut a word in the middle")
	}
	if _, ok := Gap("Hola, ¿qué tal?", "hello"); ok {
		t.Error("Gap should report a sentence without the word")
	}
}
