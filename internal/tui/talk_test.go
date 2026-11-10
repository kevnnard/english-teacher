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
	"github.com/kevnnard/english-teacher/internal/teacher"
)

// coffeeLine has the word-order error of sampleFeedback; "?", "q", "n",
// "s" and "c" are keys elsewhere, so they test that every key is a letter.
const coffeeLine = "Can I have a coffee big, please? quick nsc"

func sampleFeedback() teacher.Feedback {
	return teacher.Feedback{
		Points: []string{"Pediste con claridad y usaste please.", "Ojo: el adjetivo va antes del sustantivo."},
		Fixes: []teacher.Fix{
			{Wrong: "a coffee big", Right: "a big coffee", Meaning: "un café grande",
				Why: "En inglés el adjetivo va antes.", Major: true, Kind: "word-order"},
			{Wrong: "quick nsc", Right: "quickly", Why: "Detalle menor.", Kind: "spelling"},
		},
	}
}

func talkApp(t *testing.T) aiApp {
	t.Helper()
	x := newAIApp(t, Talk)
	x.ft.turns = []teacher.Turn{{Reply: "A big coffee, sure! Milk?", Recast: "Can I have a big coffee, please?"}}
	x.ft.feedback = sampleFeedback()
	return x
}

// chatting picks "Order a coffee", the second scenario.
func chatting(t *testing.T, x aiApp) App {
	t.Helper()
	a, _ := press(t, x.App, "down", "enter")
	return a
}

// say types line and sends it with enter, returning the app while it waits.
func say(t *testing.T, a App, line string) (App, func() App) {
	t.Helper()
	a = typeText(t, a, line)
	a, cmd := press(t, a, "enter")
	return a, func() App {
		t.Helper()
		a, cmd := sendCmd(t, a, cmd())
		if cmd != nil { // the role-play ended, so the summary was asked for
			a = run(t, a, cmd)
		}
		return a
	}
}

func TestTalkWithoutTeacherIsOff(t *testing.T) {
	a := newApp(t, deck(), &recorder{}, Talk, 120, 30)
	mustContain(t, pageText(a), "The AI teacher is off")
	mustContain(t, statusBar(a), "esc home")
}

func TestTalkPickerListsTheFourScenariosInOrder(t *testing.T) {
	x := talkApp(t)
	page := pageText(x.App)
	titles := []string{"Introduce yourself", "Order a coffee", "Ask for directions", "Small talk at work"}
	last := -1
	for _, title := range titles {
		i := strings.Index(page, title)
		if i <= last {
			t.Errorf("%q is missing or out of order:\n%s", title, page)
		}
		last = i
	}
	mustContain(t, page, "▸ Introduce yourself", "Say your name, where you are from and what you do.")
	mustContain(t, statusBar(x.App), "↑↓ choose", "enter start")

	a, _ := press(t, x.App, "down")
	mustContain(t, pageText(a), "▸ Order a coffee", "Order a drink, choose the size and pay.")
}

func TestTalkStartsWithTheOpeningGoalAndFadedFrames(t *testing.T) {
	x := talkApp(t) // the test learner is A2
	a, cmd := press(t, x.App, "down", "enter")
	if cmd != nil || len(x.ft.talkCalls) != 0 {
		t.Error("the opening line comes from the scenario, not from an AI call")
	}
	page := pageText(a)
	mustContain(t, page, "Order a coffee", "Goal: Order a drink, choose the size and pay.",
		"Barista: Hi there! What can I get you?", "Can I…", "How much…", "Turn 1/8")
	mustNotContain(t, page, "Can I have a ___, please?")
	mustContain(t, statusBar(a), "type your line", "enter send", "esc home")
	mustNotContain(t, statusBar(a), "ctrl+d")

	for level, want := range map[domain.Level][]string{
		domain.A1: {"Can I have a ___, please?", "How much is it?"},
		domain.B1: {"Goal: Order a drink"},
	} {
		y := talkApp(t)
		y.App.profile.Level = level
		page := pageText(chatting(t, y))
		mustContain(t, page, want...)
		if level == domain.B1 {
			mustNotContain(t, page, "Can I", "Frames")
		}
	}
}

func TestTalkTurnShowsTheReplyAndASubtleRecast(t *testing.T) {
	x := talkApp(t)
	a, wait := say(t, chatting(t, x), coffeeLine)
	if a.Screen() != Talk {
		t.Fatalf("screen = %v; every key must be a letter while typing", a.Screen())
	}
	mustContain(t, pageText(a), "You: "+coffeeLine, "Barista is answering")
	a = wait()

	want := []teacher.Line{{Tutor: true, Text: "Hi there! What can I get you?"}, {Text: coffeeLine}}
	if !reflect.DeepEqual(x.ft.talkCalls, [][]teacher.Line{want}) {
		t.Errorf("calls = %+v, want one with %+v", x.ft.talkCalls, want)
	}
	if x.ft.level != domain.A2 || x.ft.scenario.ID != "coffee" {
		t.Errorf("level %s, scenario %q", x.ft.level, x.ft.scenario.ID)
	}
	page := pageText(a)
	mustContain(t, page, "You: "+coffeeLine, "↳ Can I have a big coffee, please?",
		"Barista: A big coffee, sure! Milk?", "Turn 2/8")
	mustNotContain(t, page, "mistake", "Barista is answering")
	mustContain(t, statusBar(a), "enter send", "ctrl+d finish")
	if len(x.log.deltas) != 0 || len(x.ad.added) != 0 || len(x.mk.logged) != 0 {
		t.Errorf("recorded %+v, added %+v, logged %+v before the end", x.log.deltas, x.ad.added, x.mk.logged)
	}

	// The next call sends the whole transcript, the recast kept out of it.
	_, wait = say(t, a, "Yes please")
	wait()
	if got := x.ft.talkCalls[1]; len(got) != 4 || got[2].Text != "A big coffee, sure! Milk?" || got[3].Text != "Yes please" {
		t.Errorf("second call = %+v", got)
	}
}

func TestTalkEmptyLineIsNotSent(t *testing.T) {
	x := talkApp(t)
	a := typeText(t, chatting(t, x), "   ")
	if _, cmd := press(t, a, "enter"); cmd != nil {
		t.Error("a blank line must not be sent")
	}
}

func TestTalkFailedTurnKeepsTheLineAndEnterRetries(t *testing.T) {
	x := talkApp(t)
	x.ft.turnErr = errors.New("pi timed out")
	a, wait := say(t, chatting(t, x), "I want coffee")
	a = wait()

	page := pageText(a)
	mustContain(t, page, "Could not get an answer: pi timed out", "› I want coffee", "press enter to try again")
	mustNotContain(t, page, "You: I want coffee")
	mustContain(t, statusBar(a), "enter send")

	x.ft.turnErr = nil
	a, cmd := press(t, a, "enter")
	a = run(t, a, cmd)
	if len(x.ft.talkCalls) != 2 || !reflect.DeepEqual(x.ft.talkCalls[0], x.ft.talkCalls[1]) {
		t.Errorf("calls = %+v, want the same turn twice", x.ft.talkCalls)
	}
	mustContain(t, pageText(a), "You: I want coffee", "Barista: A big coffee, sure! Milk?")
	mustNotContain(t, pageText(a), "Could not get an answer")
}

func TestTalkIgnoresAStaleReply(t *testing.T) {
	x := talkApp(t)
	a, wait := say(t, chatting(t, x), "Hello")
	a = send(t, a, talkMsg{req: a.talk.req - 1, turn: teacher.Turn{Reply: "Old answer"}})
	mustContain(t, pageText(a), "Barista is answering")
	mustNotContain(t, pageText(a), "Old answer")
	a = wait()
	mustContain(t, pageText(a), "Barista: A big coffee, sure! Milk?")
}

// mustBeTheCoffeeSummary checks every effect of a finished coffee role-play.
func mustBeTheCoffeeSummary(t *testing.T, x aiApp, a App, minutes int) {
	t.Helper()
	page := pageText(a)
	mustContain(t, page, "Feedback", "Pediste con claridad y usaste please.",
		"Ojo: el adjetivo va antes del sustantivo.", "✗ a coffee big → a big coffee",
		"En inglés el adjetivo va antes.", "quick nsc → quickly", "Added 1 card")
	mustContain(t, statusBar(a), "n new role-play")
	mustNotContain(t, statusBar(a), "s save cards")

	// Only the major error becomes a card, with the learner's line as example.
	if len(x.ad.added) != 1 || len(x.ad.added[0]) != 1 {
		t.Fatalf("added = %+v", x.ad.added)
	}
	if c := x.ad.added[0][0]; c.Word != "a big coffee" || c.LastWrong != "a coffee big" ||
		c.Example != coffeeLine || c.Meaning != "un café grande" {
		t.Errorf("card = %+v", c)
	}
	logged := []domain.Mistake{{Date: domain.DateOf(fixedNow), Kind: "word-order", Wrong: "a coffee big", Right: "a big coffee"}}
	if !reflect.DeepEqual(x.mk.logged, [][]domain.Mistake{logged}) {
		t.Errorf("logged = %+v", x.mk.logged)
	}
	if a.mistakeCounts["word-order"] != 1 {
		t.Errorf("counts = %+v", a.mistakeCounts)
	}
	if len(x.log.deltas) != 1 {
		t.Fatalf("deltas = %+v; the role-play is recorded once", x.log.deltas)
	}
	if d := x.log.deltas[0]; d.Minutes != minutes || !reflect.DeepEqual(d.Activities, []string{"Talk: Order a coffee"}) {
		t.Errorf("delta = %+v", d)
	}
	if len(x.ft.feedbackCalls) != 1 {
		t.Errorf("feedback calls = %d, want 1", len(x.ft.feedbackCalls))
	}
}

func TestTalkEndsWhenTheGoalIsMet(t *testing.T) {
	x := talkApp(t)
	x.ft.turns[0].Done = true
	a := chatting(t, x)
	x.clock.t = x.clock.t.Add(7 * time.Minute)
	a, wait := say(t, a, coffeeLine)
	a = wait()

	mustBeTheCoffeeSummary(t, x, a, 7)
	if got := x.ft.feedbackCalls[0]; len(got) != 3 || got[2].Text != "A big coffee, sure! Milk?" {
		t.Errorf("the summary got %+v; want the whole transcript", got)
	}
}

func TestTalkCtrlDFinishesEarlyAndRecordsAtMost20Minutes(t *testing.T) {
	x := talkApp(t)
	a := chatting(t, x)
	if _, cmd := press(t, a, "ctrl+d"); cmd != nil {
		t.Error("ctrl+d before the first line must not finish")
	}
	a, wait := say(t, a, coffeeLine)
	a = wait()
	x.clock.t = x.clock.t.Add(45 * time.Minute)
	a, cmd := press(t, a, "ctrl+d")
	mustContain(t, pageText(a), "Writing your feedback")
	a = run(t, a, cmd)

	mustBeTheCoffeeSummary(t, x, a, 20)
}

func TestTalkEndsAfterEightTurns(t *testing.T) {
	x := talkApp(t)
	x.ft.turns = []teacher.Turn{{Reply: "Anything else?"}}
	a := chatting(t, x)
	for i := 1; i <= 7; i++ {
		var wait func() App
		a, wait = say(t, a, "One more thing")
		a = wait()
	}
	mustContain(t, pageText(a), "Turn 8/8")
	if len(x.ft.feedbackCalls) != 0 {
		t.Fatal("the summary came before the 8th turn")
	}
	_, wait := say(t, a, "Bye")
	a = wait()
	if len(x.ft.talkCalls) != 8 || len(x.ft.feedbackCalls) != 1 {
		t.Errorf("%d turns, %d summaries; want 8 and 1", len(x.ft.talkCalls), len(x.ft.feedbackCalls))
	}
	mustContain(t, pageText(a), "Feedback", "Pediste con claridad")
}

func TestTalkFailedSummaryRecordsNothingAndEnterRetries(t *testing.T) {
	x := talkApp(t)
	x.ft.feedbackErr = errors.New("pi timed out")
	a, wait := say(t, chatting(t, x), coffeeLine)
	a = wait()
	a, cmd := press(t, a, "ctrl+d")
	a = run(t, a, cmd)

	mustContain(t, pageText(a), "Could not get your feedback: pi timed out", "Press enter to try again.")
	mustContain(t, statusBar(a), "enter try again")
	if len(x.log.deltas) != 0 || len(x.ad.added) != 0 || len(x.mk.logged) != 0 {
		t.Errorf("recorded %+v, added %+v, logged %+v on failure", x.log.deltas, x.ad.added, x.mk.logged)
	}

	x.ft.feedbackErr, x.ft.feedbackCalls = nil, nil
	a, cmd = press(t, a, "enter")
	a = run(t, a, cmd)
	mustBeTheCoffeeSummary(t, x, a, 0)
}

func TestTalkIgnoresAStaleSummary(t *testing.T) {
	x := talkApp(t)
	a, wait := say(t, chatting(t, x), coffeeLine)
	a = wait()
	a, cmd := press(t, a, "ctrl+d")
	a = send(t, a, talkFeedbackMsg{req: a.talk.req - 1, feedback: sampleFeedback()})
	mustContain(t, pageText(a), "Writing your feedback")
	if len(x.log.deltas) != 0 || len(x.ad.added) != 0 {
		t.Errorf("a stale summary recorded %+v, added %+v", x.log.deltas, x.ad.added)
	}
	mustBeTheCoffeeSummary(t, x, run(t, a, cmd), 0)
}

func TestTalkCardsThatFailedToSaveCanBeSavedAgain(t *testing.T) {
	x := talkApp(t)
	x.ad.err = errors.New("disk full")
	a, wait := say(t, chatting(t, x), coffeeLine)
	a = wait()
	a, cmd := press(t, a, "ctrl+d")
	a = run(t, a, cmd)

	mustContain(t, pageText(a), "Could not add the cards: disk full", "Press s to try again.")
	mustContain(t, statusBar(a), "s save cards")
	words := len(a.cards)

	x.ad.err, x.ad.added = nil, nil
	a, _ = press(t, a, "s")
	mustBeTheCoffeeSummary(t, x, a, 0)
	if len(a.cards) != words+2 {
		t.Errorf("deck has %d cards, want %d (both directions)", len(a.cards), words+2)
	}
}

func TestTalkNStartsANewRolePlay(t *testing.T) {
	x := talkApp(t)
	x.ft.turns[0].Done = true
	a, wait := say(t, chatting(t, x), coffeeLine)
	a = wait()
	a, _ = press(t, a, "n")
	page := pageText(a)
	mustContain(t, page, "▸ Order a coffee", "Introduce yourself")
	mustNotContain(t, page, "Feedback")
	if len(x.log.deltas) != 1 {
		t.Errorf("deltas = %+v", x.log.deltas)
	}
}

func TestTalkEscGoesHomeAndKeepsTheConversation(t *testing.T) {
	x := talkApp(t)
	a, wait := say(t, chatting(t, x), coffeeLine)
	a = typeText(t, wait(), "half")
	a, _ = press(t, a, "esc")
	if a.Screen() != Home {
		t.Fatalf("screen = %v", a.Screen())
	}
	a, _ = press(t, a, ":")
	a = typeText(t, a, "talk")
	a, _ = press(t, a, "enter")
	mustContain(t, pageText(a), "Barista: A big coffee, sure! Milk?", "› half")
	if len(x.log.deltas) != 0 {
		t.Errorf("leaving recorded %+v", x.log.deltas)
	}
}

func TestTalkKeepsTheNewestLinesAndTheInputOn80x24(t *testing.T) {
	x := talkApp(t)
	x.App.profile.Level = domain.A1 // every frame shows, so the page is fullest
	a := send(t, chatting(t, x), tea.WindowSizeMsg{Width: 80, Height: 24})
	for i := 1; i <= 7; i++ {
		x.ft.turns = []teacher.Turn{{Reply: fmt.Sprintf("Reply number %d.", i)}}
		var wait func() App
		a, wait = say(t, a, fmt.Sprintf("Line number %d", i))
		a = wait()
	}
	a = typeText(t, a, "last")
	page := pageText(a)
	mustContain(t, page, "Goal: Order a drink", "Can I have a ___, please?", "Turn 8/8",
		"You: Line number 7", "Barista: Reply number 7.", "› last")
	mustNotContain(t, page, "Line number 1")
	if n := len(screenLines(a)); n != 24 {
		t.Errorf("%d rows, want 24", n)
	}
}
