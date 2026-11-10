package tui

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/kevnnard/english-teacher/internal/domain"
	"github.com/kevnnard/english-teacher/internal/teacher"
)

func sampleCorrection() teacher.Correction {
	return teacher.Correction{
		Corrected: "I am 30 years old. I am from Argentina.",
		Rubric:    teacher.Rubric{Grammar: 1, Vocabulary: 2, Coherence: 1, Task: 2},
		Comment:   "¡Muy bien por presentarte!",
		Fixes: []teacher.Fix{{
			Wrong: "I has 30 years", Right: "I am 30 years old", Meaning: "tengo 30 años",
			Why: "La edad va con to be.",
		}},
	}
}

func writeApp(t *testing.T) aiApp {
	t.Helper()
	x := newAIApp(t, Write)
	x.ft.task = teacher.WritingTask{Task: "Introduce yourself in 3 sentences.", Hint: "Empezá con: My name is..."}
	x.ft.correction = sampleCorrection()
	return x
}

// composing opens Write and waits for the task.
func composing(t *testing.T) aiApp {
	t.Helper()
	x := writeApp(t)
	a, cmd := press(t, x.App, "enter")
	x.App = run(t, a, cmd)
	return x
}

func TestWriteStartsWithAnInvitation(t *testing.T) {
	x := writeApp(t)
	mustContain(t, pageText(x.App), "Write a few sentences")
	mustContain(t, statusBar(x.App), "enter get a task")
}

func TestWriteWithoutTeacherIsOff(t *testing.T) {
	a := newApp(t, deck(), &recorder{}, Write, 120, 30)
	mustContain(t, pageText(a), "The AI teacher is off")
}

func TestWriteShowsTheTaskAndTakesText(t *testing.T) {
	x := composing(t)
	mustContain(t, pageText(x.App), "Introduce yourself in 3 sentences.")
	mustContain(t, pageText(x.App), "Empezá con: My name is...")
	if x.ft.level != domain.A2 {
		t.Errorf("level = %s", x.ft.level)
	}

	// Letters that are keys elsewhere (q, c, tab) belong to the text.
	a := typeText(t, x.App, "I has 30 years qc")
	a, _ = press(t, a, "backspace", "backspace", "backspace", "enter")
	a = typeText(t, a, "I am from Argentina.")

	if a.Screen() != Write {
		t.Fatalf("screen = %v", a.Screen())
	}
	mustContain(t, pageText(a), "I has 30 years")
	mustContain(t, pageText(a), "I am from Argentina.")
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

func TestWriteCorrectionShowsRecastsAndAddsCards(t *testing.T) {
	x := composing(t)
	a := typeText(t, x.App, "Hi. I has 30 years and I like it. I am from Argentina.")
	x.clock.t = x.clock.t.Add(6 * time.Minute)

	a, cmd := press(t, a, "ctrl+d")
	mustContain(t, pageText(a), "Correcting")
	a = run(t, a, cmd)

	if !strings.Contains(x.ft.text, "I has 30 years") {
		t.Errorf("sent %q", x.ft.text)
	}
	page := pageText(a)
	for _, want := range []string{"6/40", "¡Muy bien por presentarte!", "I am 30 years old. I am from Argentina.",
		"I has 30 years", "I am 30 years old", "La edad va con to be.", "Added 1 card to practice"} {
		mustContain(t, page, want)
	}
	if len(x.ad.added) != 1 || len(x.ad.added[0]) != 1 {
		t.Fatalf("added = %+v", x.ad.added)
	}
	c := x.ad.added[0][0]
	if c.Word != "I am 30 years old" || c.LastWrong != "I has 30 years" || c.Example != "I has 30 years and I like it." {
		t.Errorf("card = %+v", c)
	}
	d := x.log.deltas[0]
	if d.Minutes != 6 || len(d.Activities) != 1 || d.Activities[0] != "Writing: 6/40" {
		t.Errorf("delta = %+v", d)
	}
	mustContain(t, statusBar(a), "n new task")
}

func TestWriteNoErrorsAddsNoCards(t *testing.T) {
	x := composing(t)
	x.ft.correction = teacher.Correction{Corrected: "I am from Argentina.", Rubric: teacher.Rubric{Grammar: 10, Vocabulary: 10, Coherence: 10, Task: 10}, Comment: "¡Perfecto!"}
	a := typeText(t, x.App, "I am from Argentina.")
	a, cmd := press(t, a, "ctrl+d")
	a = run(t, a, cmd)

	if len(x.ad.added) != 0 {
		t.Errorf("added = %+v", x.ad.added)
	}
	mustContain(t, pageText(a), "No errors")
	if len(x.log.deltas) != 1 {
		t.Error("the writing still counts as study")
	}
}

func TestWriteCorrectionErrorKeepsTheDraft(t *testing.T) {
	x := composing(t)
	x.ft.err = errors.New("pi timed out")
	a := typeText(t, x.App, "I has a dog.")
	a, cmd := press(t, a, "ctrl+d")
	a = run(t, a, cmd)

	mustContain(t, pageText(a), "pi timed out")
	mustContain(t, pageText(a), "I has a dog.")
	x.ft.err = nil
	a, cmd = press(t, a, "ctrl+d")
	a = run(t, a, cmd)
	mustContain(t, pageText(a), "6/40")
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

func TestWriteAddErrorIsShown(t *testing.T) {
	x := composing(t)
	x.ad.err = errors.New("disk full")
	a := typeText(t, x.App, "I has 30 years.")
	a, cmd := press(t, a, "ctrl+d")
	a = run(t, a, cmd)
	mustContain(t, pageText(a), "disk full")
	if len(x.log.deltas) != 1 {
		t.Error("the writing still counts as study")
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

func TestWriteFailedCardsCanBeSavedAgain(t *testing.T) {
	x := composing(t)
	x.ad.err = errors.New("disk full")
	a := typeText(t, x.App, "I has 30 years.")
	a, cmd := press(t, a, "ctrl+d")
	a = run(t, a, cmd)
	mustContain(t, statusBar(a), "s save cards")

	x.ad.err = nil
	a, _ = press(t, a, "s")

	mustContain(t, pageText(a), "Added 1 card to practice")
	if len(x.ad.added) != 2 || len(x.log.deltas) != 1 {
		t.Errorf("add calls %d, recorded %d; the writing is recorded once", len(x.ad.added), len(x.log.deltas))
	}
	if a, _ = press(t, a, "s"); len(x.ad.added) != 2 {
		t.Error("s after a successful save must not add again")
	}
}
