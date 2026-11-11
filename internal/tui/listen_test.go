package tui

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/kevnnard/english-teacher/internal/speech"
)

// spoken is one call to the fake speaker.
type spoken struct {
	text string
	opts speech.Options
}

// fakeSpeaker records what the app asks it to say; it never plays audio.
type fakeSpeaker struct {
	said []spoken
	err  error // what every Say returns
}

func (f *fakeSpeaker) Say(_ context.Context, text string, opts speech.Options) error {
	f.said = append(f.said, spoken{text, opts})
	return f.err
}

// texts are the spoken texts, in order.
func (f *fakeSpeaker) texts() []string {
	var out []string
	for _, s := range f.said {
		out = append(out, s.text)
	}
	return out
}

// withSpeaker gives a an audio speaker, as Config.Speaker would.
func withSpeaker(a App) (App, *fakeSpeaker) {
	sp := &fakeSpeaker{}
	a.speaker = sp
	return a, sp
}

// listen presses p and runs the command it returns, as Bubble Tea would.
func listen(t *testing.T, a App, k string) App {
	t.Helper()
	a, cmd := press(t, a, k)
	return run(t, a, cmd)
}

func mustSay(t *testing.T, sp *fakeSpeaker, want ...string) {
	t.Helper()
	if got := sp.texts(); !reflect.DeepEqual(got, want) {
		t.Errorf("said %q, want %q", got, want)
	}
	sp.said = nil
}

func TestListenReviewRecognizeSaysTheWordThenTheExampleOnReveal(t *testing.T) {
	a, sp := withSpeaker(newApp(t, deck(), &recorder{}, Review, 120, 40))
	c := a.cards[a.review.queue[0]]

	a = listen(t, a, "p")
	mustSay(t, sp, c.Word)
	if a.review.phase != front {
		t.Error("p must not reveal the answer")
	}

	a, _ = press(t, a, "space")
	listen(t, a, "p")
	mustSay(t, sp, c.Word, c.Example)
}

func TestListenReviewProduceWaitsForTheCheck(t *testing.T) {
	a, sp := withSpeaker(newApp(t, prodDeck(), &recorder{}, Review, 120, 40))

	// While typing, p is a letter of the answer: speaking would give it away.
	a, cmd := press(t, a, "p")
	if cmd != nil || a.review.input != "p" || len(sp.said) != 0 {
		t.Fatalf("p while typing: input %q, said %q", a.review.input, sp.texts())
	}
	mustNotContain(t, statusBar(a), "p listen")

	a, _ = press(t, a, "backspace")
	a = typeText(t, a, "nice to meet you")
	a, _ = press(t, a, "enter")
	listen(t, a, "p")
	mustSay(t, sp, "nice to meet you")
}

func TestListenLearnSaysTheSelectedWordOrTheWholeText(t *testing.T) {
	x := loadedLearn(t)
	a, sp := withSpeaker(x.App)

	a = listen(t, a, "p")
	mustSay(t, sp, "teacher")
	a, _ = press(t, a, "down")
	a = listen(t, a, "p")
	mustSay(t, sp, "how about")

	listen(t, a, "P")
	if len(sp.said) != 1 || sp.said[0].text != sampleReading().Text || sp.said[0].opts.Slow {
		t.Errorf("P said %+v, want the whole text at normal speed", sp.said)
	}
}

func TestListenWriteSaysTheRightVersionOfTheUnfixedError(t *testing.T) {
	x := composing(t)
	a, sp := withSpeaker(x.App)
	x.App = a

	// While composing, p is a letter of the text.
	a, _ = press(t, a, "p")
	if a.write.text != "p" || len(sp.said) != 0 {
		t.Fatalf("p while composing: text %q, said %q", a.write.text, sp.texts())
	}

	a = feedback(t, x) // the third error, "two dog", is not fixed
	mustContain(t, statusBar(a), "p listen")
	a = listen(t, a, "p")
	mustSay(t, sp, "two dogs")

	a, _ = press(t, a, "enter") // rewrite
	a, _ = press(t, a, "p")
	if a.write.input != "p" || len(sp.said) != 0 {
		t.Fatalf("p while rewriting: input %q, said %q", a.write.input, sp.texts())
	}
	a, _ = press(t, a, "backspace")
	a = typeText(t, a, "two dogs")
	a, _ = press(t, a, "enter")
	listen(t, a, "p")
	mustSay(t, sp, "two dogs")
}

func TestListenQuizSaysTheRightAnswerOnlyAfterTheCheck(t *testing.T) {
	x := newQuizApp(t, quizDeck(), nil)
	a, sp := withSpeaker(x.App)
	a, _ = press(t, a, "enter") // the first question is multiple choice

	if a, cmd := press(t, a, "p"); cmd != nil || len(sp.said) != 0 || a.quiz.phase != quizAsking {
		t.Fatalf("p before the answer said %q", sp.texts())
	}
	mustNotContain(t, statusBar(a), "p listen")

	want := question(a).word
	a, _ = press(t, a, wrongOption(question(a)))
	mustContain(t, statusBar(a), "p listen")
	a = listen(t, a, "p")
	mustSay(t, sp, want)

	a, _ = press(t, a, "enter") // the second question is typed
	a, _ = press(t, a, "p")
	if a.quiz.input != "p" || len(sp.said) != 0 {
		t.Errorf("p while typing: input %q, said %q", a.quiz.input, sp.texts())
	}
}

func TestListenTwiceQuicklyIsSlower(t *testing.T) {
	a, clock := newAppClock(t, deck(), &recorder{}, Review, 120, 40)
	a, sp := withSpeaker(a)

	a = listen(t, a, "p")
	clock.t = clock.t.Add(time.Second)
	a = listen(t, a, "p")
	clock.t = clock.t.Add(5 * time.Second)
	listen(t, a, "p")

	var slow []bool
	for _, s := range sp.said {
		slow = append(slow, s.opts.Slow)
	}
	if !reflect.DeepEqual(slow, []bool{false, true, false}) {
		t.Errorf("slow = %v, want [false true false]", slow)
	}
}

func TestListenWithoutAudioSaysHowToSetItUp(t *testing.T) {
	a := newApp(t, deck(), &recorder{}, Review, 80, 24)
	mustNotContain(t, statusBar(a), "p listen")

	a, cmd := press(t, a, "p")
	if cmd != nil {
		t.Error("p without audio must not start a command")
	}
	mustContain(t, screen(a), "Audio is off: run `teacher doctor`")

	// Everything else works as before, and the notice goes away.
	a, _ = press(t, a, "space")
	if a.review.phase != revealed {
		t.Error("space must still reveal")
	}
	mustNotContain(t, screen(a), "Audio is off")
}

func TestListenErrorsShowANotice(t *testing.T) {
	tests := []struct {
		err  error
		want string
	}{
		{speech.Unavailable("voice not found", "run `teacher setup voice`"), "Audio is off: run `teacher setup voice`"},
		{errors.New("player crashed"), "Could not play audio: player crashed"},
	}
	for _, tt := range tests {
		a, sp := withSpeaker(newApp(t, deck(), &recorder{}, Review, 120, 40))
		sp.err = tt.err
		a = listen(t, a, "p")
		mustContain(t, screen(a), tt.want)
	}

	// A play that works shows nothing.
	a, _ := withSpeaker(newApp(t, deck(), &recorder{}, Review, 120, 40))
	a = listen(t, a, "p")
	mustNotContain(t, screen(a), "Audio is off", "Could not play")
}

func TestListenHintsOnlyWithASpeaker(t *testing.T) {
	a := newApp(t, deck(), &recorder{}, Review, 120, 40)
	mustNotContain(t, statusBar(a), "p listen")
	a, _ = withSpeaker(a)
	mustContain(t, statusBar(a), "p listen")

	x := loadedLearn(t)
	mustNotContain(t, statusBar(x.App), "p listen", "P read all")
	b, _ := withSpeaker(x.App)
	mustContain(t, statusBar(b), "p listen", "P read all")
}

func TestHelpShowsTheListenKeyAndStillFits(t *testing.T) {
	tests := []struct {
		screen     Screen
		want, last string // the listen key and the last key of the section
	}{
		{Review, "listen to the word", "listen to the word"},
		{Learn, "hear the selected word / the whole text", "hear the selected word"},
		{Write, "hear the right version", "your work stays"},
		{Quiz, "hear the right one", "the quiz stays"},
	}
	for _, tt := range tests {
		x := newAIApp(t, tt.screen)
		a := send(t, x.App, tea.WindowSizeMsg{Width: 80, Height: 24}, key("?"))
		page := pageText(a)
		mustContain(t, page, tt.want, tt.last, "ctrl+c")
	}
}
