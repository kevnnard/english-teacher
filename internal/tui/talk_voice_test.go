package tui

import (
	"errors"
	"fmt"
	"reflect"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/kevnnard/english-teacher/internal/domain"
	"github.com/kevnnard/english-teacher/internal/speech"
	"github.com/kevnnard/english-teacher/internal/teacher"
)

// voiceTalk is Talk with fake audio and a fake recorder and transcriber.
type voiceTalk struct {
	aiApp
	sp    *fakeSpeaker
	rec   *fakeRecorder
	tr    *fakeTranscriber
	voice *voiceLog
}

// newVoiceTalk opens Talk on a wide window. Without audio there is no
// speaker; without whisper there is no recorder and no transcriber.
func newVoiceTalk(t *testing.T, audio, whisper bool) voiceTalk {
	t.Helper()
	x := voiceTalk{aiApp: talkApp(t), voice: &voiceLog{}}
	if audio {
		x.sp = &fakeSpeaker{}
		x.App.speaker = x.sp
	}
	if whisper {
		x.rec = &fakeRecorder{log: x.voice}
		x.tr = &fakeTranscriber{log: x.voice}
		x.App.recorder, x.App.transcriber = x.rec, x.tr
	}
	x.App = send(t, x.App, tea.WindowSizeMsg{Width: 200, Height: 40})
	return x
}

// startCoffee picks "Order a coffee" and runs the command it returns,
// which says the opening when voice mode is on.
func (x *voiceTalk) startCoffee(t *testing.T) App {
	t.Helper()
	a, cmd := press(t, x.App, "down", "enter")
	if cmd != nil {
		a = run(t, a, cmd)
	}
	return a
}

// speakLine records a line that whisper hears as heard and returns the
// app once the transcript arrived.
func (x *voiceTalk) speakLine(t *testing.T, a App, heard string) App {
	t.Helper()
	x.tr.text = heard
	a, _ = press(t, a, "ctrl+r")
	a, cmd := press(t, a, "ctrl+r")
	return run(t, a, cmd)
}

// reply sends the input with enter and feeds the tutor's answer back,
// returning the command the answer produced (the spoken reply, if any).
func reply(t *testing.T, a App) (App, tea.Cmd) {
	t.Helper()
	a, cmd := press(t, a, "enter")
	if cmd == nil {
		t.Fatal("enter must send the line")
	}
	return sendCmd(t, a, cmd())
}

func TestTalkVoiceTranscriptGoesIntoTheInputAndIsNotSent(t *testing.T) {
	x := newVoiceTalk(t, false, true)
	a := typeText(t, x.startCoffee(t), "Hi,")

	a, _ = press(t, a, "ctrl+r")
	if ev := x.voice.take(); !reflect.DeepEqual(ev, []string{"start"}) {
		t.Errorf("events %q, want start", ev)
	}
	mustContain(t, pageText(a), "● Recording… ctrl+r to stop · esc cancel")
	a = typeText(t, a, "zz") // keys while recording do not type
	a, cmd := press(t, a, "ctrl+r")
	mustContain(t, pageText(a), "Listening…")
	if cmd == nil {
		t.Fatal("ctrl+r must stop and transcribe")
	}
	x.tr.text = " can I have a big coffee "
	a = run(t, a, cmd)
	if ev := x.voice.take(); !reflect.DeepEqual(ev, []string{"stop", "transcribe rec1.wav"}) {
		t.Errorf("events %q, want stop then transcribe", ev)
	}
	if a.talk.input != "Hi, can I have a big coffee" {
		t.Errorf("input = %q, want the transcript after the typed text", a.talk.input)
	}
	if len(x.ft.talkCalls) != 0 {
		t.Errorf("the line was sent: %+v", x.ft.talkCalls)
	}
	mustContain(t, pageText(a), "› Hi, can I have a big coffee")

	// The learner fixes it by typing, then sends it as usual.
	a = typeText(t, a, ", please?")
	a, _ = reply(t, a)
	if got := x.ft.talkCalls; len(got) != 1 || got[0][1].Text != "Hi, can I have a big coffee, please?" {
		t.Errorf("calls = %+v", got)
	}
	mustContain(t, pageText(a), "You: Hi, can I have a big coffee, please?")
}

func TestTalkVoiceEscCancelsTheRecordingAndKeepsTheInput(t *testing.T) {
	x := newVoiceTalk(t, false, true)
	a := typeText(t, x.startCoffee(t), "Hello")
	a, _ = press(t, a, "ctrl+r")
	a, cmd := press(t, a, "esc")
	a = run(t, a, cmd)

	if ev := x.voice.take(); !reflect.DeepEqual(ev, []string{"start", "cancel"}) {
		t.Errorf("events %q, want start then cancel", ev)
	}
	if a.Screen() != Talk || a.talk.phase != talkChatting || a.talk.input != "Hello" {
		t.Errorf("screen %v phase %v input %q, want Talk with the input kept", a.Screen(), a.talk.phase, a.talk.input)
	}
	mustContain(t, pageText(a), "Recording cancelled", "› Hello")
	a = typeText(t, a, "!") // letters type again
	if a.talk.input != "Hello!" {
		t.Errorf("input = %q", a.talk.input)
	}
}

func TestTalkVoiceEmptyTranscriptLeavesTheInputAndSaysSo(t *testing.T) {
	x := newVoiceTalk(t, false, true)
	a := typeText(t, x.startCoffee(t), "Hi")
	a = x.speakLine(t, a, "  ")
	if a.talk.input != "Hi" || a.talk.phase != talkChatting {
		t.Errorf("input %q phase %v, want the input unchanged", a.talk.input, a.talk.phase)
	}
	mustContain(t, flatPage(a), heardNothing, "› Hi")
}

func TestTalkVoiceCtrlCWhileRecordingCancelsIt(t *testing.T) {
	x := newVoiceTalk(t, false, true)
	a, _ := press(t, x.startCoffee(t), "ctrl+r")
	_, cmd := press(t, a, "ctrl+c")
	if !isQuit(cmd) {
		t.Error("ctrl+c must quit")
	}
	if ev := x.voice.take(); !reflect.DeepEqual(ev, []string{"start", "cancel"}) {
		t.Errorf("events %q: a recorder must not outlive the app", ev)
	}
}

func TestTalkVoiceStaleTranscriptIsIgnored(t *testing.T) {
	x := newVoiceTalk(t, false, true)
	a, _ := press(t, x.startCoffee(t), "ctrl+r")
	a, cmd := press(t, a, "ctrl+r")
	a, _ = press(t, a, "esc") // stop waiting for this one
	x.tr.text = "too late"
	a = run(t, a, cmd)
	if a.talk.input != "" || a.talk.phase != talkChatting {
		t.Errorf("input %q phase %v, want the old transcript ignored", a.talk.input, a.talk.phase)
	}
}

func TestTalkVoiceSpeaksTheReplyButNotTheRecast(t *testing.T) {
	x := newVoiceTalk(t, true, false)
	a := x.startCoffee(t)
	mustSay(t, x.sp, "Hi there! What can I get you?") // the opening

	a = typeText(t, a, coffeeLine)
	a, cmd := reply(t, a)
	if cmd == nil {
		t.Fatal("the reply must be spoken")
	}
	a = run(t, a, cmd)
	mustSay(t, x.sp, "A big coffee, sure! Milk?")
	mustContain(t, pageText(a), "↳ Can I have a big coffee, please?")

	// ctrl+p says the last reply again, never the recast; twice is slower.
	a, cmd = press(t, a, "ctrl+p")
	a = run(t, a, cmd)
	mustSay(t, x.sp, "A big coffee, sure! Milk?")
	a, cmd = press(t, a, "ctrl+p")
	run(t, a, cmd)
	if len(x.sp.said) != 1 || !x.sp.said[0].opts.Slow {
		t.Errorf("said %+v, want the reply again, slower", x.sp.said)
	}
}

func TestTalkVoiceCtrlTStopsAutoSpeakAndCtrlPStillReplays(t *testing.T) {
	x := newVoiceTalk(t, true, false)
	a := x.startCoffee(t)
	x.sp.said = nil
	mustContain(t, statusBar(a), "ctrl+t voice on")
	a, _ = press(t, a, "ctrl+t")
	mustContain(t, statusBar(a), "ctrl+t voice off")

	a = typeText(t, a, "Coffee")
	a, cmd := reply(t, a)
	if cmd != nil {
		t.Error("with voice mode off the reply must not be spoken")
	}
	mustContain(t, pageText(a), "Barista: A big coffee, sure! Milk?")
	a, cmd = press(t, a, "ctrl+p")
	a = run(t, a, cmd)
	mustSay(t, x.sp, "A big coffee, sure! Milk?")

	// Voice mode lasts the session: a new role-play keeps it off.
	a.startTalk()
	mustContain(t, statusBar(a), "ctrl+t voice off")
	a, _ = press(t, a, "ctrl+t")
	mustContain(t, statusBar(a), "ctrl+t voice on")
}

func TestTalkVoiceTheLastLineIsSpokenWithTheFeedbackAsked(t *testing.T) {
	x := newVoiceTalk(t, true, false)
	x.ft.turns = []teacher.Turn{{Reply: "Here you go!", Done: true}}
	a := typeText(t, x.startCoffee(t), "A coffee please")
	x.sp.said = nil
	a, cmd := reply(t, a)
	if cmd == nil || a.talk.phase != talkSummarizing {
		t.Fatalf("phase %v: want the feedback asked", a.talk.phase)
	}
	batch, ok := cmd().(tea.BatchMsg)
	if !ok {
		t.Fatal("want the reply spoken and the feedback asked together")
	}
	for _, c := range batch {
		a = send(t, a, c())
	}
	mustSay(t, x.sp, "Here you go!")
	if a.talk.phase != talkSummary {
		t.Errorf("phase %v, want the summary", a.talk.phase)
	}
}

func TestTalkVoiceUnavailable(t *testing.T) {
	// No recorder: ctrl+r says how to set up whisper; typing still works.
	x := newVoiceTalk(t, false, false)
	a, cmd := press(t, x.startCoffee(t), "ctrl+r")
	if cmd != nil || a.talk.phase != talkChatting {
		t.Errorf("phase %v: ctrl+r without whisper must not record", a.talk.phase)
	}
	mustContain(t, screen(a), whisperOffNotice)
	a = typeText(t, a, "Hi")
	if a.talk.input != "Hi" {
		t.Errorf("input = %q", a.talk.input)
	}
	// No speaker: the audio keys say audio is off.
	a, _ = press(t, a, "ctrl+p")
	mustContain(t, screen(a), audioOffNotice)

	// The recorder cannot start.
	x = newVoiceTalk(t, false, true)
	x.rec.err = speech.Unavailable("no recorder found", "install pw-record")
	a, _ = press(t, x.startCoffee(t), "ctrl+r")
	mustContain(t, screen(a), "no recorder found: install pw-record")
	if a.talk.phase != talkChatting {
		t.Errorf("phase %v, want chatting", a.talk.phase)
	}

	// The microphone did not start.
	x = newVoiceTalk(t, false, true)
	x.rec.stopErr = errors.New("the microphone did not start")
	a = typeText(t, x.startCoffee(t), "Hi")
	a = x.speakLine(t, a, "ignored")
	mustContain(t, screen(a), "the microphone did not start")
	if a.talk.input != "Hi" || a.talk.phase != talkChatting {
		t.Errorf("input %q phase %v", a.talk.input, a.talk.phase)
	}
}

func TestTalkVoiceHintsOnlyWithTheirCapability(t *testing.T) {
	tests := []struct {
		audio, whisper bool
		want, lack     []string
	}{
		{false, false, nil, []string{"ctrl+r", "ctrl+p", "ctrl+t"}},
		{true, false, []string{"ctrl+p replay", "ctrl+t voice on"}, []string{"ctrl+r"}},
		{false, true, []string{"ctrl+r speak"}, []string{"ctrl+p", "ctrl+t"}},
		{true, true, []string{"ctrl+r speak", "ctrl+p replay", "ctrl+t voice on"}, nil},
	}
	for _, tt := range tests {
		x := newVoiceTalk(t, tt.audio, tt.whisper)
		bar := statusBar(x.startCoffee(t))
		mustContain(t, bar, append([]string{"enter send"}, tt.want...)...)
		mustNotContain(t, bar, tt.lack...)
	}
}

func TestTalkHelpShowsTheVoiceKeysAndFits(t *testing.T) {
	a := newApp(t, deck(), &recorder{}, Talk, 80, 24)
	a, _ = press(t, a, "?")
	mustContain(t, pageText(a), "ctrl+r", "ctrl+t", "ctrl+p", "ctrl+c")
}

func TestTalkVoiceNoticeKeepsTheInputOn80x24(t *testing.T) {
	x := newVoiceTalk(t, false, false)
	x.App.profile.Level = domain.A1 // every frame shows, so the page is fullest
	a := send(t, x.startCoffee(t), tea.WindowSizeMsg{Width: 80, Height: 24})
	for i := 1; i <= 7; i++ {
		x.ft.turns = []teacher.Turn{{Reply: fmt.Sprintf("Reply number %d.", i)}}
		a, _ = reply(t, typeText(t, a, fmt.Sprintf("Line number %d", i)))
	}
	a = typeText(t, a, "last")
	a, _ = press(t, a, "ctrl+r")
	got := screen(a)
	mustContain(t, got, "› last", "Barista: Reply number 7.", whisperOffNotice)
	if n := len(screenLines(a)); n != 24 {
		t.Errorf("%d rows, want 24", n)
	}
}
