package tui

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/kevnnard/english-teacher/internal/domain"
	"github.com/kevnnard/english-teacher/internal/speech"
)

// voiceLog is what the fake recorder and transcriber were asked to do,
// in order, like "start", "stop" and "transcribe rec1.wav".
type voiceLog struct{ events []string }

func (l *voiceLog) add(e string) { l.events = append(l.events, e) }

// take returns the events so far and forgets them.
func (l *voiceLog) take() []string {
	e := l.events
	l.events = nil
	return e
}

// fakeRecorder never touches a microphone: each recording is a name.
type fakeRecorder struct {
	log     *voiceLog
	n       int
	err     error // what Start returns
	stopErr error // what Stop returns
}

func (r *fakeRecorder) Start(context.Context) (speech.Recording, error) {
	r.log.add("start")
	if r.err != nil {
		return nil, r.err
	}
	r.n++
	return &fakeRecording{r: r, wav: fmt.Sprintf("rec%d.wav", r.n)}, nil
}

type fakeRecording struct {
	r   *fakeRecorder
	wav string
}

func (f *fakeRecording) Stop() (string, error) {
	f.r.log.add("stop")
	return f.wav, f.r.stopErr
}

func (f *fakeRecording) Cancel() { f.r.log.add("cancel") }

// fakeTranscriber hears whatever text is set to.
type fakeTranscriber struct {
	log  *voiceLog
	text string
	err  error
}

func (f *fakeTranscriber) Transcribe(_ context.Context, wav string) (string, error) {
	f.log.add("transcribe " + wav)
	return f.text, f.err
}

// speakEnv is the Speak screen with fake audio, recorder and transcriber.
type speakEnv struct {
	App
	sp    *fakeSpeaker
	rec   *fakeRecorder
	tr    *fakeTranscriber
	voice *voiceLog
	log   *studyLog
	clock *testClock
}

// newSpeakApp opens Speak over cards. Without audio there is no speaker;
// without whisper there is no recorder and no transcriber.
func newSpeakApp(t *testing.T, cards []domain.Card, audio, whisper bool) speakEnv {
	t.Helper()
	x := speakEnv{voice: &voiceLog{}, log: &studyLog{}, clock: &testClock{t: fixedNow}}
	cfg := Config{
		Cards: cards, Save: (&recorder{}).save, Now: x.clock.now, Start: Speak, MaxNew: DefaultMaxNew,
		Record: x.log.record,
	}
	if audio {
		x.sp = &fakeSpeaker{}
		cfg.Speaker = x.sp
	}
	if whisper {
		x.rec = &fakeRecorder{log: x.voice}
		x.tr = &fakeTranscriber{log: x.voice}
		cfg.Recorder, cfg.Transcriber = x.rec, x.tr
	}
	x.App = send(t, NewApp(cfg), tea.WindowSizeMsg{Width: 120, Height: 40})
	return x
}

// sentence is the sentence on screen.
func sentence(a App) string { return a.speaking.items[a.speaking.pos].sentence }

// record presses enter twice and runs the transcription, as Bubble Tea
// would, with whisper hearing heard.
func (x *speakEnv) record(t *testing.T, a App, heard string) App {
	t.Helper()
	x.tr.text = heard
	a, _ = press(t, a, "enter")
	a, cmd := press(t, a, "enter")
	return run(t, a, cmd)
}

// flatPage is the page text as one line: wrapped sentences read whole.
func flatPage(a App) string {
	return strings.Join(strings.Fields(strings.ReplaceAll(pageText(a), "│", " ")), " ")
}

// marked is the marks as "word" or "!word" for words not understood.
func marked(a App) []string {
	var out []string
	for _, m := range a.speaking.marks {
		if m.Understood {
			out = append(out, m.Word)
		} else {
			out = append(out, "!"+m.Word)
		}
	}
	return out
}

// withoutWord is s without its word i.
func withoutWord(s string, i int) string {
	w := strings.Fields(s)
	return strings.Join(slices.Delete(w, i, i+1), " ")
}

func TestSpeakIntroOffersSentencesFromYourCards(t *testing.T) {
	x := newSpeakApp(t, deck(), true, true)
	mustContain(t, pageText(x.App), "Speak · Shadowing", "7 sentences from your cards", "Whisper")
	mustContain(t, statusBar(x.App), "enter start")
	if len(x.sp.said) != 0 || len(x.voice.events) != 0 {
		t.Error("the intro must not play or record anything")
	}
}

func TestSpeakTakesAtMostEightSentences(t *testing.T) {
	x := newSpeakApp(t, dictDeck(), true, true)
	a, _ := press(t, x.App, "enter")
	if len(a.speaking.items) != speakItems || speakItems != 8 {
		t.Fatalf("%d items, want 8", len(a.speaking.items))
	}
	for _, it := range a.speaking.items {
		if n := len(strings.Fields(it.sentence)); n > maxDictationWords {
			t.Errorf("%q has %d words", it.sentence, n)
		}
	}
}

func TestSpeakPlaysThenRecordsThenTranscribesInOrder(t *testing.T) {
	x := newSpeakApp(t, deck(), true, true)
	a, cmd := press(t, x.App, "enter")
	s := sentence(a)
	a = said(t, a, cmd, x.sp, spoken{s, speech.Options{}}) // played on arrival
	mustContain(t, pageText(a), "Sentence 1 of 7", s, "Press enter and say it")
	mustContain(t, statusBar(a), "enter record", "p again", "s slower")

	a, cmd = press(t, a, "p")
	a = said(t, a, cmd, x.sp, spoken{s, speech.Options{}})
	a, cmd = press(t, a, "s")
	a = said(t, a, cmd, x.sp, spoken{s, speech.Options{Slow: true}})

	a, cmd = press(t, a, "enter")
	if ev := x.voice.take(); !reflect.DeepEqual(ev, []string{"start"}) {
		t.Fatalf("enter: %q, want start", ev)
	}
	if cmd != nil {
		t.Error("starting must not return a command")
	}
	mustContain(t, pageText(a), "● Recording… enter to stop")
	mustContain(t, statusBar(a), "enter stop", "esc cancel")

	// Keys other than enter and esc do nothing while recording.
	a, _ = press(t, a, "tab", "p", "?")
	if a.Screen() != Speak || a.help || len(x.sp.said) != 0 {
		t.Fatalf("keys leaked while recording: screen %v help %v", a.Screen(), a.help)
	}

	x.tr.text = s
	a, cmd = press(t, a, "enter")
	mustContain(t, pageText(a), "Listening…")
	if len(x.voice.events) != 0 {
		t.Fatalf("stop ran before its command: %q", x.voice.events)
	}
	a = run(t, a, cmd)
	if ev := x.voice.take(); !reflect.DeepEqual(ev, []string{"stop", "transcribe rec1.wav"}) {
		t.Errorf("events %q, want stop then transcribe", ev)
	}
	mustContain(t, pageText(a), "Whisper heard: "+s, "✓ Whisper understood every word")
	mustContain(t, statusBar(a), "enter next", "r try again")
}

func TestSpeakMarksEachWord(t *testing.T) {
	x := newSpeakApp(t, deck(), true, true)
	a, _ := press(t, x.App, "enter")
	s := sentence(a)
	words := strings.Fields(s)
	heard := withoutWord(s, 1)

	a = x.record(t, a, heard)

	want := slices.Clone(words)
	want[1] = "!" + words[1]
	if got := marked(a); !reflect.DeepEqual(got, want) {
		t.Errorf("marks %q, want %q", got, want)
	}
	page := pageText(a)
	mustContain(t, page, "Whisper heard: "+heard, "Not understood: "+dictationKey(words[1]))
	for _, w := range words {
		mustContain(t, page, w) // every word of the sentence is on screen
	}
}

func TestSpeakEmptyTranscriptUnderstandsNothing(t *testing.T) {
	x := newSpeakApp(t, deck(), true, true)
	a, _ := press(t, x.App, "enter")
	a = x.record(t, a, "")
	for _, m := range a.speaking.marks {
		if m.Understood {
			t.Errorf("%q understood in silence", m.Word)
		}
	}
	mustContain(t, pageText(a), "Whisper heard nothing")
	mustContain(t, pageText(a), "microphone")
}

func TestSpeakEscCancelsTheRecording(t *testing.T) {
	x := newSpeakApp(t, deck(), true, true)
	a, _ := press(t, x.App, "enter")
	s := sentence(a)
	a, _ = press(t, a, "enter")
	a, cmd := press(t, a, "esc")
	a = run(t, a, cmd)

	if ev := x.voice.take(); !reflect.DeepEqual(ev, []string{"start", "cancel"}) {
		t.Errorf("events %q, want start then cancel", ev)
	}
	if a.Screen() != Speak || a.speaking.phase != speakReady || sentence(a) != s {
		t.Errorf("after esc: screen %v phase %v, want the same sentence ready", a.Screen(), a.speaking.phase)
	}
	mustContain(t, pageText(a), "Recording cancelled")
}

func TestSpeakCtrlCWhileRecordingCancelsIt(t *testing.T) {
	x := newSpeakApp(t, deck(), true, true)
	a, _ := press(t, x.App, "enter", "enter")
	_, cmd := press(t, a, "ctrl+c")
	if !isQuit(cmd) {
		t.Error("ctrl+c must quit")
	}
	if ev := x.voice.take(); !reflect.DeepEqual(ev, []string{"start", "cancel"}) {
		t.Errorf("events %q: a recorder must not outlive the app", ev)
	}
}

func TestSpeakRetryPlaysTheSameSentenceAgain(t *testing.T) {
	x := newSpeakApp(t, deck(), true, true)
	a, cmd := press(t, x.App, "enter")
	a = run(t, a, cmd)
	s := sentence(a)
	a = x.record(t, a, "")
	x.sp.said = nil

	a, cmd = press(t, a, "r")
	a = said(t, a, cmd, x.sp, spoken{s, speech.Options{}})
	if a.speaking.phase != speakReady || a.speaking.pos != 0 || len(a.speaking.marks) != 0 {
		t.Errorf("r: phase %v pos %d marks %v, want the same sentence ready", a.speaking.phase, a.speaking.pos, a.speaking.marks)
	}
	a = x.record(t, a, s)
	if ev := x.voice.take(); !reflect.DeepEqual(ev[len(ev)-3:], []string{"start", "stop", "transcribe rec2.wav"}) {
		t.Errorf("events %q, want a second recording", ev)
	}
	mustContain(t, pageText(a), "✓ Whisper understood every word")
}

func TestSpeakRoundPracticesMissedSentencesAgainAndRecordsOnce(t *testing.T) {
	x := newSpeakApp(t, deck(), true, true)
	a, cmd := press(t, x.App, "enter")
	n := len(a.speaking.items)
	var missedSentence, missedWord string
	for i := range n {
		a = run(t, a, cmd) // played on arrival
		mustContain(t, pageText(a), fmt.Sprintf("Sentence %d of %d", i+1, n))
		s := sentence(a)
		if i == 1 {
			missedSentence, missedWord = s, dictationKey(strings.Fields(s)[0])
			a = x.record(t, a, withoutWord(s, 0))
			a, cmd = press(t, a, "r") // a retry that works still practises it again
			a = run(t, a, cmd)
			a = x.record(t, a, s)
		} else {
			a = x.record(t, a, s)
		}
		if len(x.log.deltas) != 0 {
			t.Fatalf("recorded %v before the end", x.log.deltas)
		}
		x.clock.t = x.clock.t.Add(5 * time.Minute)
		a, cmd = press(t, a, "enter")
	}

	// One practice-again turn, for the sentence with a word not understood.
	a = run(t, a, cmd)
	if len(a.speaking.items) != n+1 || sentence(a) != missedSentence || !a.speaking.items[n].again {
		t.Fatalf("items %+v, want %q again at the end", a.speaking.items[n:], missedSentence)
	}
	mustContain(t, pageText(a), "Practice again · 1 of 1", missedSentence)
	a = x.record(t, a, withoutWord(missedSentence, 0)) // missed again: no third turn
	a, cmd = press(t, a, "enter")
	if cmd != nil {
		t.Error("the result must not play anything")
	}

	page := pageText(a)
	mustContain(t, page, fmt.Sprintf("%d of %d sentences fully understood", n-1, n),
		"saved in today's log", "Words to practise", missedWord)
	mustContain(t, statusBar(a), "n new round")
	want := []domain.Day{{Date: domain.DateOf(fixedNow), Minutes: maxSpeakMinutes, Activities: []string{fmt.Sprintf("Speak: %d/%d", n-1, n)}}}
	if !reflect.DeepEqual(x.log.deltas, want) {
		t.Errorf("recorded %v, want %v", x.log.deltas, want)
	}

	a, _ = press(t, a, "tab", "shift+tab", "enter", "x")
	if len(x.log.deltas) != 1 {
		t.Errorf("recorded %d times, want once", len(x.log.deltas))
	}
	a, _ = press(t, a, "n")
	if a.speaking.phase != speakIntro {
		t.Errorf("n: phase %v, want the intro", a.speaking.phase)
	}
}

func TestSpeakFailedRecordingCanBeTriedAgain(t *testing.T) {
	x := newSpeakApp(t, deck(), true, true)
	a, _ := press(t, x.App, "enter")
	x.rec.stopErr = errors.New("nothing was recorded: check the microphone")
	a = x.record(t, a, "")

	if a.speaking.phase != speakReady || a.speaking.items[0].tries != 0 {
		t.Errorf("phase %v tries %v: a failed recording is not a try", a.speaking.phase, a.speaking.items[0].tries)
	}
	mustContain(t, pageText(a), "Could not hear you: nothing was recorded")
	if ev := x.voice.take(); !reflect.DeepEqual(ev, []string{"start", "stop"}) {
		t.Errorf("events %q: nothing to transcribe", ev)
	}
}

func TestSpeakStaleTranscriptIsIgnored(t *testing.T) {
	x := newSpeakApp(t, deck(), true, true)
	a, _ := press(t, x.App, "enter")
	a = send(t, a, transcribedMsg{try: 99, text: "hello"})
	if a.speaking.phase != speakReady {
		t.Errorf("phase %v: an old transcript changed the page", a.speaking.phase)
	}
}

func TestSpeakWithoutWhisperSaysHowToInstallIt(t *testing.T) {
	x := newSpeakApp(t, deck(), true, false)
	mustContain(t, flatPage(x.App), "Whisper is off", "brew install whisper-cpp", "teacher setup whisper", "pw-record")
	a, cmd := press(t, x.App, "enter")
	if cmd != nil || a.speaking.phase != speakIntro || len(x.sp.said) != 0 {
		t.Error("enter must not start without whisper")
	}
	mustNotContain(t, statusBar(a), "enter start")
}

func TestSpeakUnavailableAtRuntimeSaysHowToInstall(t *testing.T) {
	x := newSpeakApp(t, deck(), true, true)
	x.rec.err = speech.Unavailable("pw-record not found", "install pw-record (pipewire), arecord (alsa-utils) or sox")
	a, _ := press(t, x.App, "enter", "enter")

	if a.speaking.phase != speakIntro {
		t.Errorf("phase %v, want the install page", a.speaking.phase)
	}
	mustContain(t, flatPage(a), "Whisper is off", "pw-record not found", "brew install whisper-cpp", "teacher setup whisper")

	// The transcriber can be the missing piece too.
	x = newSpeakApp(t, deck(), true, true)
	x.tr.err = speech.Unavailable("whisper model not found", "run `teacher setup whisper`")
	a, _ = press(t, x.App, "enter")
	a = x.record(t, a, "")
	mustContain(t, flatPage(a), "Whisper is off", "whisper model not found")
}

func TestSpeakWithoutAudioSaysHowToTurnItOn(t *testing.T) {
	x := newSpeakApp(t, deck(), false, true)
	mustContain(t, pageText(x.App), audioOffNotice, "teacher setup voice")
	a, cmd := press(t, x.App, "enter")
	if cmd != nil || a.speaking.phase != speakIntro || len(x.voice.events) != 0 {
		t.Error("enter must not start without audio")
	}
}

func TestSpeakHelpAndTab(t *testing.T) {
	a := newApp(t, deck(), &recorder{}, Speak, 80, 24)
	if !strings.Contains(screenLines(a)[0], "Pairs Speak Words") {
		t.Errorf("tab bar %q: want Speak after Pairs", screenLines(a)[0])
	}
	a, _ = press(t, a, "?")
	mustContain(t, pageText(a), "Speak", "start / stop recording", "ctrl+c")
}
