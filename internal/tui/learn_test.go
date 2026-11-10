package tui

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/kevnnard/english-teacher/internal/domain"
	"github.com/kevnnard/english-teacher/internal/teacher"
)

// fakeTeacher answers like the AI teacher without any network.
type fakeTeacher struct {
	reading    teacher.Reading
	task       teacher.WritingTask
	correction teacher.Correction
	err        error
	known      []string // the known words of the last reading request
	level      domain.Level
	text       string // the last text sent to Correct
}

func (f *fakeTeacher) Reading(_ context.Context, l domain.Level, known []string) (teacher.Reading, error) {
	f.level, f.known = l, known
	return f.reading, f.err
}

func (f *fakeTeacher) WritingTask(_ context.Context, l domain.Level, _ []string) (teacher.WritingTask, error) {
	f.level = l
	return f.task, f.err
}

func (f *fakeTeacher) Correct(_ context.Context, l domain.Level, _, text string) (teacher.Correction, error) {
	f.level, f.text = l, text
	return f.correction, f.err
}

// adder is a fake AddFunc that remembers the cards it got.
type adder struct {
	added [][]domain.Card
	err   error
}

func (ad *adder) add(cards []domain.Card) (int, error) {
	ad.added = append(ad.added, cards)
	return len(cards), ad.err
}

func sampleReading() teacher.Reading {
	return teacher.Reading{
		Title: "A new friend",
		Text:  "Hello! My name is Ana. I am a teacher. How about you?",
		Glossary: []teacher.Gloss{
			{Word: "teacher", Meaning: "profesor/a", Example: "I am a teacher.", New: true},
			{Word: "how about", Meaning: "¿y vos?", Example: "How about you?", New: true},
			{Word: "hello", Meaning: "hola", Example: "Hello!"},
		},
	}
}

type aiApp struct {
	App
	ft    *fakeTeacher
	ad    *adder
	log   *studyLog
	clock *testClock
}

func newAIApp(t *testing.T, start Screen) aiApp {
	t.Helper()
	x := aiApp{ft: &fakeTeacher{reading: sampleReading()}, ad: &adder{}, log: &studyLog{}, clock: &testClock{t: fixedNow}}
	a := NewApp(Config{
		Cards: deck(), Save: (&recorder{}).save, Now: x.clock.now, Start: start, MaxNew: DefaultMaxNew,
		Profile: domain.Profile{Level: domain.A2}, Record: x.log.record,
		Teacher: x.ft, Add: x.ad.add,
	})
	x.App = send(t, a, tea.WindowSizeMsg{Width: 120, Height: 40})
	return x
}

// run executes cmd, as Bubble Tea would, and feeds its message back.
func run(t *testing.T, a App, cmd tea.Cmd) App {
	t.Helper()
	if cmd == nil {
		t.Fatal("want a command")
	}
	return send(t, a, cmd())
}

func TestLearnStartsWithAnInvitation(t *testing.T) {
	x := newAIApp(t, Learn)
	mustContain(t, pageText(x.App), "Read a short text written for you")
	mustContain(t, statusBar(x.App), "enter read")
}

func TestLearnWithoutTeacherSaysHowToTurnItOn(t *testing.T) {
	a := NewApp(Config{Cards: deck(), Save: (&recorder{}).save, Now: (&testClock{t: fixedNow}).now, Start: Learn})
	a = send(t, a, tea.WindowSizeMsg{Width: 120, Height: 40})
	mustContain(t, pageText(a), "The AI teacher is off")
	if _, cmd := press(t, a, "enter"); cmd != nil {
		t.Error("enter must do nothing without a teacher")
	}
}

func TestLearnLoadsAReadingAtTheLearnersLevel(t *testing.T) {
	x := newAIApp(t, Learn)

	a, cmd := press(t, x.App, "enter")
	mustContain(t, pageText(a), "Writing a text for you")
	a = run(t, a, cmd)

	if x.ft.level != domain.A2 || len(x.ft.known) == 0 {
		t.Errorf("asked level %s with known %v", x.ft.level, x.ft.known)
	}
	page := pageText(a)
	for _, want := range []string{"A new friend", "My name is Ana", "Glossary", "teacher", "profesor/a", "new"} {
		mustContain(t, page, want)
	}
}

func TestLearnKnownWordsAreDistinct(t *testing.T) {
	x := newAIApp(t, Learn)
	a, cmd := press(t, x.App, "enter")
	run(t, a, cmd)
	seen := map[string]bool{}
	for _, w := range x.ft.known {
		if seen[strings.ToLower(w)] {
			t.Errorf("%q sent twice", w)
		}
		seen[strings.ToLower(w)] = true
	}
}

func loadedLearn(t *testing.T) aiApp {
	t.Helper()
	x := newAIApp(t, Learn)
	a, cmd := press(t, x.App, "enter")
	x.App = run(t, a, cmd)
	return x
}

func TestLearnNewWordsStartMarkedAndSpaceToggles(t *testing.T) {
	x := loadedLearn(t)
	mustContain(t, pageText(x.App), "[x] teacher")
	mustContain(t, pageText(x.App), "[ ] hello")

	a, _ := press(t, x.App, "down", "down", "space", "up", "space")

	mustContain(t, pageText(a), "[x] hello")
	mustContain(t, pageText(a), "[ ] how about")
}

func TestLearnDoneAddsMarkedWordsAndRecordsTheReading(t *testing.T) {
	x := loadedLearn(t)
	x.clock.t = x.clock.t.Add(4 * time.Minute)

	a, _ := press(t, x.App, "a")

	if len(x.ad.added) != 1 || len(x.ad.added[0]) != 2 {
		t.Fatalf("added = %+v", x.ad.added)
	}
	if c := x.ad.added[0][0]; c.Word != "teacher" || c.Meaning != "profesor/a" || c.Example != "I am a teacher." {
		t.Errorf("card = %+v", c)
	}
	mustContain(t, pageText(a), "Added 2 new words to review")
	if len(x.log.deltas) != 1 {
		t.Fatalf("deltas = %+v", x.log.deltas)
	}
	d := x.log.deltas[0]
	if d.Minutes != 4 || len(d.Activities) != 1 || d.Activities[0] != "Reading: A new friend" || d.Reviews != 0 {
		t.Errorf("delta = %+v", d)
	}
	if !a.streak().StudiedToday {
		t.Error("a finished reading counts as studying today")
	}
	if words := countWord(a.cards, "teacher"); words != 2 {
		t.Errorf("teacher is in %d directions, want 2", words)
	}
}

func TestLearnDoneTwiceAddsOnce(t *testing.T) {
	x := loadedLearn(t)
	a, _ := press(t, x.App, "a", "a")
	if len(x.ad.added) != 1 || len(x.log.deltas) != 1 {
		t.Errorf("added %d times, recorded %d times", len(x.ad.added), len(x.log.deltas))
	}
	mustContain(t, statusBar(a), "n new text")
}

func TestLearnReadingMinutesAreCapped(t *testing.T) {
	x := loadedLearn(t)
	x.clock.t = x.clock.t.Add(3 * time.Hour)
	press(t, x.App, "a")
	if got := x.log.deltas[0].Minutes; got != maxReadingMinutes {
		t.Errorf("minutes = %d, want %d", got, maxReadingMinutes)
	}
}

func TestLearnAddErrorIsShownAndNothingIsRecorded(t *testing.T) {
	x := loadedLearn(t)
	x.ad.err = errors.New("disk full")

	a, _ := press(t, x.App, "a")

	mustContain(t, pageText(a), "disk full")
	if len(x.log.deltas) != 0 {
		t.Errorf("deltas = %+v", x.log.deltas)
	}
}

func TestLearnErrorOffersARetry(t *testing.T) {
	x := newAIApp(t, Learn)
	x.ft.err = errors.New("pi not found")

	a, cmd := press(t, x.App, "enter")
	a = run(t, a, cmd)

	mustContain(t, pageText(a), "pi not found")
	mustContain(t, statusBar(a), "enter try again")
	x.ft.err = nil
	a, cmd = press(t, a, "enter")
	a = run(t, a, cmd)
	mustContain(t, pageText(a), "A new friend")
}

func TestLearnStaleAnswerIsIgnored(t *testing.T) {
	x := newAIApp(t, Learn)
	_, cmd := press(t, x.App, "enter")
	msg := cmd()
	a := x.App // a model that never asked
	a = send(t, a, msg)
	mustContain(t, pageText(a), "Read a short text written for you")
}

func countWord(cards []domain.Card, w string) int {
	n := 0
	for _, c := range cards {
		if c.Word == w {
			n++
		}
	}
	return n
}

func TestLearnLongGlossaryScrollsWithTheCursor(t *testing.T) {
	x := newAIApp(t, Learn)
	r := sampleReading()
	r.Text = strings.Repeat("Hello! My name is Ana and I am a teacher in a small town. ", 5)
	for i := 0; i < 15; i++ {
		r.Glossary = append(r.Glossary, teacher.Gloss{Word: fmt.Sprintf("word%02d", i), Meaning: "m"})
	}
	x.ft.reading = r
	a := send(t, x.App, tea.WindowSizeMsg{Width: 100, Height: 24})
	a, cmd := press(t, a, "enter")
	a = run(t, a, cmd)

	keys := make([]string, len(r.Glossary)-1)
	for i := range keys {
		keys[i] = "down"
	}
	a, _ = press(t, a, keys...)

	lines := screenLines(a)
	if len(lines) != 24 {
		t.Errorf("view has %d lines, want 24", len(lines))
	}
	mustContain(t, pageText(a), "▸ [ ] word14")
	mustContain(t, statusBar(a), "space mark")
}
