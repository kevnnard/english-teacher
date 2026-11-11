package tui

import (
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/kevnnard/english-teacher/internal/domain"
	"github.com/kevnnard/english-teacher/internal/speech"
)

// prosodyItem is the word or sentence on screen in Word stress or
// Intonation.
func prosodyItem(a App) domain.ProsodyItem { return a.prosody.items[a.prosody.pos] }

// pressProsody answers the item on screen right, or wrong when !right.
func pressProsody(t *testing.T, a App, right bool) App {
	t.Helper()
	it := prosodyItem(a)
	k := it.Answer
	if !right {
		k = (k + 1) % len(it.Choices)
	}
	a, cmd := press(t, a, fmt.Sprint(k+1))
	if cmd != nil {
		t.Fatal("answering must not play anything")
	}
	return a
}

// drillOf is the built-in stress or intonation drill with id.
func drillOf(t *testing.T, id string) domain.Drill {
	t.Helper()
	for _, d := range append(domain.StressDrills(), domain.ToneDrills()...) {
		if d.ID == id {
			return d
		}
	}
	t.Fatalf("no drill %s", id)
	return domain.Drill{}
}

func TestPairsMSwitchesToWordStressAndIntonation(t *testing.T) {
	x := newPairsApp(t, true, nil, nil)
	mustContain(t, statusBar(x.App), "m stress · intonation")

	a, cmd := press(t, x.App, "m")
	if cmd != nil {
		t.Error("switching must not play anything")
	}
	mustContain(t, pageText(a), "Pairs · Word stress", "stressed syllable", fmt.Sprintf("%d words", stressItems))
	mustContain(t, statusBar(a), "enter start", "m intonation")

	a, _ = press(t, a, "m")
	mustContain(t, pageText(a), "Pairs · Intonation", "question", "statement", fmt.Sprintf("%d sentences", toneItems))
	mustContain(t, statusBar(a), "m minimal pairs")

	a, _ = press(t, a, "m")
	mustContain(t, pageText(a), "Pairs · Minimal pairs", "20 words")
	if len(x.sp.said) != 0 {
		t.Error("an intro must not play anything")
	}
}

func TestWordStressPlaysTheWordAndChecksTheSyllable(t *testing.T) {
	x := newPairsApp(t, true, nil, nil)
	a, cmd := press(t, x.App, "m", "enter")
	if len(a.prosody.items) != stressItems {
		t.Fatalf("%d items, want %d", len(a.prosody.items), stressItems)
	}
	it := prosodyItem(a)
	a = said(t, a, cmd, x.sp, spoken{it.Say, speech.Options{}}) // played on arrival
	page := pageText(a)
	mustContain(t, page, fmt.Sprintf("Word 1 of %d", stressItems), "Which syllable is stressed?", it.Shown)
	for i, syl := range it.Choices {
		mustContain(t, page, fmt.Sprintf("%d %s", i+1, syl))
	}
	mustContain(t, statusBar(a), fmt.Sprintf("1–%d choose", len(it.Choices)), "p again", "s slower")

	a, cmd = press(t, a, "s")
	a = said(t, a, cmd, x.sp, spoken{it.Say, speech.Options{Slow: true}})
	// A number with no syllable does nothing.
	a, _ = press(t, a, fmt.Sprint(len(it.Choices)+1))
	if len(a.prosody.chosen) != 0 {
		t.Fatal("a number past the last syllable must not answer")
	}

	a = pressProsody(t, a, true)
	mustContain(t, pageText(a), "✓ "+it.Reveal)
	mustContain(t, statusBar(a), "enter next")

	a, cmd = press(t, a, "enter")
	a = run(t, a, cmd)
	wrong := prosodyItem(a)
	a = pressProsody(t, a, false)
	d := drillOf(t, wrong.Drill)
	stressed := strings.ToUpper(wrong.Choices[wrong.Answer])
	mustContain(t, pageText(a), "✗ The stress is on "+stressed+": "+wrong.Reveal, d.Label)
	if a.prosody.score() != 1 {
		t.Errorf("score %d, want 1", a.prosody.score())
	}
}

func TestIntonationSaysTheEndMarkButShowsOnlyTheWords(t *testing.T) {
	// Intonation keeps the configured voice even with two installed.
	x := newPairsApp(t, true, []string{"en_US-amy-medium", "en_US-lessac-medium"}, nil)
	a, cmd := press(t, x.App, "m", "m", "enter")
	if len(a.prosody.items) != toneItems {
		t.Fatalf("%d items, want %d", len(a.prosody.items), toneItems)
	}
	it := prosodyItem(a)
	a = said(t, a, cmd, x.sp, spoken{it.Say, speech.Options{}})
	if !strings.HasSuffix(it.Say, "?") && !strings.HasSuffix(it.Say, ".") {
		t.Fatalf("said %q: want the end mark the voice follows", it.Say)
	}
	page := pageText(a)
	mustContain(t, page, fmt.Sprintf("Sentence 1 of %d", toneItems), "Did the voice rise or fall?",
		it.Shown, "1 question ↗", "2 statement ↘")
	mustNotContain(t, page, it.Shown+"?", it.Shown+".")

	a = pressProsody(t, a, false)
	d := drillOf(t, it.Drill)
	mustContain(t, pageText(a), "✗ You heard a "+strings.Fields(it.Choices[it.Answer])[0]+": "+it.Reveal, d.Label)
}

func TestWordStressRoundIsRecordedOnceLikePairStats(t *testing.T) {
	x := newPairsApp(t, true, nil, nil)
	a, cmd := press(t, x.App, "m", "enter")
	missed := 0
	for i := range stressItems {
		a = run(t, a, cmd)
		right := prosodyItem(a).Drill != "stress:family" // the families are always wrong
		if !right {
			missed++
		}
		a = pressProsody(t, a, right)
		if i < stressItems-1 && (len(x.log.deltas) != 0 || len(x.pairs.rounds) != 0) {
			t.Fatalf("saved before the last word: %v %v", x.log.deltas, x.pairs.rounds)
		}
		x.clock.t = x.clock.t.Add(time.Minute)
		a, cmd = press(t, a, "enter")
	}
	if cmd != nil {
		t.Error("the result must not play anything")
	}
	score := stressItems - missed
	fam := drillOf(t, "stress:family")
	mustContain(t, pageText(a), fmt.Sprintf("%d/%d", score, stressItems), "saved in today's log",
		fam.Label, fmt.Sprintf("0/%d", missed), "Weakest: "+fam.Label)
	mustContain(t, statusBar(a), "n new round")

	wantDay := []domain.Day{{Date: domain.DateOf(fixedNow), Minutes: maxPairsMinutes,
		Activities: []string{fmt.Sprintf("Stress: %d/%d", score, stressItems)}}}
	if !reflect.DeepEqual(x.log.deltas, wantDay) {
		t.Errorf("recorded %v, want %v", x.log.deltas, wantDay)
	}
	if len(x.pairs.rounds) != 1 {
		t.Fatalf("results saved %d times, want once", len(x.pairs.rounds))
	}
	total := 0
	for _, st := range x.pairs.rounds[0] {
		total += st.Attempts
		if !strings.HasPrefix(st.Contrast, "stress:") {
			t.Errorf("saved %+v: want only stress drills", st)
		}
		if st.Contrast == "stress:family" && (st.Correct != 0 || st.Attempts != missed) {
			t.Errorf("stress:family saved as %+v, want 0/%d", st, missed)
		}
	}
	if total != stressItems {
		t.Errorf("saved %d attempts, want %d", total, stressItems)
	}

	// The next round starts with the drill just missed.
	a, _ = press(t, a, "n")
	mustContain(t, pageText(a), "Weakest first: "+fam.Label)
	a, _ = press(t, a, "enter")
	if prosodyItem(a).Drill != "stress:family" {
		t.Errorf("next round starts with %s, want stress:family", prosodyItem(a).Drill)
	}
}

func TestIntonationRoundIsRecordedAsIntonation(t *testing.T) {
	x := newPairsApp(t, true, nil, nil)
	a, cmd := press(t, x.App, "m", "m", "enter")
	for range toneItems {
		a = run(t, a, cmd)
		a = pressProsody(t, a, true)
		a, cmd = press(t, a, "enter")
	}
	mustContain(t, pageText(a), fmt.Sprintf("✓ %d/%d", toneItems, toneItems))
	want := []string{fmt.Sprintf("Intonation: %d/%d", toneItems, toneItems)}
	if len(x.log.deltas) != 1 || !reflect.DeepEqual(x.log.deltas[0].Activities, want) {
		t.Errorf("recorded %v, want %v", x.log.deltas, want)
	}
	if len(x.pairs.rounds) != 1 || len(x.pairs.rounds[0]) != 2 {
		t.Fatalf("saved %v, want the question and statement totals", x.pairs.rounds)
	}
}

// Prosody results share the pair stats; the minimal pairs must not
// change because of them.
func TestProsodyStatsLeaveMinimalPairsAsTheyWere(t *testing.T) {
	stats := map[string]domain.PairStat{
		"th-s":          {Contrast: "th-s", Attempts: 10, Correct: 2},
		"stress:family": {Contrast: "stress:family", Attempts: 10, Correct: 0},
		"tone:question": {Contrast: "tone:question", Attempts: 10, Correct: 0},
	}
	plain := newPairsApp(t, true, nil, map[string]domain.PairStat{"th-s": stats["th-s"]})
	mixed := newPairsApp(t, true, nil, stats)
	if got, want := pageText(mixed.App), pageText(plain.App); got != want {
		t.Errorf("minimal pairs intro with prosody stats:\n%s\nwant:\n%s", got, want)
	}
	if order := mixed.pairsOrder(); order[0].ID != "th-s" {
		t.Errorf("weakest contrast %s, want th-s", order[0].ID)
	}

	// A word-stress round adds to the shared totals and keeps the others.
	a, cmd := press(t, mixed.App, "m", "enter")
	if prosodyItem(a).Drill != "stress:family" {
		t.Errorf("stress round starts with %s, want the stored weakest", prosodyItem(a).Drill)
	}
	for range stressItems {
		a = run(t, a, cmd)
		a = pressProsody(t, a, true)
		a, cmd = press(t, a, "enter")
	}
	if st := a.pairStats["th-s"]; st != stats["th-s"] {
		t.Errorf("th-s %+v after a stress round, want %+v", st, stats["th-s"])
	}
	if st := a.pairStats["stress:family"]; st.Attempts <= 10 || st.Correct == 0 {
		t.Errorf("stress:family %+v: want the round added", st)
	}

	// A minimal-pairs round still saves only contrasts, as "Pairs: N/20".
	a, _ = press(t, a, "n", "m", "m")
	mustContain(t, pageText(a), "Pairs · Minimal pairs")
	a, cmd = press(t, a, "enter")
	for range pairsItems {
		a = run(t, a, cmd)
		a = pressAnswer(t, a, true)
		a, cmd = press(t, a, "enter")
	}
	last := lastRound(mixed.pairs.rounds)
	for _, st := range last {
		if strings.Contains(st.Contrast, ":") {
			t.Errorf("a pairs round saved %+v", st)
		}
	}
	acts := mixed.log.deltas[len(mixed.log.deltas)-1].Activities
	if !reflect.DeepEqual(acts, []string{"Pairs: 20/20"}) {
		t.Errorf("pairs round logged %v", acts)
	}
}

// lastRound is the last round saved.
func lastRound(rounds [][]domain.PairStat) []domain.PairStat {
	if len(rounds) == 0 {
		return nil
	}
	return rounds[len(rounds)-1]
}

func TestProsodyAbandonedRoundSavesNothing(t *testing.T) {
	x := newPairsApp(t, true, nil, nil)
	a, cmd := press(t, x.App, "m", "enter")
	for range 3 {
		a = run(t, a, cmd)
		a = pressProsody(t, a, true)
		a, cmd = press(t, a, "enter")
	}
	a, _ = press(t, a, "esc")
	if len(x.log.deltas) != 0 || len(x.pairs.rounds) != 0 {
		t.Errorf("esc midway saved %v %v", x.log.deltas, x.pairs.rounds)
	}
	a, _ = press(t, a, "shift+tab", "shift+tab", "shift+tab", "shift+tab") // Stats, Words, Speak, Pairs
	mustContain(t, pageText(a), fmt.Sprintf("Word 4 of %d", stressItems))
	// m is not a switch in the middle of a round.
	a, _ = press(t, a, "m")
	mustContain(t, pageText(a), "Pairs · Word stress")
}

func TestProsodyShowsAFailedSave(t *testing.T) {
	x := newPairsApp(t, true, nil, nil)
	x.pairs.err = errors.New("disk full")
	a, cmd := press(t, x.App, "m", "m", "enter")
	for range toneItems {
		a = run(t, a, cmd)
		a = pressProsody(t, a, true)
		a, cmd = press(t, a, "enter")
	}
	mustContain(t, pageText(a), "Could not save your results: disk full")
	if a.Err() != nil {
		t.Error("a failed stats save must not stop the app")
	}
}

func TestProsodyWithoutAudioSaysHowToTurnItOn(t *testing.T) {
	x := newPairsApp(t, false, nil, nil)
	a, _ := press(t, x.App, "m")
	mustContain(t, pageText(a), "Pairs · Word stress", "Audio is off: run `teacher doctor`", "teacher setup voice")
	a, cmd := press(t, a, "enter")
	if cmd != nil || a.prosody.phase != pairsIntro {
		t.Error("enter without audio must not start")
	}
	mustNotContain(t, statusBar(a), "enter start")
	mustContain(t, statusBar(a), "m intonation")
}

func TestPairsHelpTellsTheProsodyKeys(t *testing.T) {
	a := newApp(t, deck(), &recorder{}, Pairs, 80, 24)
	a, _ = press(t, a, "?")
	mustContain(t, pageText(a), "word stress", "intonation")
	if n := len(screenLines(a)); n > 24 {
		t.Errorf("help is %d rows, want it to fit 24", n)
	}
}

// Every page of both kinds fits 80×24, even a wrong answer with its tip
// and a result with every drill.
func TestProsodyPagesFit80x24(t *testing.T) {
	for _, keys := range [][]string{{"m"}, {"m", "m"}} {
		x := newPairsApp(t, true, nil, nil)
		a := send(t, x.App, tea.WindowSizeMsg{Width: 80, Height: 24})
		a, _ = press(t, a, keys...)
		pages := []string{screen(a)}
		a, cmd := press(t, a, "enter")
		for i := range a.prosody.items {
			a = run(t, a, cmd)
			if i == 0 {
				pages = append(pages, screen(a))
			}
			a = pressProsody(t, a, i%3 != 0)
			if i == 0 {
				pages = append(pages, screen(a))
			}
			a, cmd = press(t, a, "enter")
		}
		pages = append(pages, screen(a))
		for _, page := range pages {
			lines := strings.Split(page, "\n")
			if len(lines) > 24 || strings.TrimSpace(lines[len(lines)-2]) != "│" && strings.TrimSpace(lines[len(lines)-2]) != "" {
				t.Errorf("page does not fit 80×24 with room to spare:\n%s", page)
			}
			if testing.Verbose() {
				t.Log("\n" + page)
			}
		}
	}
}
