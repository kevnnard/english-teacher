package domain

import (
	"reflect"
	"strings"
	"testing"
)

// item is a cloze item that only has what Interleave looks at.
func item(point, answer string) ClozeItem {
	return ClozeItem{Point: point, Answer: answer, Sentence: "x " + answer}
}

// points are the points of items, in order.
func points(items []ClozeItem) []string {
	out := make([]string, len(items))
	for i, it := range items {
		out[i] = it.Point
	}
	return out
}

// noRepeats fails when two neighbours practice the same point.
func noRepeats(t *testing.T, items []ClozeItem) {
	t.Helper()
	for i := 1; i < len(items); i++ {
		if strings.EqualFold(items[i].Point, items[i-1].Point) {
			t.Fatalf("items %d and %d both practice %q: %v", i-1, i, items[i].Point, points(items))
		}
	}
}

func TestInterleaveMixesThePointsWithoutRepeats(t *testing.T) {
	// The model tends to group items by point; the round must not.
	items := []ClozeItem{
		item("past simple", "went"), item("past simple", "saw"), item("past simple", "ate"),
		item("articles", "a"), item("articles", "the"),
		item("going to", "going to"), item("going to", "is going to"),
	}

	got, err := Interleave(items, nil, 6)

	if err != nil {
		t.Fatal(err)
	}
	want := []string{"past simple", "articles", "going to", "past simple", "articles", "going to"}
	if !reflect.DeepEqual(points(got), want) {
		t.Errorf("points = %v, want %v", points(got), want)
	}
	if got[0].Answer != "went" || got[3].Answer != "saw" {
		t.Errorf("items of one point must keep their order: %v", got)
	}
	noRepeats(t, got)
}

func TestInterleaveWeightsTheLearnersMistakesFirst(t *testing.T) {
	items := []ClozeItem{
		item("past simple", "went"), item("past simple", "saw"),
		item("articles", "a"), item("articles", "the"), item("articles", "an"),
		item("going to", "going to"), item("going to", "are going to"),
		item("plurals", "boxes"),
	}

	// The learner's top Write mistakes are articles, then plurals.
	got, err := Interleave(items, []string{"Articles", "plurals"}, 8)

	if err != nil {
		t.Fatal(err)
	}
	want := []string{"articles", "plurals", "past simple", "going to", "articles", "past simple", "articles", "going to"}
	if !reflect.DeepEqual(points(got), want) {
		t.Errorf("points = %v, want %v", points(got), want)
	}
	noRepeats(t, got)
}

func TestInterleaveStopsBeforeRepeatingAPoint(t *testing.T) {
	items := []ClozeItem{
		item("past simple", "went"), item("past simple", "saw"), item("past simple", "ate"), item("past simple", "had"),
		item("articles", "a"), item("going to", "going to"),
	}

	got, err := Interleave(items, nil, 8)

	if err != nil {
		t.Fatal(err)
	}
	// Past simple, articles, going to, past simple; then only past simple
	// is left, and a third one in a row is never asked.
	want := []string{"past simple", "articles", "going to", "past simple"}
	if !reflect.DeepEqual(points(got), want) {
		t.Errorf("points = %v, want %v", points(got), want)
	}
}

func TestInterleaveNeedsThreePoints(t *testing.T) {
	items := []ClozeItem{
		item("past simple", "went"), item("Past Simple", "saw"),
		item("articles", "a"), item("articles", "the"),
	}

	_, err := Interleave(items, nil, 8)

	if err == nil || !strings.Contains(err.Error(), "2 grammar points") {
		t.Errorf("err = %v, want too few points (2 grammar points)", err)
	}
}

func TestInterleaveDropsItemsWithoutAPointOrAnswer(t *testing.T) {
	items := []ClozeItem{
		item("", "went"), item("past simple", " "),
		item("past simple", "saw"), item("articles", "a"), item("going to", "going to"),
	}

	got, err := Interleave(items, nil, 8)

	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"past simple", "articles", "going to"}; !reflect.DeepEqual(points(got), want) {
		t.Errorf("points = %v, want %v", points(got), want)
	}
}

func TestChunkCardHasThePhraseItsMeaningAndUseAndAnExample(t *testing.T) {
	c := Chunk{Phrase: " I'm looking for… ", Meaning: "Estoy buscando…", Use: "to say what you want in a shop", Example: "I'm looking for a jacket."}

	card := c.Card()

	if card.Word != "I'm looking for…" || card.Example != "I'm looking for a jacket." {
		t.Errorf("card = %+v", card)
	}
	if want := "Estoy buscando… — to say what you want in a shop"; card.Meaning != want {
		t.Errorf("meaning = %q, want %q", card.Meaning, want)
	}
	if card.EaseFactor != DefaultEaseFactor || card.Direction != Recognition {
		t.Errorf("a chunk card is a fresh card: %+v", card)
	}
	if (Chunk{Phrase: "  "}).Valid() || !c.Valid() {
		t.Error("a chunk needs a phrase and a meaning")
	}
}
