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

// pairsLog is a fake RecordPairsFunc that remembers every round saved.
type pairsLog struct {
	rounds [][]domain.PairStat
	err    error
}

func (l *pairsLog) record(r []domain.PairStat) error {
	l.rounds = append(l.rounds, r)
	return l.err
}

// pairsEnv is the Pairs screen with a fake speaker, the study log and the
// pair stats log.
type pairsEnv struct {
	App
	sp    *fakeSpeaker // nil when the app has no audio
	log   *studyLog
	pairs *pairsLog
	clock *testClock
}

// newPairsApp opens Pairs with the given installed voices and stats;
// audio is off when audio is false.
func newPairsApp(t *testing.T, audio bool, voices []string, stats map[string]domain.PairStat) pairsEnv {
	t.Helper()
	x := pairsEnv{log: &studyLog{}, pairs: &pairsLog{}, clock: &testClock{t: fixedNow}}
	cfg := Config{
		Cards: deck(), Save: (&recorder{}).save, Now: x.clock.now, Start: Pairs, MaxNew: DefaultMaxNew,
		Record: x.log.record, PairStats: stats, RecordPairs: x.pairs.record, Voices: voices,
	}
	if audio {
		x.sp = &fakeSpeaker{}
		cfg.Speaker = x.sp
	}
	x.App = send(t, NewApp(cfg), tea.WindowSizeMsg{Width: 120, Height: 40})
	return x
}

// pairItem is the item on screen.
func pairItem(a App) domain.PairItem { return a.pairs.items[a.pairs.pos] }

// contrastOf is the built-in contrast with id.
func contrastOf(t *testing.T, id string) domain.Contrast {
	t.Helper()
	for _, c := range domain.Contrasts() {
		if c.ID == id {
			return c
		}
	}
	t.Fatalf("no contrast %s", id)
	return domain.Contrast{}
}

// tipStart is the start of c's tip, which fits on one line before the
// page wraps it.
func tipStart(c domain.Contrast) string { return string([]rune(c.Tip)[:40]) }

// said runs cmd and checks the speaker said exactly want.
func said(t *testing.T, a App, cmd tea.Cmd, sp *fakeSpeaker, want ...spoken) App {
	t.Helper()
	a = run(t, a, cmd)
	if !reflect.DeepEqual(sp.said, want) {
		t.Errorf("said %+v, want %+v", sp.said, want)
	}
	sp.said = nil
	return a
}

// pressRight answers the item on screen right, or wrong when !right.
func pressAnswer(t *testing.T, a App, right bool) App {
	t.Helper()
	k := pairItem(a).Played
	if !right {
		k = 1 - k
	}
	a, cmd := press(t, a, fmt.Sprint(k+1))
	if cmd != nil {
		t.Fatal("answering must not play anything")
	}
	return a
}

func TestPairsIntroExplainsTheRound(t *testing.T) {
	x := newPairsApp(t, true, nil, nil)
	mustContain(t, pageText(x.App), "Pairs", "Minimal pairs", "choose which one", "20 words")
	mustContain(t, statusBar(x.App), "enter start")
	if len(x.sp.said) != 0 {
		t.Error("the intro must not play anything")
	}
}

func TestPairsPlaysTheWordAndReplays(t *testing.T) {
	x := newPairsApp(t, true, nil, nil)
	a, cmd := press(t, x.App, "enter")
	if len(a.pairs.items) != pairsItems {
		t.Fatalf("%d items, want %d", len(a.pairs.items), pairsItems)
	}
	it := pairItem(a)
	a = said(t, a, cmd, x.sp, spoken{it.Word(), speech.Options{}}) // played on arrival
	page := pageText(a)
	mustContain(t, page, "Word 1 of 20", "Which word did you hear?", "1 "+it.Pair[0], "2 "+it.Pair[1])
	mustContain(t, statusBar(a), "1 / 2 choose", "p again", "s slower")

	a, cmd = press(t, a, "p")
	a = said(t, a, cmd, x.sp, spoken{it.Word(), speech.Options{}})
	a, cmd = press(t, a, "s")
	said(t, a, cmd, x.sp, spoken{it.Word(), speech.Options{Slow: true}})
}

func TestPairsAlternatesTheInstalledVoices(t *testing.T) {
	x := newPairsApp(t, true, []string{"en_US-amy-medium", "en_US-lessac-medium"}, nil)
	a, cmd := press(t, x.App, "enter")
	a = said(t, a, cmd, x.sp, spoken{pairItem(a).Word(), speech.Options{Voice: "en_US-amy-medium"}})
	a = pressAnswer(t, a, true)
	a, cmd = press(t, a, "enter")
	a = said(t, a, cmd, x.sp, spoken{pairItem(a).Word(), speech.Options{Voice: "en_US-lessac-medium"}})
	// A replay keeps the item's voice.
	a, cmd = press(t, a, "s")
	said(t, a, cmd, x.sp, spoken{pairItem(a).Word(), speech.Options{Slow: true, Voice: "en_US-lessac-medium"}})

	// One voice: the default, with no name.
	y := newPairsApp(t, true, []string{"en_US-lessac-medium"}, nil)
	b, cmd := press(t, y.App, "enter")
	said(t, b, cmd, y.sp, spoken{pairItem(b).Word(), speech.Options{}})
}

func TestPairsChecksAgainstTheWordPlayed(t *testing.T) {
	x := newPairsApp(t, true, nil, nil)
	a, cmd := press(t, x.App, "enter")
	a = run(t, a, cmd)
	x.sp.said = nil

	right := pairItem(a)
	a = pressAnswer(t, a, true)
	mustContain(t, pageText(a), "✓ "+right.Word())
	mustNotContain(t, pageText(a), "You heard")
	mustContain(t, statusBar(a), "enter next")

	// After the feedback, 1 and 2 play each word to compare them.
	a, cmd = press(t, a, "1")
	a = said(t, a, cmd, x.sp, spoken{right.Pair[0], speech.Options{}})
	a, cmd = press(t, a, "2")
	a = said(t, a, cmd, x.sp, spoken{right.Pair[1], speech.Options{}})
	if a.pairs.score() != 1 {
		t.Errorf("score %d after one right answer and two comparisons, want 1", a.pairs.score())
	}

	a, cmd = press(t, a, "enter")
	a = run(t, a, cmd)
	wrong := pairItem(a)
	a = pressAnswer(t, a, false)
	tip := contrastOf(t, wrong.Contrast)
	mustContain(t, pageText(a), "✗ You heard: "+wrong.Word(), tip.Label, tipStart(tip))
	if a.pairs.score() != 1 {
		t.Errorf("score %d, want 1", a.pairs.score())
	}
}

func TestPairsResultIsRecordedOnceWithTheStats(t *testing.T) {
	x := newPairsApp(t, true, nil, nil)
	a, cmd := press(t, x.App, "enter")
	var wrongs []bool
	for i := range pairsItems {
		a = run(t, a, cmd)
		right := pairItem(a).Contrast != "th-s" // th-s is always wrong
		wrongs = append(wrongs, !right)
		a = pressAnswer(t, a, right)
		if i < pairsItems-1 && (len(x.log.deltas) != 0 || len(x.pairs.rounds) != 0) {
			t.Fatalf("saved before the last item: %v %v", x.log.deltas, x.pairs.rounds)
		}
		x.clock.t = x.clock.t.Add(time.Minute)
		a, cmd = press(t, a, "enter")
	}
	if cmd != nil {
		t.Error("the result must not play anything")
	}
	missed := 0
	for _, w := range wrongs {
		if w {
			missed++
		}
	}
	score := pairsItems - missed

	page := pageText(a)
	thS := contrastOf(t, "th-s")
	mustContain(t, page, fmt.Sprintf("%d/20", score), "saved in today's log",
		thS.Label, fmt.Sprintf("0/%d", missed), "Weakest: "+thS.Label, tipStart(thS))
	mustContain(t, statusBar(a), "n new round")

	// 20 minutes open, capped like a quiz.
	wantDay := []domain.Day{{Date: domain.DateOf(fixedNow), Minutes: maxPairsMinutes,
		Activities: []string{fmt.Sprintf("Pairs: %d/20", score)}}}
	if !reflect.DeepEqual(x.log.deltas, wantDay) {
		t.Errorf("recorded %v, want %v", x.log.deltas, wantDay)
	}
	if len(x.pairs.rounds) != 1 {
		t.Fatalf("pair stats saved %d times, want once", len(x.pairs.rounds))
	}
	total := 0
	for _, st := range x.pairs.rounds[0] {
		total += st.Attempts
		if st.Contrast == "th-s" && (st.Correct != 0 || st.Attempts != missed) {
			t.Errorf("th-s saved as %+v, want 0/%d", st, missed)
		}
	}
	if total != pairsItems {
		t.Errorf("saved %d attempts, want %d", total, pairsItems)
	}

	// Leaving and coming back records nothing more.
	a, _ = press(t, a, "tab", "shift+tab", "enter", "1")
	if len(x.log.deltas) != 1 || len(x.pairs.rounds) != 1 {
		t.Errorf("recorded %d days, %d rounds; want once", len(x.log.deltas), len(x.pairs.rounds))
	}

	// The next round starts with the contrast just missed.
	a, _ = press(t, a, "n")
	if a.pairs.phase != pairsIntro {
		t.Fatalf("n: phase %v, want the intro", a.pairs.phase)
	}
	mustContain(t, pageText(a), "Weakest first: "+thS.Label)
	a, _ = press(t, a, "enter")
	if pairItem(a).Contrast != "th-s" {
		t.Errorf("next round starts with %s, want th-s", pairItem(a).Contrast)
	}
}

func TestPairsAbandonedRoundSavesNothing(t *testing.T) {
	x := newPairsApp(t, true, nil, nil)
	a, cmd := press(t, x.App, "enter")
	for range 5 {
		a = run(t, a, cmd)
		a = pressAnswer(t, a, true)
		a, cmd = press(t, a, "enter")
	}
	a, _ = press(t, a, "esc")
	if a.Screen() != Home || len(x.log.deltas) != 0 || len(x.pairs.rounds) != 0 {
		t.Errorf("esc midway: screen %v, saved %v %v", a.Screen(), x.log.deltas, x.pairs.rounds)
	}
	a, _ = press(t, a, "shift+tab", "shift+tab", "shift+tab", "shift+tab") // Stats, Words, Speak, Pairs
	mustContain(t, pageText(a), "Word 6 of 20")
}

func TestPairsShowsAFailedSave(t *testing.T) {
	x := newPairsApp(t, true, nil, nil)
	x.pairs.err = errors.New("disk full")
	a, cmd := press(t, x.App, "enter")
	for range pairsItems {
		a = run(t, a, cmd)
		a = pressAnswer(t, a, true)
		a, cmd = press(t, a, "enter")
	}
	mustContain(t, pageText(a), "20/20", "Could not save your pair results: disk full")
	if a.Err() != nil {
		t.Error("a failed stats save must not stop the app")
	}
}

func TestPairsWithoutAudioSaysHowToTurnItOn(t *testing.T) {
	x := newPairsApp(t, false, nil, nil)
	mustContain(t, pageText(x.App), "Audio is off: run `teacher doctor`", "teacher setup voice")
	a, cmd := press(t, x.App, "enter")
	if cmd != nil || a.pairs.phase != pairsIntro {
		t.Error("enter without audio must not start")
	}
	mustNotContain(t, statusBar(a), "enter start")
}

func TestPairsHelpAndTab(t *testing.T) {
	a := newApp(t, deck(), &recorder{}, Pairs, 80, 24)
	if !strings.Contains(screenLines(a)[0], "Listen Pairs Speak") {
		t.Errorf("tab bar %q: want Pairs after Listen", screenLines(a)[0])
	}
	a, _ = press(t, a, "?")
	mustContain(t, pageText(a), "Pairs", "which word you heard", "ctrl+c")
}
