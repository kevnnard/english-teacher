package tui

import (
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/kevnnard/english-teacher/internal/domain"
	"github.com/kevnnard/english-teacher/internal/srs"
)

// --- recognition (EN → ES) ---

func TestReviewFrontShowsWordAndPronunciationOnly(t *testing.T) {
	a := newApp(t, deck(), &recorder{}, Review, 80, 24)

	v := screen(a)

	mustContain(t, v, "Recognize · English → Spanish", "hello", "/həˈloʊ/", "1 / 5")
	mustNotContain(t, v, "Hola", "Hello, how are you?", "good")
	mustContain(t, statusBar(a), "space reveal")
}

func TestReviewSpaceRevealsMeaningAndExample(t *testing.T) {
	a := newApp(t, deck(), &recorder{}, Review, 80, 24)

	a, _ = press(t, a, "space")

	mustContain(t, screen(a), "Hola", `"Hello, how are you?"`, "1 again", "2 hard", "3 good", "4 easy")
	mustContain(t, statusBar(a), "1-4 grade")
}

func TestReviewGradeBeforeRevealIsIgnored(t *testing.T) {
	rec := &recorder{}
	a := newApp(t, deck(), rec, Review, 80, 24)

	a, _ = press(t, a, "3")

	if len(rec.saved) != 0 {
		t.Errorf("saved %d cards before reveal, want 0", len(rec.saved))
	}
	mustContain(t, screen(a), "1 / 5")
}

func TestReviewKeys1To4MapToSM2Grades(t *testing.T) {
	for n := 1; n <= 4; n++ {
		t.Run(fmt.Sprint(n), func(t *testing.T) {
			rec := &recorder{}
			a := newApp(t, deck(), rec, Review, 80, 24)

			a, _ = press(t, a, "space", fmt.Sprint(n))

			ans, _ := domain.NewAnswer(n)
			want := srs.Review(deck()[0], ans.Grade(), fixedNow)
			if len(rec.saved) != 1 || !reflect.DeepEqual(rec.saved[0], want) {
				t.Fatalf("saved %+v, want %+v", rec.saved, want)
			}
			mustContain(t, screen(a), "hi", "2 / 5")
		})
	}
}

func TestReviewIgnoresOldGradeKeys(t *testing.T) {
	rec := &recorder{}
	a := newApp(t, deck(), rec, Review, 80, 24)

	a, _ = press(t, a, "space", "0", "5")

	if len(rec.saved) != 0 {
		t.Errorf("0 and 5 are not grades any more, saved %d", len(rec.saved))
	}
	mustContain(t, screen(a), "1 / 5")
}

// --- undo ---

func TestUndoRestoresTheLastGrade(t *testing.T) {
	rec := &recorder{}
	a := newApp(t, deck(), rec, Review, 80, 24)
	a, _ = press(t, a, "space", "1")
	mustContain(t, statusBar(a), "✗ 1", "u undo")

	a, _ = press(t, a, "u")

	if len(rec.saved) != 2 || !reflect.DeepEqual(rec.saved[1], deck()[0]) {
		t.Fatalf("undo should save the card as it was before the grade, saved %+v", rec.saved)
	}
	v := screen(a)
	mustContain(t, v, "hello", "1 / 5")
	mustNotContain(t, v, "Hola")
	mustNotContain(t, statusBar(a), "✗")

	// The card can be graded again, from its original state.
	a, _ = press(t, a, "space", "3")
	want := srs.Review(deck()[0], domain.Good.Grade(), fixedNow)
	if !reflect.DeepEqual(rec.saved[2], want) {
		t.Errorf("regrade saved %+v, want %+v", rec.saved[2], want)
	}
}

func TestUndoOnlyReachesTheLastGrade(t *testing.T) {
	rec := &recorder{}
	a := newApp(t, deck(), rec, Review, 80, 24)
	a, _ = press(t, a, "space", "3", "space", "3")

	a, _ = press(t, a, "u", "u")

	if len(rec.saved) != 3 {
		t.Fatalf("want 2 grades and 1 undo saved, got %d saves", len(rec.saved))
	}
	if rec.saved[2].Word != "hi" {
		t.Errorf("undo restored %q, want the last graded card hi", rec.saved[2].Word)
	}
	mustContain(t, screen(a), "2 / 5")
}

func TestUndoBeforeAnyGradeDoesNothing(t *testing.T) {
	rec := &recorder{}
	a := newApp(t, deck(), rec, Review, 80, 24)

	a, _ = press(t, a, "u")

	if len(rec.saved) != 0 {
		t.Errorf("saved %d, want nothing to undo", len(rec.saved))
	}
	mustContain(t, screen(a), "1 / 5")
}

// --- production (ES → EN, typed) ---

func TestProductionPromptsInSpanishWithAGappedExample(t *testing.T) {
	a := newApp(t, prodDeck(), &recorder{}, Review, 80, 24)

	v := screen(a)

	mustContain(t, v, "Produce · Spanish → English", "Mucho gusto", `"________________, I'm Kevin."`, "›")
	mustNotContain(t, v, "nice to meet you", "Nice to meet you")
}

func TestProductionTypingCapturesEveryLetter(t *testing.T) {
	a := newApp(t, prodDeck(), &recorder{}, Review, 80, 24)

	a = typeText(t, a, "q u c ? : 1")
	if a.Screen() != Review || a.help || a.palette.open {
		t.Fatalf("letters should go to the answer, not to global keys")
	}
	mustContain(t, screen(a), "› q u c ? : 1")

	a, _ = press(t, a, "backspace", "backspace")
	mustContain(t, screen(a), "› q u c ? :")
	mustNotContain(t, screen(a), ": 1")
}

func TestProductionExactAnswerAutoGradesGood(t *testing.T) {
	rec := &recorder{}
	a := newApp(t, prodDeck(), rec, Review, 80, 24)

	a = typeText(t, a, "Nice to meet you!")
	a, _ = press(t, a, "enter")

	want := srs.Review(prodDeck()[1], domain.Good.Grade(), fixedNow)
	if len(rec.saved) != 1 || !reflect.DeepEqual(rec.saved[0], want) {
		t.Fatalf("exact answer should save grade good: %+v", rec.saved)
	}
	mustContain(t, screen(a), "✓")
	mustContain(t, statusBar(a), "4 easy")

	a, _ = press(t, a, "enter")
	mustContain(t, screen(a), "Done.")
}

func TestProductionExactAnswerUpgradesToEasy(t *testing.T) {
	rec := &recorder{}
	a := newApp(t, prodDeck(), rec, Review, 80, 24)

	a = typeText(t, a, "nice to meet you")
	a, _ = press(t, a, "enter", "4")

	want := srs.Review(prodDeck()[1], domain.Easy.Grade(), fixedNow)
	if len(rec.saved) != 2 || !reflect.DeepEqual(rec.saved[1], want) {
		t.Fatalf("4 should regrade the original card as easy: %+v", rec.saved)
	}
	mustContain(t, screen(a), "Done.")
	mustContain(t, statusBar(a), "✓ 1", "✗ 0")
}

func TestProductionNearMissShowsLetterDiff(t *testing.T) {
	a := newApp(t, prodDeck(), &recorder{}, Review, 80, 24)

	a = typeText(t, a, "nice to meat you")
	a, _ = press(t, a, "enter")

	mustContain(t, screen(a), "nice to me[a→e]t you", "✗ almost · 1 letter", "1 again", "2 hard")
	mustNotContain(t, screen(a), "3 good", "4 easy")

	content := a.View().Content
	r := a.st.main
	if !strings.Contains(content, r.deleted.Render("a")) || !strings.Contains(content, r.inserted.Render("e")) {
		t.Errorf("deleted letters should be struck and red, inserted underlined and green:\n%q", content)
	}
}

func TestProductionNearMissAllowsOnlyAgainOrHard(t *testing.T) {
	for _, tt := range []struct {
		key  string
		want domain.Answer
	}{{"1", domain.Again}, {"2", domain.Hard}} {
		t.Run(tt.key, func(t *testing.T) {
			rec := &recorder{}
			a := newApp(t, prodDeck(), rec, Review, 80, 24)
			a = typeText(t, a, "nice to meat you")

			a, _ = press(t, a, "enter", "3", "4")
			if len(rec.saved) != 0 {
				t.Fatalf("3 and 4 should be ignored on a near miss, saved %+v", rec.saved)
			}
			a, _ = press(t, a, tt.key)

			want := srs.Review(prodDeck()[1], tt.want.Grade(), fixedNow)
			want.LastWrong = "nice to meat you"
			if len(rec.saved) != 1 || !reflect.DeepEqual(rec.saved[0], want) {
				t.Fatalf("saved %+v, want %+v", rec.saved, want)
			}
			mustContain(t, screen(a), "Done.")
		})
	}
}

func TestProductionWrongAnswerShowsExpectedAndOnlyAgain(t *testing.T) {
	rec := &recorder{}
	a := newApp(t, prodDeck(), rec, Review, 80, 24)
	a = typeText(t, a, "goodbye")

	a, _ = press(t, a, "enter", "2")

	v := screen(a)
	mustContain(t, v, "✗", "nice to meet you", "1 again")
	mustNotContain(t, v, "almost", "2 hard")
	if len(rec.saved) != 0 {
		t.Fatalf("2 should be ignored on a wrong answer, saved %+v", rec.saved)
	}

	a, _ = press(t, a, "1")
	if len(rec.saved) != 1 || rec.saved[0].LastWrong != "goodbye" || rec.saved[0].TimesFailed != 1 {
		t.Errorf("saved %+v, want a failure with LastWrong goodbye", rec.saved)
	}
}

func TestProductionEmptyAnswerIsWrong(t *testing.T) {
	a := newApp(t, prodDeck(), &recorder{}, Review, 80, 24)

	a, _ = press(t, a, "enter")

	mustContain(t, screen(a), "nice to meet you", "1 again")
	mustNotContain(t, screen(a), "almost")
}

func TestUndoInProductionReturnsToTyping(t *testing.T) {
	rec := &recorder{}
	cards := prodDeck()
	cards[1].LastWrong = "nice to mit you" // from an earlier session
	a := newApp(t, cards, rec, Review, 80, 24)
	a = typeText(t, a, "nice to meat you")
	a, _ = press(t, a, "enter", "1")

	a, _ = press(t, a, "u")

	if len(rec.saved) != 2 || !reflect.DeepEqual(rec.saved[1], cards[1]) {
		t.Fatalf("undo should restore the card and its last wrong answer: %+v", rec.saved)
	}
	mustContain(t, screen(a), "Mucho gusto", "›")
	mustNotContain(t, screen(a), "meat", "Done.")
}

func TestReviewMixesBothDirections(t *testing.T) {
	cards := append(deck(), prodDeck()...)
	a := newApp(t, cards, &recorder{}, Review, 80, 24)

	mustContain(t, screen(a), "1 / 6")
	mustContain(t, statusBar(a), "6 due")
}

// --- coach ---

func TestCoachShowsHistoryOfTheCurrentCard(t *testing.T) {
	cards := prodDeck()
	cards[1].TimesCorrect, cards[1].TimesFailed, cards[1].LastWrong = 1, 2, "nice to mit you"
	last := fixedNow.AddDate(0, 0, -2)
	cards[1].LastReviewed = &last

	a := newApp(t, cards, &recorder{}, Review, 160, 24)

	v := screen(a)
	mustContain(t, v, "Coach", "production", "seen 3 · ✓ 1 · ✗ 2", `last: wrote "nice to mit you"`)
	mustNotContain(t, v, "Tip", "Use it today")
}

func TestCoachForANewCard(t *testing.T) {
	a := newApp(t, prodDeck(), &recorder{}, Review, 160, 24)

	mustContain(t, screen(a), "seen 0")
	mustNotContain(t, screen(a), "last: wrote")
}

func TestCoachOnlyInWideLayout(t *testing.T) {
	a := newApp(t, prodDeck(), &recorder{}, Review, 120, 24)

	mustNotContain(t, screen(a), "Coach", "seen")
}

// --- session end ---

// finish grades every card of deck(): hello and hi fail, the rest pass.
func finish(t *testing.T, a App, clock *testClock) App {
	t.Helper()
	for _, k := range []string{"1", "1", "3", "3", "4"} {
		clock.t = clock.t.Add(20 * time.Second)
		a, _ = press(t, a, "space", k)
	}
	return a
}

func TestSessionEndSummary(t *testing.T) {
	a, clock := newAppClock(t, deck(), &recorder{}, Review, 80, 24)

	a = finish(t, a, clock)

	v := screen(a)
	mustContain(t, v, "Done. 5 reviewed in 2 min.", "✓ 3", "✗ 2", "Tricky today", "hello", "hi",
		"Next review", "tomorrow · 5 cards", "r review the 2 mistakes again", "enter home")
	mustContain(t, statusBar(a), "r again", "enter home")

	a, _ = press(t, a, "enter")
	if a.Screen() != Home {
		t.Errorf("enter on the session end should go Home, got %v", a.Screen())
	}
	mustContain(t, screen(a), "No reviews due — come back tomorrow")
}

func TestSessionEndListsAtMostThreeTrickyWords(t *testing.T) {
	a, clock := newAppClock(t, deck(), &recorder{}, Review, 80, 24)
	for range 5 {
		clock.t = clock.t.Add(time.Second)
		a, _ = press(t, a, "space", "1")
	}

	v := screen(a)
	mustContain(t, v, "Done. 5 reviewed in 1 min.", "hello", "hi", "nice to meet you")
	mustNotContain(t, v, "Argentina", "Brazil")
}

func TestSessionEndWithoutMistakes(t *testing.T) {
	a, clock := newAppClock(t, deck(), &recorder{}, Review, 80, 24)
	for range 5 {
		clock.t = clock.t.Add(time.Second)
		a, _ = press(t, a, "space", "3")
	}

	v := screen(a)
	mustNotContain(t, v, "Tricky today", "mistakes again")
	mustContain(t, v, "▸ enter home")
	if _, cmd := press(t, a, "r"); cmd != nil {
		t.Error("r without mistakes should do nothing")
	}
}

func TestReviewMistakesAgainDoesNotChangeSchedules(t *testing.T) {
	rec := &recorder{}
	a, clock := newAppClock(t, deck(), rec, Review, 80, 24)
	a = finish(t, a, clock)
	saves := len(rec.saved)

	a, _ = press(t, a, "r")
	mustContain(t, screen(a), "hello", "1 / 2")
	a, _ = press(t, a, "space", "3", "space", "1")

	if len(rec.saved) != saves {
		t.Errorf("the mistakes round saved %d cards, want none", len(rec.saved)-saves)
	}
	v := screen(a)
	mustContain(t, v, "Done. 5 reviewed", "✓ 3", "✗ 2", "Mistakes round", "✓ 1", "✗ 1")
	mustContain(t, statusBar(a), "✓ 3", "✗ 2")
}

func TestUndoAfterTheLastCardReopensIt(t *testing.T) {
	rec := &recorder{}
	a, clock := newAppClock(t, deck(), rec, Review, 80, 24)
	a = finish(t, a, clock)

	a, _ = press(t, a, "u")

	mustContain(t, screen(a), "Brazil", "5 / 5")
	mustNotContain(t, screen(a), "Done.")
}

// --- session cap ---

func TestSessionCapsNewCards(t *testing.T) {
	var cards []domain.Card
	for i := range 25 {
		cards = append(cards, word(fmt.Sprintf("word%02d", i), "", "", ""))
	}
	cards = append(cards, deck()[0]) // one due card

	for _, tt := range []struct{ maxNew, want int }{{DefaultMaxNew, 21}, {3, 4}, {0, 1}} {
		a := NewApp(Config{Cards: cards, Save: (&recorder{}).save, Now: (&testClock{fixedNow}).now, MaxNew: tt.maxNew, Start: Review})

		mustContain(t, screen(a), fmt.Sprintf("1 / %d", tt.want))
		mustContain(t, screen(a), "hello") // due cards come first
	}
}

// --- errors and empty sessions ---

func TestReviewProgressSurvivesLeavingTheScreen(t *testing.T) {
	a := newApp(t, deck(), &recorder{}, Review, 80, 24)

	a, _ = press(t, a, "space", "3", "esc", "enter")

	if a.Screen() != Review {
		t.Fatalf("enter on Home should resume the review, got %v", a.Screen())
	}
	mustContain(t, screen(a), "2 / 5")
	a, _ = press(t, a, "esc")
	mustContain(t, screen(a), "Review 4 words")
}

func TestReviewWithNothingDue(t *testing.T) {
	a := newApp(t, deck()[5:], &recorder{}, Review, 80, 24)

	mustContain(t, screen(a), "No reviews due — come back tomorrow")
}

func TestReviewSaveErrorIsShownAndStopsTheApp(t *testing.T) {
	rec := &recorder{err: errors.New("disk full")}
	a := newApp(t, deck(), rec, Review, 80, 24)

	a, _ = press(t, a, "space", "3")

	mustContain(t, screen(a), "disk full")
	if a.Err() == nil {
		t.Error("Err() = nil, want the save error")
	}
	if _, cmd := press(t, a, "space"); !isQuit(cmd) {
		t.Error("any key after a save error should quit")
	}
}
