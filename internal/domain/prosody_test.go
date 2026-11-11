package domain

import (
	"fmt"
	"strings"
	"testing"
)

func TestStressDrillsMarkOneSyllableOfRealWords(t *testing.T) {
	drills := StressDrills()
	if len(drills) < 3 {
		t.Fatalf("%d stress drills, want several", len(drills))
	}
	seen := map[string]bool{}
	words := map[string]bool{}
	for _, d := range drills {
		if !strings.HasPrefix(d.ID, "stress:") || seen[d.ID] {
			t.Errorf("drill id %q: want a unique stress: id", d.ID)
		}
		seen[d.ID] = true
		if d.Label == "" || d.Tip == "" || len(d.Items) < 3 {
			t.Errorf("%s: want a label, a tip and at least 3 words", d.ID)
		}
		for _, it := range d.Items {
			if it.Drill != d.ID {
				t.Errorf("%s: item %q belongs to %q", d.ID, it.Say, it.Drill)
			}
			if len(it.Choices) < 2 || it.Answer < 0 || it.Answer >= len(it.Choices) {
				t.Errorf("%s: %q has %d syllables and stress %d", d.ID, it.Say, len(it.Choices), it.Answer)
			}
			if strings.Join(it.Choices, "") != it.Say || it.Shown != it.Say {
				t.Errorf("%s: syllables %q do not spell %q", d.ID, it.Choices, it.Say)
			}
			if words[it.Say] {
				t.Errorf("%q is in two drills", it.Say)
			}
			words[it.Say] = true
		}
	}
	if !seen["stress:family"] {
		t.Error("want the word families whose stress moves")
	}
}

// stressOf is the built-in word with its stressed syllable in capitals,
// as the feedback shows it.
func stressOf(t *testing.T, word string) string {
	t.Helper()
	for _, d := range StressDrills() {
		for _, it := range d.Items {
			if it.Say == word {
				return it.Reveal
			}
		}
	}
	t.Fatalf("no stress word %q", word)
	return ""
}

func TestStressFamiliesMoveTheStress(t *testing.T) {
	for word, want := range map[string]string{
		"photograph":   "PHO·to·graph",
		"photographer": "pho·TOG·ra·pher",
		"photography":  "pho·TOG·ra·phy",
		"photographic": "pho·to·GRAPH·ic",
		"economy":      "e·CON·o·my",
		"economic":     "e·co·NOM·ic",
		"table":        "TA·ble",
		"begin":        "be·GIN",
		"information":  "in·for·MA·tion",
	} {
		if got := stressOf(t, word); got != want {
			t.Errorf("%s: %q, want %q", word, got, want)
		}
	}
}

func TestToneDrillsSayTheSameWordsAsAQuestionOrAStatement(t *testing.T) {
	drills := ToneDrills()
	if len(drills) != 2 || drills[0].ID != "tone:question" || drills[1].ID != "tone:statement" {
		t.Fatalf("tone drills %+v, want question and statement", drills)
	}
	q, s := drills[0], drills[1]
	if len(q.Items) < 8 || len(q.Items) != len(s.Items) {
		t.Fatalf("%d questions, %d statements; want the same sentences, at least 8", len(q.Items), len(s.Items))
	}
	for i := range q.Items {
		qi, si := q.Items[i], s.Items[i]
		if qi.Shown != si.Shown || strings.ContainsAny(qi.Shown, "?.") {
			t.Errorf("shown %q / %q: want the same words with no end mark", qi.Shown, si.Shown)
		}
		// The end mark is all the voice gets: piper rises on ? and falls on .
		if qi.Say != qi.Shown+"?" || si.Say != si.Shown+"." {
			t.Errorf("said %q / %q for %q", qi.Say, si.Say, qi.Shown)
		}
		if qi.Answer != 0 || si.Answer != 1 || len(qi.Choices) != 2 || qi.Choices[0] != si.Choices[0] {
			t.Errorf("%q: answers %d / %d, choices %q", qi.Shown, qi.Answer, si.Answer, qi.Choices)
		}
		if !strings.Contains(qi.Reveal, "?") || !strings.Contains(si.Reveal, ".") {
			t.Errorf("reveal %q / %q: want the end mark", qi.Reveal, si.Reveal)
		}
	}
}

func TestProsodyRoundPutsTheWeakestDrillFirst(t *testing.T) {
	drills := StressDrills()
	stats := map[string]PairStat{}
	for _, d := range drills {
		stats[d.ID] = PairStat{Contrast: d.ID, Attempts: 20, Correct: 19}
	}
	stats["stress:family"] = PairStat{Contrast: "stress:family", Attempts: 20, Correct: 4}

	items := ProsodyRound(drills, stats, 15, intn(2))
	if len(items) != 15 {
		t.Fatalf("%d items, want 15", len(items))
	}
	if items[0].Drill != "stress:family" {
		t.Errorf("first item from %s, want stress:family", items[0].Drill)
	}
	n := map[string]int{}
	for _, it := range items {
		n[it.Drill]++
	}
	if len(n) < 3 {
		t.Errorf("items per drill %v, want at least 3 drills", n)
	}
	for id, k := range n {
		if id != "stress:family" && k >= n["stress:family"] {
			t.Errorf("items per drill %v: want the weakest to get the most", n)
		}
	}
	// The same seed gives the same round.
	again := ProsodyRound(drills, stats, 15, intn(2))
	for i := range items {
		if items[i].Say != again[i].Say {
			t.Fatalf("item %d: %q then %q with the same seed", i, items[i].Say, again[i].Say)
		}
	}
}

func TestProsodyRoundMixesQuestionsAndStatements(t *testing.T) {
	items := ProsodyRound(ToneDrills(), nil, 12, intn(4))
	n := map[string]int{}
	for _, it := range items {
		n[it.Drill]++
	}
	if len(items) != 12 || n["tone:question"] != 6 || n["tone:statement"] != 6 {
		t.Errorf("%d items %v, want 6 of each", len(items), n)
	}
	if ProsodyRound(nil, nil, 12, intn(4)) != nil || ProsodyRound(ToneDrills(), nil, 0, intn(4)) != nil {
		t.Error("no drills or no items: want no round")
	}
}

func TestTallyProsodyCountsEachDrill(t *testing.T) {
	items := []ProsodyItem{{Drill: "tone:question"}, {Drill: "tone:statement"}, {Drill: "tone:question"}}
	got := TallyProsody(items, []bool{false, true})
	want := []PairStat{{Contrast: "tone:question", Attempts: 1}, {Contrast: "tone:statement", Attempts: 1, Correct: 1}}
	if fmt.Sprint(got) != fmt.Sprint(want) {
		t.Errorf("TallyProsody = %+v, want %+v (unanswered items are not counted)", got, want)
	}
}

// Prosody shares the round scheduling of minimal pairs; a seeded round
// of pairs must stay exactly what it was before.
func TestPairRoundIsUnchangedBySharedScheduling(t *testing.T) {
	var b strings.Builder
	for _, it := range PairRound(Contrasts(), map[string]PairStat{"b-v": {Attempts: 6, Correct: 1}}, 20, intn(9)) {
		fmt.Fprintf(&b, "%s ", it.Word())
	}
	const want = "very ship pool hut mouth dough walk esteem best live fool cap sink den opened especial bat fill Luke van"
	if got := strings.TrimSpace(b.String()); got != want {
		t.Errorf("seeded pair round:\n got %s\nwant %s", got, want)
	}
}

func TestWeakestDrillsKeepsTheBuiltInOrderOnTies(t *testing.T) {
	order := WeakestDrills(StressDrills(), map[string]PairStat{"stress:ity": {Attempts: 4}})
	if order[0].ID != "stress:ity" || order[1].ID != "stress:noun2" || order[2].ID != "stress:verb2" {
		t.Errorf("order %s %s %s, want stress:ity stress:noun2 stress:verb2", order[0].ID, order[1].ID, order[2].ID)
	}
}
