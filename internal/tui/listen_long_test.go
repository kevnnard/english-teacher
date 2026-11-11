package tui

import (
	"errors"
	"reflect"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/kevnnard/english-teacher/internal/domain"
	"github.com/kevnnard/english-teacher/internal/speech"
	"github.com/kevnnard/english-teacher/internal/teacher"
)

// longEnv is the Listen screen with a fake teacher, speaker and study log.
type longEnv struct {
	App
	ft    *fakeTeacher
	sp    *fakeSpeaker // nil without audio
	log   *studyLog
	clock *testClock
}

// newLongApp opens Listen at A2 with the given voices; audio and ai say
// whether it has a speaker and a teacher.
func newLongApp(t *testing.T, audio, ai bool, voices []string) longEnv {
	t.Helper()
	x := longEnv{ft: &fakeTeacher{listening: sampleListening()}, log: &studyLog{}, clock: &testClock{t: fixedNow}}
	cfg := Config{
		Cards: deck(), Save: (&recorder{}).save, Now: x.clock.now, Start: Listen, MaxNew: DefaultMaxNew,
		Profile: domain.Profile{Level: domain.A2}, Record: x.log.record, Voices: voices,
	}
	if ai {
		cfg.Teacher = x.ft
	}
	if audio {
		x.sp = &fakeSpeaker{}
		cfg.Speaker = x.sp
	}
	x.App = send(t, NewApp(cfg), tea.WindowSizeMsg{Width: 120, Height: 40})
	return x
}

// sampleListening is a dialogue of four sentences by two speakers, with
// three questions whose right answer is always 1.
func sampleListening() teacher.Listening {
	q := func(s string) teacher.QuizItem {
		return teacher.QuizItem{Question: s, Options: []string{"Right", "Wrong", "Other"}, Answer: 0, Why: "Lo dice Ana."}
	}
	return teacher.Listening{
		Title: "At the market", Kind: teacher.ListeningDialogue,
		Lines: []teacher.ListeningLine{
			{Speaker: "Ana", Text: "Hi Tom!"},
			{Speaker: "Tom", Text: "Hello Ana."},
			{Speaker: "Ana", Text: "I want some fruit."},
			{Speaker: "Tom", Text: "The apples are good."},
		},
		Questions: []teacher.QuizItem{q("What does Ana want?"), q("Who is Tom?"), q("Where are they?")},
	}
}

var twoVoices = []string{"en_US-amy-medium", "en_US-ryan-medium"}

// startLong presses s and runs the request, as Bubble Tea would; it
// returns the command of the first sentence, not yet run.
func startLong(t *testing.T, x longEnv) (App, tea.Cmd) {
	t.Helper()
	a, cmd := press(t, x.App, "s")
	mustContain(t, pageText(a), "Writing a text for you")
	return sendCmd(t, a, cmd())
}

// playAll runs cmd and every sentence it chains to, as Bubble Tea would.
func playAll(t *testing.T, a App, cmd tea.Cmd) App {
	t.Helper()
	for cmd != nil {
		a, cmd = sendCmd(t, a, cmd())
	}
	return a
}

func TestLongListeningFlowListenThenQuestionsThenTranscript(t *testing.T) {
	x := newLongApp(t, true, true, twoVoices)
	mustContain(t, statusBar(x.App), "s long listening")

	// 1. Generate: one call at the learner's level, a dialogue first.
	a, cmd := startLong(t, x)
	if !reflect.DeepEqual(x.ft.listeningCalls, []teacher.ListeningKind{teacher.ListeningDialogue}) || x.ft.level != domain.A2 {
		t.Fatalf("calls %v at %s", x.ft.listeningCalls, x.ft.level)
	}

	// 2. Listen with progress only: no sentence on screen.
	page := pageText(a)
	mustContain(t, page, "Long listening", "Sentence 1 of 4", "playing")
	mustNotContain(t, page, "Hi Tom!", "fruit", "What does Ana want?")
	mustContain(t, statusBar(a), "space pause", "r replay all")
	a, cmd = sendCmd(t, a, cmd()) // sentence 1 ends; sentence 2 starts
	mustContain(t, pageText(a), "Sentence 2 of 4")
	if a, _ := press(t, a, "enter"); a.long.phase != longListening {
		t.Fatal("enter must not open the questions before the text was heard")
	}
	a = playAll(t, a, cmd)
	mustSay(t, x.sp, "Hi Tom!", "Hello Ana.", "I want some fruit.", "The apples are good.")
	mustContain(t, pageText(a), "Press enter for the questions")
	mustNotContain(t, pageText(a), "Hi Tom!")

	// 3. Questions, one at a time, still without the text.
	a, _ = press(t, a, "enter")
	mustContain(t, pageText(a), "Question 1 of 3", "What does Ana want?", "1  Right")
	mustNotContain(t, pageText(a), "Hi Tom!")
	a, _ = press(t, a, "1")
	mustContain(t, pageText(a), "✓ Right", "Lo dice Ana.")
	a, _ = press(t, a, "enter", "2") // wrong
	mustContain(t, pageText(a), "✗ Not quite. The answer is 1: Right")
	if len(x.log.deltas) != 0 {
		t.Fatal("recorded before the last question")
	}
	a, _ = press(t, a, "enter", "1")

	// 4. Recorded once: "Listening: N/M" and the minutes.
	if len(x.log.deltas) != 1 || !reflect.DeepEqual(x.log.deltas[0].Activities, []string{"Listening: 2/3"}) {
		t.Fatalf("recorded %+v", x.log.deltas)
	}
	mustContain(t, pageText(a), "Press enter for the transcript")

	// 5. The transcript, sentence by sentence; p replays the selected one.
	a, _ = press(t, a, "enter")
	mustContain(t, pageText(a), "2/3", "Ana: Hi Tom!", "Tom: The apples are good.")
	a, _ = press(t, a, "down", "down")
	a, cmd = press(t, a, "p")
	a = run(t, a, cmd)
	x.clock.t = x.clock.t.Add(time.Second)
	a, cmd = press(t, a, "p") // twice quickly: slower
	a = run(t, a, cmd)
	want := []spoken{
		{"I want some fruit.", speech.Options{Voice: twoVoices[0]}},
		{"I want some fruit.", speech.Options{Voice: twoVoices[0], Slow: true}},
	}
	if !reflect.DeepEqual(x.sp.said, want) {
		t.Errorf("transcript p said %+v, want %+v", x.sp.said, want)
	}
	mustContain(t, statusBar(a), "p hear it", "n new text")

	// n asks for a new text: a story this time.
	a, cmd = press(t, a, "n")
	run(t, a, cmd)
	if got := x.ft.listeningCalls; len(got) != 2 || got[1] != teacher.ListeningStory {
		t.Errorf("calls %v, want a story second", got)
	}
}

func TestLongListeningAlternatesVoicesPerSpeaker(t *testing.T) {
	x := newLongApp(t, true, true, twoVoices)
	a, cmd := startLong(t, x)
	playAll(t, a, cmd)
	var voices []string
	for _, s := range x.sp.said {
		voices = append(voices, s.opts.Voice)
	}
	want := []string{twoVoices[0], twoVoices[1], twoVoices[0], twoVoices[1]} // Ana, Tom, Ana, Tom
	if !reflect.DeepEqual(voices, want) {
		t.Errorf("voices %q, want %q", voices, want)
	}

	// With one voice, every line uses the configured one.
	y := newLongApp(t, true, true, twoVoices[:1])
	a, cmd = startLong(t, y)
	playAll(t, a, cmd)
	for _, s := range y.sp.said {
		if s.opts.Voice != "" {
			t.Errorf("one voice: said %+v with a voice", s)
		}
	}
	if len(y.sp.said) != 4 {
		t.Errorf("one voice: said %d sentences, want 4", len(y.sp.said))
	}
}

func TestLongListeningEscCancelsPlayback(t *testing.T) {
	x := newLongApp(t, true, true, twoVoices)
	a, cmd := startLong(t, x)
	a, _ = press(t, a, "esc")
	if a.Screen() != Home || a.long.playing {
		t.Fatalf("screen %v, playing %v", a.Screen(), a.long.playing)
	}
	// The sentence already on its way gets a cancelled context, and
	// nothing plays after it.
	a, next := sendCmd(t, a, cmd())
	if len(x.sp.ctxs) != 1 || x.sp.ctxs[0].Err() == nil {
		t.Fatalf("contexts %v: want one, cancelled", x.sp.ctxs)
	}
	if next != nil {
		t.Error("a cancelled play must not chain to the next sentence")
	}

	// Leaving with tab cancels too, and so does pausing.
	y := newLongApp(t, true, true, twoVoices)
	a, cmd = startLong(t, y)
	a, _ = press(t, a, "tab")
	cmd()
	if y.sp.ctxs[0].Err() == nil {
		t.Error("tab must cancel the playback")
	}
	z := newLongApp(t, true, true, twoVoices)
	a, cmd = startLong(t, z)
	a, _ = press(t, a, "space")
	mustContain(t, pageText(a), "Paused at sentence 1 of 4")
	cmd()
	if z.sp.ctxs[0].Err() == nil {
		t.Error("space must cancel the sentence playing")
	}
	// space again goes on from the interrupted sentence; r from the start.
	a, cmd = press(t, a, "space")
	a, _ = sendCmd(t, a, cmd())
	a, cmd = press(t, a, "r")
	sendCmd(t, a, cmd())
	if got := z.sp.texts(); !reflect.DeepEqual(got, []string{"Hi Tom!", "Hi Tom!", "Hi Tom!"}) {
		t.Errorf("said %q", got)
	}
}

func TestLongListeningMessages(t *testing.T) {
	// The AI fails: the error and how to try again; no audio plays.
	x := newLongApp(t, true, true, nil)
	x.ft.listeningErr = errors.New("pi timed out")
	a, cmd := startLong(t, x)
	if cmd != nil || len(x.sp.said) != 0 {
		t.Error("a failed text must not play")
	}
	mustContain(t, pageText(a), "✗ Could not get a text: pi timed out", "Press enter to try again")
	a, cmd = press(t, a, "enter")
	run(t, a, cmd)
	if len(x.ft.listeningCalls) != 2 {
		t.Errorf("enter must ask again: %d calls", len(x.ft.listeningCalls))
	}

	// No pi: the teacher is off, and nothing is asked.
	y := newLongApp(t, true, false, nil)
	a, cmd = press(t, y.App, "s")
	if cmd != nil {
		t.Error("s without a teacher must not start a command")
	}
	mustContain(t, pageText(a), "Long listening", "The AI teacher is off", "the pi command")

	// No speaker: the audio-off notice, and no AI call.
	z := newLongApp(t, false, true, nil)
	a, cmd = press(t, z.App, "s")
	if cmd != nil || len(z.ft.listeningCalls) != 0 {
		t.Errorf("s without audio: cmd %v, calls %v", cmd != nil, z.ft.listeningCalls)
	}
	mustContain(t, pageText(a), "Long listening", audioOffNotice)

	// s goes back to dictation.
	a, _ = press(t, a, "s")
	mustContain(t, pageText(a), "Dictation")
}

func TestLongListeningAudioErrorOpensTheQuestions(t *testing.T) {
	x := newLongApp(t, true, true, nil)
	x.sp.err = errors.New("player crashed")
	a, cmd := startLong(t, x)
	a = playAll(t, a, cmd)
	if len(x.sp.said) != 1 {
		t.Errorf("said %d sentences after an error, want 1", len(x.sp.said))
	}
	mustContain(t, screen(a), "Could not play audio: player crashed")
	a, _ = press(t, a, "enter")
	mustContain(t, pageText(a), "Question 1 of 3")
}

func TestHelpShowsLongListeningAndFits(t *testing.T) {
	x := newLongApp(t, true, true, nil)
	a := send(t, x.App, tea.WindowSizeMsg{Width: 80, Height: 24}, key("?"))
	mustContain(t, pageText(a), "long listening", "pause / go on", "ctrl+c")
}
