package tui

import (
	"fmt"
	"math/rand/v2"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/kevnnard/english-teacher/internal/domain"
	"github.com/kevnnard/english-teacher/internal/speech"
)

// listenEnv is the Listen screen with a fake speaker, a study log and the
// card saver, so tests can check dictation never touches the schedules.
type listenEnv struct {
	App
	sp    *fakeSpeaker // nil when the app has no audio
	log   *studyLog
	rec   *recorder
	clock *testClock
	cards []domain.Card
}

// newListenApp opens Listen over cards, with audio when audio is set.
func newListenApp(t *testing.T, cards []domain.Card, audio bool) listenEnv {
	t.Helper()
	x := listenEnv{log: &studyLog{}, rec: &recorder{}, clock: &testClock{t: fixedNow}, cards: cards}
	cfg := Config{
		Cards: cards, Save: x.rec.save, Now: x.clock.now, Start: Listen, MaxNew: DefaultMaxNew,
		Record: x.log.record,
	}
	if audio {
		x.sp = &fakeSpeaker{}
		cfg.Speaker = x.sp
	}
	x.App = send(t, NewApp(cfg), tea.WindowSizeMsg{Width: 120, Height: 40})
	return x
}

// untouched checks dictation saved no card and changed no schedule.
func (x listenEnv) untouched(t *testing.T, a App) {
	t.Helper()
	if len(x.rec.saved) != 0 {
		t.Errorf("dictation saved %d cards", len(x.rec.saved))
	}
	if !reflect.DeepEqual(a.cards, x.cards) {
		t.Error("dictation changed the cards in memory")
	}
}

// dictDeck has twelve short examples, so a dictation has ten items.
func dictDeck() []domain.Card {
	var cards []domain.Card
	for i := range 12 {
		w := fmt.Sprintf("word%d", i)
		cards = append(cards, word(w, "", "palabra", fmt.Sprintf("I see the word%d, every day.", i)))
	}
	return cards
}

// newRand is a seeded random source, as startDictation makes one.
func newRand(seed uint64) *rand.Rand { return rand.New(rand.NewPCG(seed, 1)) }

// item is the sentence on screen.
func item(a App) string { return a.dictation.items[a.dictation.pos] }

// heard runs cmd, as Bubble Tea would, and checks what the speaker said.
func heard(t *testing.T, a App, cmd tea.Cmd, sp *fakeSpeaker, text string, slow bool) App {
	t.Helper()
	a = run(t, a, cmd)
	want := []spoken{{text, speech.Options{Slow: slow}}}
	if !reflect.DeepEqual(sp.said, want) {
		t.Errorf("said %+v, want %+v", sp.said, want)
	}
	sp.said = nil
	return a
}

func TestListenIntroOffersADictationFromYourCards(t *testing.T) {
	x := newListenApp(t, deck(), true)
	mustContain(t, pageText(x.App), "Listen", "Dictation", "7 sentences from your cards",
		"never changes your review dates")
	mustContain(t, statusBar(x.App), "enter start")
	if len(x.sp.said) != 0 {
		t.Error("the intro must not play anything")
	}
}

func TestListenPlaysOnArrivalAndReplaysWithCtrlKeys(t *testing.T) {
	x := newListenApp(t, deck(), true)
	a, cmd := press(t, x.App, "enter")
	first := item(a)
	a = heard(t, a, cmd, x.sp, first, false) // played on arrival
	mustContain(t, pageText(a), "Sentence 1 of 7", "Type what you hear")
	mustContain(t, statusBar(a), "ctrl+p again", "ctrl+s slower", "enter check")
	mustNotContain(t, pageText(a), first) // the answer is not on screen

	a, cmd = press(t, a, "ctrl+p")
	a = heard(t, a, cmd, x.sp, first, false)
	a, cmd = press(t, a, "ctrl+s")
	a = heard(t, a, cmd, x.sp, first, true)

	// p and s are letters of the answer.
	a, cmd = press(t, a, "p", "s")
	if a.dictation.input != "ps" || len(x.sp.said) != 0 {
		t.Fatalf("p s while typing: input %q, said %q", a.dictation.input, x.sp.texts())
	}
	if cmd != nil {
		t.Error("a letter must not start a command")
	}
	x.untouched(t, a)
}

func TestListenChecksLikeATypedAnswer(t *testing.T) {
	x := newListenApp(t, deck(), true)
	a, _ := press(t, x.App, "enter")

	// Case and punctuation never make it wrong.
	s := item(a)
	typed := strings.ToLower(strings.NewReplacer(",", "", ".", "", "!", "", "?", "").Replace(s))
	a = typeText(t, a, typed)
	a, _ = press(t, a, "enter")
	mustContain(t, pageText(a), "✓ correct", s)
	mustContain(t, statusBar(a), "enter next")

	// The next sentence plays on arrival.
	a, cmd := press(t, a, "enter")
	a = heard(t, a, cmd, x.sp, item(a), false)

	// A small typo is almost, and still counts.
	s = item(a)
	a = typeText(t, a, strings.TrimRight(s, ".!?")+"x")
	a, _ = press(t, a, "enter")
	mustContain(t, pageText(a), "✓ almost", "[x→]")

	// A wrong answer shows the sentence and the words missed.
	a, cmd = press(t, a, "enter")
	a = run(t, a, cmd)
	s = item(a)
	a = typeText(t, a, "the cat")
	a, _ = press(t, a, "enter")
	page := pageText(a)
	mustContain(t, page, "✗ not quite", "the sentence is", s, "Missed:")

	// An empty answer means "I don't know": it shows the sentence.
	a, cmd = press(t, a, "enter")
	a = run(t, a, cmd)
	s = item(a)
	a, _ = press(t, a, "enter")
	mustContain(t, pageText(a), "✗ you wrote nothing", s)

	want := []bool{true, true, false, false}
	var got []bool
	for _, ans := range a.dictation.answers {
		got = append(got, ans.right)
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("right = %v, want %v", got, want)
	}
}

func TestListenMissedWordsAreMarked(t *testing.T) {
	got := missedWords("hello are", "Hello, how are you?")
	if !reflect.DeepEqual(got, []string{"how", "you"}) {
		t.Errorf("missed %q, want [how you]", got)
	}
	if got := missedWords("I'm from argentina", "I’m from Argentina."); len(got) != 0 {
		t.Errorf("missed %q, want none", got)
	}
}

func TestListenRunsTenItemsAndRecordsTheResultOnce(t *testing.T) {
	x := newListenApp(t, dictDeck(), true)
	a, cmd := press(t, x.App, "enter")
	if len(a.dictation.items) != dictationItems {
		t.Fatalf("%d items, want %d", len(a.dictation.items), dictationItems)
	}
	var missed []string
	for i := range dictationItems {
		a = run(t, a, cmd) // played on arrival
		mustContain(t, pageText(a), fmt.Sprintf("Sentence %d of 10", i+1))
		if i < 3 {
			missed = append(missed, item(a))
			a, _ = press(t, a, "enter") // I don't know
		} else {
			a = typeText(t, a, item(a))
			a, _ = press(t, a, "enter")
		}
		if i < dictationItems-1 && len(x.log.deltas) != 0 {
			t.Fatalf("recorded %v before the last item", x.log.deltas)
		}
		x.clock.t = x.clock.t.Add(5 * time.Minute)
		a, cmd = press(t, a, "enter")
	}
	if cmd != nil {
		t.Error("the result must not play anything")
	}

	page := pageText(a)
	mustContain(t, page, "7/10", "saved in today's log", "To practice")
	mustContain(t, page, missed...)
	mustContain(t, statusBar(a), "n new dictation")

	// 50 minutes open, capped like a quiz.
	want := []domain.Day{{Date: domain.DateOf(fixedNow), Minutes: maxListenMinutes, Activities: []string{"Listen: 7/10"}}}
	if !reflect.DeepEqual(x.log.deltas, want) {
		t.Errorf("recorded %v, want %v", x.log.deltas, want)
	}

	// Leaving and coming back records nothing more.
	a, _ = press(t, a, "tab", "shift+tab", "enter", "x")
	if len(x.log.deltas) != 1 {
		t.Errorf("recorded %d times, want once", len(x.log.deltas))
	}
	x.untouched(t, a)

	a, _ = press(t, a, "n")
	if a.dictation.phase != dictationIntro {
		t.Errorf("n: phase %v, want the intro", a.dictation.phase)
	}
}

func TestListenWithFewExamplesSaysHowToGetMore(t *testing.T) {
	cards := []domain.Card{
		word("hello", "", "Hola", "Hello, how are you?"),
		word("hi", "", "Hola", ""),
		word("long", "", "Largo", "This example sentence is far too long for a beginner to type from listening alone."),
		word("bye", "", "Adiós", "Bye!"),
	}
	x := newListenApp(t, cards, true)
	mustContain(t, pageText(x.App), "Not enough sentences yet",
		"Add words with an example sentence", "study them in Review")
	a, cmd := press(t, x.App, "enter")
	if cmd != nil || a.dictation.phase != dictationIntro || len(x.sp.said) != 0 {
		t.Errorf("enter started a dictation of %d sentences", len(a.dictation.items))
	}
	mustNotContain(t, statusBar(a), "enter start")
}

func TestListenWithoutAudioSaysHowToTurnItOn(t *testing.T) {
	x := newListenApp(t, deck(), false)
	mustContain(t, pageText(x.App), "Audio is off: run `teacher doctor`", "teacher setup voice")
	a, cmd := press(t, x.App, "enter")
	if cmd != nil || a.dictation.phase != dictationIntro {
		t.Error("enter without audio must not start")
	}
	mustNotContain(t, statusBar(a), "enter start")
}

func TestListenItemsAreShortUniqueAndSeenFirst(t *testing.T) {
	cards := deck() // five studied, two new; all short
	prod := cards[0]
	prod.Direction = domain.Production
	cards = append(cards, prod,
		word("long", "", "Largo", "This example sentence is far too long for a beginner to type from listening alone."))

	items := dictationSentences(cards, newRand(1))
	if len(items) != 7 {
		t.Fatalf("items %q, want the 7 short examples once each", items)
	}
	studied := map[string]bool{}
	for _, c := range deck() {
		if c.LastReviewed != nil {
			studied[c.Example] = true
		}
	}
	for i, s := range items {
		if want := i < len(studied); studied[s] != want {
			t.Errorf("item %d %q: studied %v, want studied ones first", i, s, studied[s])
		}
	}
	if again := dictationSentences(cards, newRand(1)); !slices.Equal(again, items) {
		t.Errorf("same seed gave %q then %q", items, again)
	}
	if len(dictationSentences(dictDeck(), newRand(1))) != dictationItems {
		t.Errorf("want at most %d items", dictationItems)
	}
}

func TestTabBarFitsEightyColumns(t *testing.T) {
	for _, s := range screens {
		a := newApp(t, deck(), &recorder{}, s, 80, 24)
		top := screenLines(a)[0]
		if w := ansi.StringWidth(top); w > 80 {
			t.Errorf("%v: top bar is %d wide", s, w)
		}
		if s == Review {
			continue // the page number takes the room: only the active tab
		}
		var names []string
		for _, n := range screens {
			names = append(names, n.String())
		}
		mustContain(t, top, strings.Join(names, " "))
	}
}
