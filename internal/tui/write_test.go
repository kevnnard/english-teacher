package tui

import (
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/kevnnard/english-teacher/internal/domain"
	"github.com/kevnnard/english-teacher/internal/teacher"
)

// sampleText has the three major errors and the minor one of sampleCorrection.
const sampleText = "Hi. I has 30 years and I like it. I am teacher. I have two dog."

func sampleCorrection() teacher.Correction {
	return teacher.Correction{
		Corrected: "Hi! I am 30 years old and I like it. I am a teacher. I have two dogs.",
		Rubric:    teacher.Rubric{Grammar: 8, Vocabulary: 8, Coherence: 6, Task: 10},
		Comment:   "¡Muy bien por presentarte!",
		Fixes: []teacher.Fix{
			{Wrong: "Hi", Right: "Hi!", Why: "Un saludo puede llevar signo.", Kind: "punctuation"},
			{Wrong: "I has 30 years", Right: "I am 30 years old", Meaning: "tengo 30 años",
				Why: "La edad va con to be.", Major: true, Kind: "be"},
			{Wrong: "I am teacher", Right: "I am a teacher", Meaning: "soy profesor",
				Why: "Las profesiones llevan a/an.", Major: true, Kind: "article"},
			{Wrong: "two dog", Right: "two dogs", Why: "Más de uno lleva -s.", Major: true, Kind: "plural"},
		},
	}
}

func writeApp(t *testing.T, counts map[string]int) aiApp {
	t.Helper()
	x := newAIAppWithCounts(t, Write, counts)
	x.ft.task = teacher.WritingTask{
		Task: "Introduce yourself in 3 sentences.", Hint: "Empezá con: My name is...",
		Phrases: []string{"I am ... years old", "I live in"},
	}
	x.ft.correction = sampleCorrection()
	return x
}

// composing opens Write and waits for the task.
func composing(t *testing.T) aiApp {
	return composingWith(t, nil)
}

func composingWith(t *testing.T, counts map[string]int) aiApp {
	t.Helper()
	x := writeApp(t, counts)
	a, cmd := press(t, x.App, "enter")
	x.App = run(t, a, cmd)
	return x
}

// selfCorrecting sends sampleText and waits for the correction.
func selfCorrecting(t *testing.T, x aiApp) App {
	t.Helper()
	a := typeText(t, x.App, sampleText)
	a, cmd := press(t, a, "ctrl+d")
	return run(t, a, cmd)
}

// attempt types one fix and submits it with enter.
func attempt(t *testing.T, a App, fix string) (App, func() App) {
	t.Helper()
	a = typeText(t, a, fix)
	a, cmd := press(t, a, "enter")
	return a, func() App { return run(t, a, cmd) }
}

// feedback fixes the first error exactly, sends "I'm a good teacher" to
// the judge and does not know the third; the judge accepts the second.
func feedback(t *testing.T, x aiApp) App {
	t.Helper()
	x.ft.verdicts = []bool{true}
	a := selfCorrecting(t, x)
	a, _ = attempt(t, a, "i am 30 years old.")
	a, _ = attempt(t, a, "I'm a good teacher")
	_, wait := attempt(t, a, "")
	return wait()
}

func TestWriteStartsWithAnInvitation(t *testing.T) {
	x := writeApp(t, nil)
	mustContain(t, pageText(x.App), "Write a few sentences")
	mustContain(t, statusBar(x.App), "enter get a task")
}

func TestWriteWithoutTeacherIsOff(t *testing.T) {
	a := newApp(t, deck(), &recorder{}, Write, 120, 30)
	mustContain(t, pageText(a), "The AI teacher is off")
}

func TestWritePlanShowsTheTaskAndUsefulPhrases(t *testing.T) {
	x := composing(t)
	mustContain(t, pageText(x.App), "Introduce yourself in 3 sentences.", "Empezá con: My name is...",
		"Useful phrases", "I am ... years old", "I live in")
	if x.ft.level != domain.A2 || x.ft.focus != nil {
		t.Errorf("level = %s, focus = %v; want A2 and no focus without recurring errors", x.ft.level, x.ft.focus)
	}
}

func TestWriteTakesText(t *testing.T) {
	x := composing(t)
	// Letters that are keys elsewhere (q, c, d, n, s) belong to the text.
	a := typeText(t, x.App, "I has 30 years qcdns")
	a, _ = press(t, a, "backspace", "backspace", "backspace", "backspace", "backspace", "enter")
	a = typeText(t, a, "I am from Argentina.")

	if a.Screen() != Write {
		t.Fatalf("screen = %v", a.Screen())
	}
	mustContain(t, pageText(a), "I has 30 years", "I am from Argentina.")
	mustNotContain(t, pageText(a), "qcdns")
	mustContain(t, statusBar(a), "ctrl+d send")
}

func TestWriteEmptyTextIsNotSent(t *testing.T) {
	x := composing(t)
	a := typeText(t, x.App, "   ")
	_, cmd := press(t, a, "ctrl+d")
	if cmd != nil {
		t.Error("blank text must not be sent")
	}
}

func TestWriteTaskTargetsTheTwoMostRepeatedKinds(t *testing.T) {
	x := composingWith(t, map[string]int{"plural": 1, "spelling": 2, "be": 3, "article": 2, "other": 9})
	// "other" says nothing about what to practice; article comes before
	// spelling in teacher.Kinds, so it wins the tie.
	if want := []string{"be", "article"}; !reflect.DeepEqual(x.ft.focus, want) {
		t.Errorf("focus = %v, want %v", x.ft.focus, want)
	}
}

func TestWriteSelfCorrectMarksTheMajorErrorsWithoutTheAnswer(t *testing.T) {
	x := composing(t)
	x.clock.t = x.clock.t.Add(6 * time.Minute)
	a := selfCorrecting(t, x)

	if x.ft.text != sampleText {
		t.Errorf("sent %q", x.ft.text)
	}
	page := pageText(a)
	mustContain(t, page, "Fix the marked errors", "1. «I has 30 years»", "2. «I am teacher»", "3. «two dog»",
		"to be (am/is/are)", "articles (a/an/the)")
	if n := strings.Count(page, "«I has 30 years»"); n < 2 {
		t.Errorf("the error is marked %d times; want it in the text and in the list", n)
	}
	// No answer, no minor detail and no score before the learner tries.
	mustNotContain(t, page, "I am 30 years old", "I am a teacher", "two dogs", "La edad va con to be", "32/40")
	mustContain(t, statusBar(a), "enter next")
	if len(x.ad.added) != 0 || len(x.mk.logged) != 0 {
		t.Errorf("added %v, logged %v before the feedback", x.ad.added, x.mk.logged)
	}
	// The writing is recorded once, when the correction arrives.
	if len(x.log.deltas) != 1 {
		t.Fatalf("deltas = %+v", x.log.deltas)
	}
	if d := x.log.deltas[0]; d.Minutes != 6 || !reflect.DeepEqual(d.Activities, []string{"Writing: 32/40"}) {
		t.Errorf("delta = %+v", d)
	}
}

func TestWriteRecordsAtMost30Minutes(t *testing.T) {
	x := composing(t)
	x.clock.t = x.clock.t.Add(45 * time.Minute)
	selfCorrecting(t, x)
	if len(x.log.deltas) != 1 || x.log.deltas[0].Minutes != 30 {
		t.Errorf("deltas = %+v", x.log.deltas)
	}
}

func TestWriteExactFixesAreLocalAndTheRestGoToOneJudgeCall(t *testing.T) {
	x := composing(t)
	x.ft.verdicts = []bool{true}
	a := selfCorrecting(t, x)

	a, wait := attempt(t, a, "i am 30 years old.")
	mustContain(t, pageText(a), "2. «I am teacher»")
	a, _ = attempt(t, a, "I'm a good teacher")
	a, wait = attempt(t, a, "")
	mustContain(t, pageText(a), "Checking your fixes")
	a = wait()

	want := []teacher.Attempt{{Sentence: "I am teacher.", Wrong: "I am teacher", Right: "I am a teacher",
		Attempt: "I'm a good teacher"}}
	if !reflect.DeepEqual(x.ft.judgeCalls, [][]teacher.Attempt{want}) {
		t.Errorf("judge calls = %+v, want one with %+v", x.ft.judgeCalls, want)
	}
	page := pageText(a)
	if n := strings.Count(page, "✓ fixed by you"); n != 2 {
		t.Errorf("%d fixed by you, want 2:\n%s", n, page)
	}
	mustContain(t, page, "✗ two dog → two dogs", "Más de uno lleva -s.", "La edad va con to be.",
		"+1 minor details", "Added 1 card")

	// Only the error the learner could not fix becomes a card.
	if len(x.ad.added) != 1 || len(x.ad.added[0]) != 1 {
		t.Fatalf("added = %+v", x.ad.added)
	}
	if c := x.ad.added[0][0]; c.Word != "two dogs" || c.LastWrong != "two dog" || c.Example != "I have two dog." {
		t.Errorf("card = %+v", c)
	}
	// Every major error is logged, fixed or not.
	nov9 := domain.DateOf(fixedNow)
	logged := []domain.Mistake{
		{Date: nov9, Kind: "be", Wrong: "I has 30 years", Right: "I am 30 years old"},
		{Date: nov9, Kind: "article", Wrong: "I am teacher", Right: "I am a teacher"},
		{Date: nov9, Kind: "plural", Wrong: "two dog", Right: "two dogs"},
	}
	if !reflect.DeepEqual(x.mk.logged, [][]domain.Mistake{logged}) {
		t.Errorf("logged = %+v", x.mk.logged)
	}
	if len(x.log.deltas) != 1 {
		t.Errorf("deltas = %+v; the writing is recorded once", x.log.deltas)
	}
	mustContain(t, statusBar(a), "enter rewrite", "d minor details")
}

func TestWriteNoJudgeCallWhenEveryFixIsExactOrEmpty(t *testing.T) {
	x := composing(t)
	a := selfCorrecting(t, x)
	a, _ = attempt(t, a, "I am 30 years old")
	a, _ = attempt(t, a, "")
	a = typeText(t, a, "two dogs")
	a, cmd := press(t, a, "enter")
	if cmd != nil {
		t.Error("no attempt needs the judge, so no AI call")
	}
	if len(x.ft.judgeCalls) != 0 {
		t.Errorf("judge calls = %+v", x.ft.judgeCalls)
	}
	page := pageText(a)
	if n := strings.Count(page, "✓ fixed by you"); n != 2 {
		t.Errorf("%d fixed by you, want 2", n)
	}
	mustContain(t, page, "✗ I am teacher → I am a teacher")
	if len(x.ad.added) != 1 || x.ad.added[0][0].Word != "I am a teacher" {
		t.Errorf("added = %+v", x.ad.added)
	}
	if len(x.mk.logged) != 1 || len(x.mk.logged[0]) != 3 {
		t.Errorf("logged = %+v", x.mk.logged)
	}
}

func TestWriteJudgeFailureCountsTheAttemptsAsNotFixed(t *testing.T) {
	x := composing(t)
	x.ft.judgeErr = errors.New("pi timed out")
	a := selfCorrecting(t, x)
	a, _ = attempt(t, a, "I am 30 years old")
	a, _ = attempt(t, a, "I'm a good teacher")
	_, wait := attempt(t, a, "two dogs")
	a = wait()

	page := pageText(a)
	mustContain(t, page, "Could not check your fixes: pi timed out", "count as not fixed",
		"✗ I am teacher → I am a teacher")
	if n := strings.Count(page, "✓ fixed by you"); n != 2 {
		t.Errorf("%d fixed by you, want 2", n)
	}
	if len(x.ad.added) != 1 || len(x.ad.added[0]) != 1 || x.ad.added[0][0].Word != "I am a teacher" {
		t.Errorf("added = %+v", x.ad.added)
	}
	if len(x.mk.logged) != 1 || len(x.mk.logged[0]) != 3 {
		t.Errorf("logged = %+v", x.mk.logged)
	}
}

func TestWriteIgnoresAStaleJudgeAnswer(t *testing.T) {
	x := composing(t)
	x.ft.verdicts = []bool{true}
	a := selfCorrecting(t, x)
	a, _ = attempt(t, a, "")
	a, _ = attempt(t, a, "I'm a good teacher")
	a, wait := attempt(t, a, "")

	a = send(t, a, judgeMsg{req: a.write.req - 1, verdicts: []bool{false}})
	mustContain(t, pageText(a), "Checking your fixes")
	if len(x.mk.logged) != 0 {
		t.Errorf("a stale answer logged %+v", x.mk.logged)
	}
	a = wait()
	if n := strings.Count(pageText(a), "✓ fixed by you"); n != 1 {
		t.Errorf("%d fixed by you, want 1", n)
	}
}

func TestWriteMinorDetailsAreCollapsedAndDTogglesThem(t *testing.T) {
	x := composing(t)
	a := feedback(t, x)
	mustContain(t, pageText(a), "+1 minor details")
	mustNotContain(t, pageText(a), "Un saludo puede llevar signo.")

	a, _ = press(t, a, "d")
	mustContain(t, pageText(a), "Hi → Hi!", "Un saludo puede llevar signo.")
	a, _ = press(t, a, "d")
	mustNotContain(t, pageText(a), "Un saludo puede llevar signo.")
}

func TestWriteRewriteChecksEachUnfixedErrorLocally(t *testing.T) {
	x := composing(t)
	a := selfCorrecting(t, x)
	a, _ = attempt(t, a, "")
	a, _ = attempt(t, a, "")
	a, _ = attempt(t, a, "")
	judged := len(x.ft.judgeCalls)

	a, _ = press(t, a, "enter")
	page := pageText(a)
	mustContain(t, page, "Rewrite from memory", "1/3", "tengo 30 años")
	mustNotContain(t, page, "I am 30 years old")
	mustContain(t, statusBar(a), "enter check")

	a, _ = attempt(t, a, "I am 30 years old")
	mustContain(t, pageText(a), "✓ I am 30 years old")
	a, _ = press(t, a, "enter")

	mustContain(t, pageText(a), "2/3", "soy profesor")
	a, _ = attempt(t, a, "I am a teachr")
	mustContain(t, pageText(a), "✗ almost", "[→e]")
	a, _ = press(t, a, "enter")

	// Without a Spanish meaning, the page asks for the wrong chunk's correction.
	mustContain(t, pageText(a), "3/3", "the correct version of «two dog»")
	a, _ = attempt(t, a, "a dog")
	mustContain(t, pageText(a), "✗ right answer: two dogs")
	a, cmd := press(t, a, "enter")

	if cmd != nil || len(x.ft.judgeCalls) != judged {
		t.Error("the rewrite never calls the AI")
	}
	mustContain(t, pageText(a), "Rewrite: 1/3 right")
}

func TestWriteResultShowsRubricGradeCardsAndPattern(t *testing.T) {
	x := composingWith(t, map[string]int{"be": 2, "article": 1})
	a := feedback(t, x)
	a, _ = press(t, a, "enter") // the rewrite of "two dog"
	a, _ = attempt(t, a, "two dogs")
	a, _ = press(t, a, "enter")

	page := pageText(a)
	mustContain(t, page, "Grammar 8/10 · Vocabulary 8/10 · Coherence 6/10 · Task 10/10", "32/40 B",
		"¡Muy bien por presentarte!", "Added 1 card", "Pattern: 3rd time with to be (am/is/are)")
	mustContain(t, statusBar(a), "n new task")

	// The next task practices the kinds the learner keeps getting wrong.
	a, cmd := press(t, a, "n")
	run(t, a, cmd)
	if want := []string{"be", "article"}; !reflect.DeepEqual(x.ft.focus, want) {
		t.Errorf("focus = %v, want %v", x.ft.focus, want)
	}
}

func TestWriteNoPatternTheFirstTime(t *testing.T) {
	x := composing(t)
	a := feedback(t, x)
	a, _ = press(t, a, "enter")
	a, _ = attempt(t, a, "two dogs")
	a, _ = press(t, a, "enter")
	mustContain(t, pageText(a), "32/40 B")
	mustNotContain(t, pageText(a), "Pattern:")
}

func TestWriteNewTaskFromFeedback(t *testing.T) {
	x := composing(t)
	a := feedback(t, x)
	a, cmd := press(t, a, "n")
	a = run(t, a, cmd)
	mustContain(t, pageText(a), "Introduce yourself in 3 sentences.")
	mustNotContain(t, pageText(a), "fixed by you")
}

func TestWriteRecordMistakesFailureIsShown(t *testing.T) {
	x := composingWith(t, map[string]int{"be": 2})
	x.mk.err = errors.New("disk full")
	a := feedback(t, x)
	mustContain(t, pageText(a), "Could not save the error log: disk full")
	a, _ = press(t, a, "enter")
	a, _ = attempt(t, a, "two dogs")
	a, _ = press(t, a, "enter")
	// The pattern is still true for the learner, but the stored count did
	// not change, so the next task does not target it yet.
	mustContain(t, pageText(a), "Pattern: 3rd time with to be")
	a, cmd := press(t, a, "n")
	run(t, a, cmd)
	if want := []string{"be"}; !reflect.DeepEqual(x.ft.focus, want) {
		t.Errorf("focus = %v, want %v", x.ft.focus, want)
	}
}

func TestWriteNoErrors(t *testing.T) {
	x := composing(t)
	x.ft.correction = teacher.Correction{Corrected: "I am from Argentina.",
		Rubric: teacher.Rubric{Grammar: 10, Vocabulary: 10, Coherence: 10, Task: 10}, Comment: "¡Perfecto!"}
	a := typeText(t, x.App, "I am from Argentina.")
	a, cmd := press(t, a, "ctrl+d")
	a = run(t, a, cmd)

	mustContain(t, pageText(a), "No errors")
	if len(x.ad.added) != 0 || len(x.mk.logged) != 0 || len(x.ft.judgeCalls) != 0 {
		t.Errorf("added %v, logged %v, judged %v", x.ad.added, x.mk.logged, x.ft.judgeCalls)
	}
	if len(x.log.deltas) != 1 {
		t.Error("the writing still counts as study")
	}
	a, _ = press(t, a, "enter")
	mustContain(t, pageText(a), "40/40 A", "¡Perfecto!")
}

func TestWriteCorrectionErrorKeepsTheDraft(t *testing.T) {
	x := composing(t)
	x.ft.err = errors.New("pi timed out")
	a := typeText(t, x.App, "I has a dog.")
	a, cmd := press(t, a, "ctrl+d")
	a = run(t, a, cmd)

	mustContain(t, pageText(a), "pi timed out", "I has a dog.")
	if len(x.log.deltas) != 0 || len(x.mk.logged) != 0 || len(x.ad.added) != 0 {
		t.Errorf("a failed correction recorded %v, logged %v, added %v", x.log.deltas, x.mk.logged, x.ad.added)
	}
	x.ft.err = nil
	a, cmd = press(t, a, "ctrl+d")
	a = run(t, a, cmd)
	mustContain(t, pageText(a), "1. «I has 30 years»")
	if len(x.log.deltas) != 1 {
		t.Errorf("deltas = %+v", x.log.deltas)
	}
}

func TestWriteEscKeepsTheDraft(t *testing.T) {
	x := composing(t)
	a := typeText(t, x.App, "My draft")
	a, _ = press(t, a, "esc")
	if a.Screen() != Home {
		t.Fatalf("screen = %v", a.Screen())
	}
	a, _ = press(t, a, "tab", "tab", "tab")
	mustContain(t, pageText(a), "My draft")
}

func TestWriteEscDuringSelfCorrectKeepsTheAttempts(t *testing.T) {
	x := composing(t)
	a := selfCorrecting(t, x)
	a, _ = attempt(t, a, "I am 30 years old")
	a = typeText(t, a, "I am a")
	a, _ = press(t, a, "esc")
	if a.Screen() != Home {
		t.Fatalf("screen = %v", a.Screen())
	}
	a, _ = press(t, a, "tab", "tab", "tab")
	mustContain(t, pageText(a), "2. «I am teacher»", "I am a")
	a = typeText(t, a, " teacher")
	a, _ = press(t, a, "enter")
	a, _ = attempt(t, a, "")
	if n := strings.Count(pageText(a), "✓ fixed by you"); n != 2 {
		t.Errorf("%d fixed by you, want 2:\n%s", n, pageText(a))
	}
}

func TestWriteFailedCardsCanBeSavedAgain(t *testing.T) {
	x := composing(t)
	x.ad.err = errors.New("disk full")
	a := feedback(t, x)
	mustContain(t, pageText(a), "disk full")
	mustContain(t, statusBar(a), "s save cards")

	x.ad.err = nil
	a, _ = press(t, a, "s")

	mustContain(t, pageText(a), "Added 1 card")
	if len(x.ad.added) != 2 || len(x.log.deltas) != 1 || len(x.mk.logged) != 1 {
		t.Errorf("add calls %d, recorded %d, logged %d; the writing is recorded and logged once",
			len(x.ad.added), len(x.log.deltas), len(x.mk.logged))
	}
	if a, _ = press(t, a, "s"); len(x.ad.added) != 2 {
		t.Error("s after a successful save must not add again")
	}
}

func TestSentenceWith(t *testing.T) {
	text := "Hi. I has 30 years and I like it! I am from Argentina."
	if got := sentenceWith(text, "i HAS 30 years"); got != "I has 30 years and I like it!" {
		t.Errorf("got %q", got)
	}
	if got := sentenceWith(text, "nowhere"); got != "" {
		t.Errorf("got %q", got)
	}
}

func TestOrdinal(t *testing.T) {
	for n, want := range map[int]string{1: "1st", 2: "2nd", 3: "3rd", 4: "4th", 11: "11th", 12: "12th",
		13: "13th", 21: "21st", 22: "22nd", 103: "103rd", 111: "111th"} {
		if got := ordinal(n); got != want {
			t.Errorf("ordinal(%d) = %q, want %q", n, got, want)
		}
	}
}
