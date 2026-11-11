package domain

import (
	"math/rand/v2"
	"testing"
)

// intn is a seeded random source, as the app gives PairRound one.
func intn(seed uint64) func(int) int {
	r := rand.New(rand.NewPCG(seed, 1))
	return r.IntN
}

func TestContrastsAreRealWordPairs(t *testing.T) {
	contrasts := Contrasts()
	if len(contrasts) < 3 {
		t.Fatalf("%d contrasts, want several", len(contrasts))
	}
	seen := map[string]bool{}
	for _, c := range contrasts {
		if c.ID == "" || seen[c.ID] {
			t.Errorf("contrast id %q is empty or repeated", c.ID)
		}
		seen[c.ID] = true
		if c.Label == "" || c.Tip == "" {
			t.Errorf("%s: want a label and a tip", c.ID)
		}
		if len(c.Pairs) < 3 {
			t.Errorf("%s: %d pairs, want at least 3", c.ID, len(c.Pairs))
		}
		for _, p := range c.Pairs {
			if p[0] == "" || p[1] == "" || p[0] == p[1] {
				t.Errorf("%s: bad pair %q", c.ID, p)
			}
		}
	}
	for _, id := range []string{"i-ee", "u-oo", "a-u", "b-v", "th-s", "th-d", "ed", "s-cluster"} {
		if !seen[id] {
			t.Errorf("missing contrast %s", id)
		}
	}
	// The list is a copy: changing it does not change the built-in pairs.
	contrasts[0].Pairs[0][0] = "changed"
	if Contrasts()[0].Pairs[0][0] == "changed" {
		t.Error("Contrasts must return a copy")
	}
}

func TestPairWeightFavorsWeakAndUnseenContrasts(t *testing.T) {
	unseen := PairWeight(PairStat{})
	weak := PairWeight(PairStat{Attempts: 10, Correct: 3})
	strong := PairWeight(PairStat{Attempts: 10, Correct: 10})
	if !(weak > unseen && unseen > strong) {
		t.Errorf("weights weak %v, unseen %v, strong %v; want weak > unseen > strong", weak, unseen, strong)
	}
	if unseen != 5.5 {
		t.Errorf("unseen weight %v, want 1 + 9×½ = 5.5", unseen)
	}
	if strong < 1 || PairWeight(PairStat{Attempts: 100}) > 10 {
		t.Error("weights must stay between 1 and 10")
	}
}

// countBy is how many items of each contrast a round has.
func countBy(items []PairItem) map[string]int {
	n := map[string]int{}
	for _, it := range items {
		n[it.Contrast]++
	}
	return n
}

func TestPairRoundPutsTheWeakestContrastFirstWithMoreItems(t *testing.T) {
	contrasts := Contrasts()
	stats := map[string]PairStat{}
	for _, c := range contrasts {
		stats[c.ID] = PairStat{Contrast: c.ID, Attempts: 20, Correct: 19}
	}
	stats["th-s"] = PairStat{Contrast: "th-s", Attempts: 20, Correct: 5} // the weakest
	stats["b-v"] = PairStat{Contrast: "b-v", Attempts: 20, Correct: 10}  // second
	delete(stats, "ed")                                                  // unseen: third

	items := PairRound(contrasts, stats, 20, intn(1))
	if len(items) != 20 {
		t.Fatalf("%d items, want 20", len(items))
	}
	if items[0].Contrast != "th-s" || items[1].Contrast != "b-v" || items[2].Contrast != "ed" {
		t.Errorf("first items %s %s %s, want th-s b-v ed", items[0].Contrast, items[1].Contrast, items[2].Contrast)
	}
	n := countBy(items)
	if !(n["th-s"] > n["b-v"] && n["b-v"] > n["i-ee"]) {
		t.Errorf("items per contrast %v: want th-s > b-v > a strong one", n)
	}
	if len(n) < 3 {
		t.Errorf("%d contrasts in the round, want at least 3", len(n))
	}
	// Round robin: the same contrast twice in a row only once the others
	// have run out.
	for i := 1; i < len(items); i++ {
		if items[i].Contrast != items[i-1].Contrast {
			continue
		}
		for _, rest := range items[i:] {
			if rest.Contrast != items[i].Contrast {
				t.Fatalf("item %d repeats %s while others remain", i, items[i].Contrast)
			}
		}
	}
}

func TestPairRoundWithoutStatsMixesEveryContrast(t *testing.T) {
	contrasts := Contrasts()
	items := PairRound(contrasts, nil, 20, intn(7))
	n := countBy(items)
	if len(n) != len(contrasts) {
		t.Errorf("items per contrast %v, want all %d", n, len(contrasts))
	}
	if items[0].Contrast != contrasts[0].ID {
		t.Errorf("ties keep the built-in order: first %s", items[0].Contrast)
	}
	for _, it := range items {
		if it.Played != 0 && it.Played != 1 {
			t.Fatalf("played %d, want 0 or 1", it.Played)
		}
		if it.Word() != it.Pair[it.Played] {
			t.Errorf("Word %q, want %q", it.Word(), it.Pair[it.Played])
		}
	}
}

func TestPairRoundMixesAtLeastThreeContrasts(t *testing.T) {
	contrasts := Contrasts()[:4]
	stats := map[string]PairStat{
		contrasts[0].ID: {Attempts: 100, Correct: 0},
		contrasts[1].ID: {Attempts: 100, Correct: 0},
		contrasts[2].ID: {Attempts: 100, Correct: 100},
		contrasts[3].ID: {Attempts: 100, Correct: 100},
	}
	if n := countBy(PairRound(contrasts, stats, 20, intn(3))); len(n) < 3 {
		t.Errorf("items per contrast %v, want at least 3 contrasts", n)
	}
}

func TestPairRoundIsSeeded(t *testing.T) {
	a := PairRound(Contrasts(), nil, 20, intn(5))
	b := PairRound(Contrasts(), nil, 20, intn(5))
	for i := range a {
		if a[i] != b[i] {
			t.Fatalf("item %d: %+v then %+v with the same seed", i, a[i], b[i])
		}
	}
}

func TestTallyPairsCountsEachContrast(t *testing.T) {
	items := []PairItem{{Contrast: "b-v"}, {Contrast: "i-ee"}, {Contrast: "b-v"}}
	got := TallyPairs(items, []bool{true, false, false})
	want := []PairStat{{Contrast: "b-v", Attempts: 2, Correct: 1}, {Contrast: "i-ee", Attempts: 1}}
	if len(got) != len(want) || got[0] != want[0] || got[1] != want[1] {
		t.Errorf("TallyPairs = %+v, want %+v", got, want)
	}
	if acc := got[0].Accuracy(); acc != 0.5 {
		t.Errorf("Accuracy = %v, want 0.5", acc)
	}
}

func TestWeakestFirstKeepsTheBuiltInOrderOnTies(t *testing.T) {
	stats := map[string]PairStat{"a-u": {Attempts: 4, Correct: 0}}
	order := WeakestFirst(Contrasts(), stats)
	if order[0].ID != "a-u" || order[1].ID != "i-ee" || order[2].ID != "u-oo" {
		t.Errorf("order %s %s %s, want a-u i-ee u-oo", order[0].ID, order[1].ID, order[2].ID)
	}
}
