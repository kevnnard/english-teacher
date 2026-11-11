package domain

import (
	"slices"
	"strings"
)

// Prosody (spec S6): word stress and intonation, two things Spanish
// does differently. A Spanish speaker reads the written accent; English
// writes none, and the stress moves inside a word family (PHOtograph,
// phoTOgraphy). English also asks a yes/no question with the words of a
// statement and only the voice rising at the end.
//
// Both are drills of the Pairs screen: the learner hears a word or a
// sentence and chooses. The right answer is in the built-in data below;
// no model decides it. Results are kept per drill like the results of a
// minimal-pair contrast, so the next round starts with the weakest.

// ProsodyItem is one word or sentence of a drill.
type ProsodyItem struct {
	Drill   string   // the Drill ID, stored with the results like a contrast
	Say     string   // what is spoken
	Shown   string   // what the page shows before the answer
	Choices []string // the options, pressed as 1, 2, …
	Answer  int      // the index of the right option
	Reveal  string   // what the page shows after the answer
}

// Drill is one group of prosody items with what they train.
type Drill struct {
	ID    string // stable key, stored with the learner's results
	Label string // what the drill trains, in Spanish
	Tip   string // how to hear or make it, in Spanish
	Items []ProsodyItem
}

// stressWord is a word cut into syllables, with the stressed one.
type stressWord struct {
	syllables string // the syllables, joined by "·"
	stress    int    // the index of the stressed syllable
}

// stressGroup is the data of one stress drill.
type stressGroup struct {
	id, label, tip string
	words          []stressWord
}

// stressGroups are the built-in stress drills. Each stress is the main
// stress of the word in American English, which is also the one piper's
// phonemizer (espeak-ng) gives, so the voice says what the key expects.
var stressGroups = []stressGroup{
	{"stress:noun2", "nombre de 2 sílabas: 1.ª",
		"Casi todos los nombres y adjetivos de dos sílabas llevan el golpe en la primera: TA·ble, HAP·py.",
		[]stressWord{{"ta·ble", 0}, {"hap·py", 0}, {"wa·ter", 0}, {"doc·tor", 0}, {"gar·den", 0}, {"win·dow", 0}}},
	{"stress:verb2", "verbo de 2 sílabas: 2.ª",
		"Muchos verbos de dos sílabas llevan el golpe en la segunda: be·GIN, de·CIDE. La primera casi no suena.",
		[]stressWord{{"be·gin", 1}, {"de·cide", 1}, {"for·get", 1}, {"re·lax", 1}, {"en·joy", 1}, {"ex·plain", 1}}},
	{"stress:tion", "-tion, -ic: la de antes",
		"Con -tion, -sion e -ic el golpe va en la sílaba justo antes: in·for·MA·tion, fan·TAS·tic.",
		[]stressWord{{"in·for·ma·tion", 2}, {"ed·u·ca·tion", 2}, {"de·ci·sion", 1}, {"con·ver·sa·tion", 2}, {"fan·tas·tic", 1}, {"e·lec·tric", 1}}},
	{"stress:ity", "-ity, -logy: 3.ª del final",
		"Con -ity, -logy y -graphy el golpe va tres sílabas antes del final: a·BIL·i·ty, bi·OL·o·gy.",
		[]stressWord{{"bi·ol·o·gy", 1}, {"u·ni·ver·si·ty", 2}, {"a·bil·i·ty", 1}, {"com·mu·ni·ty", 1}, {"ge·og·ra·phy", 1}}},
	{"stress:family", "familias: el golpe se mueve",
		"En una familia de palabras el golpe cambia con el final: PHO·to·graph, pho·TOG·ra·phy, pho·to·GRAPH·ic. No lo copies de la palabra base.",
		[]stressWord{{"pho·to·graph", 0}, {"pho·tog·ra·pher", 1}, {"pho·tog·ra·phy", 1}, {"pho·to·graph·ic", 2},
			{"e·con·o·my", 1}, {"e·co·nom·ic", 2}, {"pol·i·tics", 0}, {"po·lit·i·cal", 1}, {"pol·i·ti·cian", 2}}},
}

// StressDrills returns the built-in word-stress drills: hear a word and
// choose its stressed syllable. It is a fresh copy each time.
func StressDrills() []Drill {
	out := make([]Drill, len(stressGroups))
	for i, g := range stressGroups {
		d := Drill{ID: g.id, Label: g.label, Tip: g.tip}
		for _, w := range g.words {
			syl := strings.Split(w.syllables, "·")
			marked := slices.Clone(syl)
			marked[w.stress] = strings.ToUpper(marked[w.stress])
			word := strings.Join(syl, "")
			d.Items = append(d.Items, ProsodyItem{
				Drill: g.id, Say: word, Shown: word, Choices: syl, Answer: w.stress,
				Reveal: strings.Join(marked, "·"),
			})
		}
		out[i] = d
	}
	return out
}

// toneSentences have the word order of a statement, so only the voice
// tells a question. Each one was checked with piper and the default
// voice (en_US-lessac-medium) at both speeds: with "." the pitch falls
// at the end (about 100–120 Hz), with "?" it ends high (about 220–310
// Hz), roughly an octave apart. Sentences whose question fell too, like
// "You've got a car?" or "You speak Spanish?", were left out. The check
// is TestLiveIntonationRisesAndFalls in internal/speech/piper.
var toneSentences = []string{
	"You're coming home", "She likes coffee", "It's ready", "They live here",
	"He's your brother", "We meet tomorrow", "This is your bag", "You're tired",
	"They're here", "You're a teacher", "He called you", "The shop is open",
	"You're from Mexico", "She's your mother",
}

// toneChoices are the two options of an intonation item.
var toneChoices = []string{"question ↗", "statement ↘"}

// ToneDrills returns the two intonation drills, question and statement:
// the same sentences, said with "?" (rising) or "." (falling). The
// learner chooses which one was heard.
func ToneDrills() []Drill {
	q := Drill{ID: "tone:question", Label: "pregunta: la voz sube ↗",
		Tip: "Una pregunta de sí o no con el orden de una afirmación (You're tired?) solo se nota en la voz: sube al final, en la última palabra."}
	s := Drill{ID: "tone:statement", Label: "afirmación: la voz baja ↘",
		Tip: "Una afirmación baja al final: la voz cae en la última palabra y se apaga. Si no baja, suena a pregunta o a que no has terminado."}
	for _, words := range toneSentences {
		q.Items = append(q.Items, ProsodyItem{Drill: q.ID, Say: words + "?", Shown: words,
			Choices: slices.Clone(toneChoices), Answer: 0, Reveal: words + "? ↗"})
		s.Items = append(s.Items, ProsodyItem{Drill: s.ID, Say: words + ".", Shown: words,
			Choices: slices.Clone(toneChoices), Answer: 1, Reveal: words + ". ↘"})
	}
	return []Drill{q, s}
}

// ProsodyRound picks n items of drills the way PairRound picks minimal
// pairs: the weakest drills (by PairWeight of their stats) first and with
// the most items, at least three drills when there are three, round
// robin, each drill's items shuffled by intn. It is pure.
func ProsodyRound(drills []Drill, stats map[string]PairStat, n int, intn func(int) int) []ProsodyItem {
	order := slices.DeleteFunc(WeakestDrills(drills, stats), func(d Drill) bool { return len(d.Items) == 0 })
	if len(order) == 0 || n <= 0 {
		return nil
	}
	weights, sizes := make([]float64, len(order)), make([]int, len(order))
	for i, d := range order {
		weights[i], sizes[i] = PairWeight(stats[d.ID]), len(d.Items)
	}
	var items []ProsodyItem
	weightedRound(weights, sizes, n, intn, func(g, k int) {
		items = append(items, order[g].Items[k])
	})
	return items
}

// WeakestDrills returns drills ordered by PairWeight, the weakest first;
// ties keep the order of drills. It is WeakestFirst for drills.
func WeakestDrills(drills []Drill, stats map[string]PairStat) []Drill {
	order := slices.Clone(drills)
	weight := func(d Drill) float64 { return PairWeight(stats[d.ID]) }
	slices.SortStableFunc(order, func(a, b Drill) int { return compareDesc(weight(a), weight(b)) })
	return order
}

// TallyProsody counts the answers of a round per drill, like TallyPairs:
// right[i] is whether items[i] was answered right.
func TallyProsody(items []ProsodyItem, right []bool) []PairStat {
	ids := make([]string, len(items))
	for i, it := range items {
		ids[i] = it.Drill
	}
	return tally(ids, right)
}
