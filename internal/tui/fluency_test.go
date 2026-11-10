package tui

import (
	"reflect"
	"testing"
	"time"

	"github.com/kevnnard/english-teacher/internal/domain"
)

func TestWordsPerMinute(t *testing.T) {
	tests := []struct {
		name           string
		text           string
		elapsed, limit time.Duration
		want           float64
	}{
		{"no words", "", 4 * time.Minute, 4 * time.Minute, 0},
		{"only spaces and new lines", "  \n \t ", 4 * time.Minute, 4 * time.Minute, 0},
		{"the whole round", "one two three four five six seven eight", 4 * time.Minute, 4 * time.Minute, 2},
		{"early finish", "a b c d e f g h i j k l m n o", 90 * time.Second, 3 * time.Minute, 10},
		{"late is capped at the limit", "a b c d e f g h i j", 3 * time.Minute, 2 * time.Minute, 5},
		{"new lines split words", "I wake up\nat seven", 30 * time.Second, 2 * time.Minute, 10},
		{"no time yet", "hello", 0, 2 * time.Minute, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := wordsPerMinute(tt.text, tt.elapsed, tt.limit); got != tt.want {
				t.Errorf("wordsPerMinute(%q, %v, %v) = %v, want %v", tt.text, tt.elapsed, tt.limit, got, tt.want)
			}
		})
	}
}

// fluencyApp opens the Fluency screen with the AI teacher's fakes, so the
// study log and the clock can be checked.
func fluencyApp(t *testing.T) aiApp {
	t.Helper()
	return newAIApp(t, Fluency)
}

// tick delivers the timer message of the round on screen, as tea.Tick
// would after a second; the test clock decides how much time has passed.
func tick(t *testing.T, a App) (App, bool) {
	t.Helper()
	a, cmd := sendCmd(t, a, fluencyTickMsg{run: a.fluency.run})
	return a, cmd != nil
}

// startRound presses enter on a ready round and checks a timer started.
func startRound(t *testing.T, a App) App {
	t.Helper()
	a, cmd := press(t, a, "enter")
	if cmd == nil {
		t.Fatal("starting a round must start its timer")
	}
	return a
}

func TestFluencyWorksWithoutTheTeacher(t *testing.T) {
	a := newApp(t, deck(), &recorder{}, Fluency, 120, 30)
	mustContain(t, pageText(a), "Fluency", "Your morning routine")
	mustNotContain(t, pageText(a), "The AI teacher is off")
}

func TestFluencyPickerListsTheTopicsAndYourOwn(t *testing.T) {
	x := fluencyApp(t)
	page := pageText(x.App)
	mustContain(t, page, "4, 3 and 2 minutes", "▸ Your morning routine", "Your best friend",
		"Your last weekend", "Your home", "Your own topic")
	mustContain(t, statusBar(x.App), "↑↓ choose", "enter pick")

	a, _ := press(t, x.App, "down", "j")
	mustContain(t, pageText(a), "▸ Your last weekend")
	a, _ = press(t, a, "up")
	mustContain(t, pageText(a), "▸ Your best friend")
}

func TestFluencyThreeRoundsShowWordsPerMinute(t *testing.T) {
	x := fluencyApp(t)
	a, _ := press(t, x.App, "down", "down", "down", "enter") // Your home
	page := pageText(a)
	mustContain(t, page, "Your home", "Round 1 of 3", "4:00")
	mustContain(t, statusBar(a), "enter start")

	// Round 1 runs to the end of its 4 minutes.
	a = startRound(t, a)
	mustContain(t, statusBar(a), "ctrl+d end round")
	a = typeText(t, a, "I live in a small house with my family") // 9 words
	mustContain(t, pageText(a), "4:00 left", "9 words", "I live in a small house")

	x.clock.t = x.clock.t.Add(75 * time.Second)
	a, more := tick(t, a)
	if !more {
		t.Error("the timer must keep ticking while the round runs")
	}
	mustContain(t, pageText(a), "2:45 left")

	x.clock.t = x.clock.t.Add(165 * time.Second)
	a, more = tick(t, a)
	if more {
		t.Error("the timer must stop when the round is over")
	}
	mustContain(t, pageText(a), "Round 1 · 4:00 · 9 words · 2.2 wpm", "Round 2 of 3", "3:00")
	if len(x.log.deltas) != 0 {
		t.Errorf("recorded %v before round 3 ended", x.log.deltas)
	}

	// The input is locked: the round is over, so a letter does nothing.
	before := a.fluency.rounds[0].text
	a, _ = press(t, a, "x")
	if a.fluency.rounds[0].text != before {
		t.Error("the text of a finished round changed")
	}

	// Round 2 ends early with ctrl+d after 1:30.
	a = startRound(t, a)
	a = typeText(t, a, "My home is small but it is warm and I love my room") // 13 words
	x.clock.t = x.clock.t.Add(90 * time.Second)
	a, cmd := press(t, a, "ctrl+d")
	if cmd != nil {
		t.Error("ending a round early starts nothing")
	}
	mustContain(t, pageText(a), "Round 2 · 3:00 · 13 words · 8.7 wpm", "Round 3 of 3", "2:00")

	// Round 3 times out after 2 minutes.
	a = startRound(t, a)
	a = typeText(t, a, "I live in a small warm house with my family and I love my room") // 15 words
	x.clock.t = x.clock.t.Add(2 * time.Minute)
	a, _ = tick(t, a)

	page = pageText(a)
	mustContain(t, page, "Fluency · Your home",
		"Round 1 · 4:00 · 9 words · 2.2 wpm",
		"Round 2 · 3:00 · 13 words · 8.7 wpm",
		"Round 3 · 2:00 · 15 words · 7.5 wpm",
		"Round 1 → 3: +5.2 wpm", "I live in a small warm house")
	mustContain(t, statusBar(a), "n new topic")

	// 4:00 + 1:30 + 2:00 of real time is 7 whole minutes.
	want := []domain.Day{{Date: domain.DateOf(fixedNow), Minutes: 7, Activities: []string{"Fluency: Your home"}}}
	if !reflect.DeepEqual(x.log.deltas, want) {
		t.Errorf("recorded %v, want %v", x.log.deltas, want)
	}

	// A late tick and more keys record nothing more.
	a, more = tick(t, a)
	a, _ = press(t, a, "x", "enter")
	if more || len(x.log.deltas) != 1 {
		t.Errorf("after the result: more ticks %v, deltas %v", more, x.log.deltas)
	}

	a, _ = press(t, a, "n")
	mustContain(t, pageText(a), "▸ Your home")
	if len(x.log.deltas) != 1 {
		t.Error("starting again recorded something")
	}
}

func TestFluencyRecordsAtMostNineMinutes(t *testing.T) {
	x := fluencyApp(t)
	a, _ := press(t, x.App, "enter")
	for range 3 {
		a = startRound(t, a)
		a = typeText(t, a, "I get up")
		x.clock.t = x.clock.t.Add(20 * time.Minute) // the learner walked away
		a, _ = tick(t, a)
	}
	want := []domain.Day{{Date: domain.DateOf(fixedNow), Minutes: 9, Activities: []string{"Fluency: Your morning routine"}}}
	if !reflect.DeepEqual(x.log.deltas, want) {
		t.Errorf("recorded %v, want %v", x.log.deltas, want)
	}
	// Each round counts at most its own limit.
	mustContain(t, pageText(a), "Round 1 · 4:00 · 3 words · 0.8 wpm", "Round 3 · 2:00 · 3 words · 1.5 wpm",
		"Round 1 → 3: +0.8 wpm")
}

func TestFluencyEveryKeyIsALetterWhileTheRoundRuns(t *testing.T) {
	x := fluencyApp(t)
	a, _ := press(t, x.App, "enter")
	a = startRound(t, a)
	a = typeText(t, a, "q?:nc")
	a, _ = press(t, a, "enter", "tab", "shift+tab")
	a = typeText(t, a, "ok")
	a, _ = press(t, a, "backspace")
	if a.Screen() != Fluency || a.help || a.palette.open {
		t.Fatalf("screen %v, help %v, palette %v: keys must be letters", a.Screen(), a.help, a.palette.open)
	}
	if got := a.fluency.rounds[0].text; got != "q?:nc\no" {
		t.Errorf("text = %q", got)
	}
}

func TestFluencyAKeyAfterTheTimeIsUpEndsTheRound(t *testing.T) {
	x := fluencyApp(t)
	a, _ := press(t, x.App, "enter")
	a = startRound(t, a)
	a = typeText(t, a, "I get up")
	x.clock.t = x.clock.t.Add(4*time.Minute + time.Second) // before the next tick arrives
	a = typeText(t, a, "late")
	if got := a.fluency.rounds[0].text; got != "I get up" {
		t.Errorf("text = %q; the input must lock when the time is up", got)
	}
	mustContain(t, pageText(a), "Round 1 · 4:00 · 3 words · 0.8 wpm", "Round 2 of 3")
}

func TestFluencyOwnTopic(t *testing.T) {
	x := fluencyApp(t)
	a, _ := press(t, x.App, "down", "down", "down", "down", "down", "enter")
	mustContain(t, pageText(a), "Your own topic", "› ")
	mustContain(t, statusBar(a), "enter ok", "esc back")

	a, _ = press(t, a, "enter") // empty: nothing happens
	if a.fluency.phase != fluencyOwnTopic {
		t.Fatal("an empty topic must not start")
	}
	a = typeText(t, a, "My cat qn")
	a, _ = press(t, a, "backspace", "backspace", "backspace", "enter")
	if a.Screen() != Fluency {
		t.Fatalf("screen = %v", a.Screen())
	}
	mustContain(t, pageText(a), "My cat", "Round 1 of 3")

	for range 3 {
		a = startRound(t, a)
		a, _ = press(t, a, "ctrl+d")
	}
	if len(x.log.deltas) != 1 || x.log.deltas[0].Activities[0] != "Fluency: My cat" {
		t.Errorf("recorded %v", x.log.deltas)
	}

	// esc while typing a topic goes back to the list.
	b, _ := press(t, x.App, "down", "down", "down", "down", "down", "enter")
	b = typeText(t, b, "abc")
	b, _ = press(t, b, "esc")
	if b.Screen() != Fluency || b.fluency.phase != fluencyPick {
		t.Errorf("esc: screen %v, phase %v", b.Screen(), b.fluency.phase)
	}
}

func TestFluencyEscKeepsTheRoundAndTheTimerIsHarmlessElsewhere(t *testing.T) {
	x := fluencyApp(t)
	a, _ := press(t, x.App, "enter")
	a = startRound(t, a)
	a = typeText(t, a, "I get up")
	a, _ = press(t, a, "esc")
	if a.Screen() != Home {
		t.Fatalf("esc: screen = %v", a.Screen())
	}

	// Ticks on Home keep the clock honest without touching the screen.
	x.clock.t = x.clock.t.Add(time.Minute)
	a, more := tick(t, a)
	if !more || a.Screen() != Home {
		t.Errorf("tick on Home: more %v, screen %v", more, a.Screen())
	}
	x.clock.t = x.clock.t.Add(3 * time.Minute)
	a, more = tick(t, a)
	if more || a.Screen() != Home || len(x.log.deltas) != 0 {
		t.Errorf("round end on Home: more %v, screen %v, deltas %v", more, a.Screen(), x.log.deltas)
	}
	mustContain(t, pageText(a), "Good evening") // Home is still on screen

	a, _ = press(t, a, "tab", "tab", "tab", "tab", "tab") // Home → Fluency
	if a.Screen() != Fluency {
		t.Fatalf("screen = %v", a.Screen())
	}
	mustContain(t, pageText(a), "Round 1 · 4:00 · 3 words · 0.8 wpm", "Round 2 of 3")

	// A tick from an earlier round is ignored.
	a = startRound(t, a)
	stale := a.fluency.run - 1
	a, cmd := sendCmd(t, a, fluencyTickMsg{run: stale})
	if cmd != nil || a.fluency.phase != fluencyRunning {
		t.Error("a stale tick must do nothing")
	}
}

func TestFluencyLeavingBeforeRoundThreeEndsRecordsNothing(t *testing.T) {
	x := fluencyApp(t)
	a, _ := press(t, x.App, "enter")
	for range 2 {
		a = startRound(t, a)
		a = typeText(t, a, "I get up")
		a, _ = press(t, a, "ctrl+d")
	}
	a = startRound(t, a)
	a = typeText(t, a, "I get")
	a, _ = press(t, a, "esc")
	a, _ = press(t, a, "q") // q on Home quits
	if len(x.log.deltas) != 0 {
		t.Errorf("recorded %v before round 3 ended", x.log.deltas)
	}

	// n between rounds goes back to the topics and records nothing.
	b, _ := press(t, x.App, "enter")
	b = startRound(t, b)
	b, _ = press(t, b, "ctrl+d")
	mustContain(t, statusBar(b), "enter start", "n new topic")
	b, _ = press(t, b, "n")
	if b.fluency.phase != fluencyPick || len(x.log.deltas) != 0 {
		t.Errorf("n: phase %v, deltas %v", b.fluency.phase, x.log.deltas)
	}
}
