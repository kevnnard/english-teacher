package tui

import (
	"context"
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/kevnnard/english-teacher/internal/speech"
)

// Talk by voice (spec S7): while composing a line, ctrl+r records it and
// whisper writes it down INTO the input, after anything already typed.
// The line is never sent by itself: the learner reads what whisper heard,
// fixes it by typing and sends it with enter, so they notice their own
// words. In voice mode (on by default when audio works) the tutor's
// replies are read aloud as they arrive; ctrl+t turns that off for the
// session and ctrl+p says the last reply again. The recast under the
// learner's line is only shown, never spoken: it helps without
// interrupting.
//
// The keys are ctrl keys because the input has focus: every letter types.

// voiceOn reports whether the tutor's lines are read aloud as they come.
func (a App) voiceOn() bool { return a.speaker != nil && !a.talkQuiet }

// sayTutor returns a command that says a tutor line in voice mode, or nil.
func (a App) sayTutor(text string) tea.Cmd {
	if !a.voiceOn() {
		return nil
	}
	return a.say([]string{text}, speech.Options{})
}

// lastReply is the newest tutor line of the conversation.
func (t talkState) lastReply() string {
	for i := len(t.lines) - 1; i >= 0; i-- {
		if t.lines[i].Tutor {
			return t.lines[i].Text
		}
	}
	return ""
}

// updateTalkVoice handles the voice keys of a conversation: ctrl+r, ctrl+t
// and ctrl+p while typing, and the keys of a recording in progress.
func (a App) updateTalkVoice(k string) (tea.Model, tea.Cmd) {
	t := &a.talk
	switch t.phase {
	case talkChatting:
		switch k {
		case "ctrl+r":
			a.startTalkRecording()
		case "ctrl+t":
			if a.speaker == nil {
				a.listen.notice = audioOffNotice
				return a, nil
			}
			a.talkQuiet = !a.talkQuiet
		case "ctrl+p":
			return a.speak([]string{t.lastReply()}) // twice quickly is slower
		}
	case talkRecording:
		switch k {
		case "ctrl+r":
			rec := t.recording
			t.recording, t.phase = nil, talkListening
			return a, transcribe(rec, a.transcriber, transcribedMsg{try: t.try, talk: true})
		case "esc":
			rec := t.recording
			t.recording, t.phase = nil, talkChatting
			a.listen.notice = "Recording cancelled."
			return a, func() tea.Msg {
				rec.Cancel()
				return nil
			}
		}
	case talkListening:
		if k == "esc" {
			t.try++ // the transcript on its way is ignored
			t.phase = talkChatting
			a.listen.notice = "Recording cancelled."
		}
	}
	return a, nil
}

// startTalkRecording starts recording the learner's line, or says why it
// cannot. Starting a program takes milliseconds, so it happens right here.
func (a *App) startTalkRecording() {
	if a.recorder == nil || a.transcriber == nil {
		a.listen.notice = whisperOffNotice
		return
	}
	t := &a.talk
	rec, err := a.recorder.Start(context.Background())
	if err != nil {
		a.listen.notice = "Could not hear you: " + err.Error()
		return
	}
	t.recording, t.phase = rec, talkRecording
	t.try++
}

// gotTalkTranscript puts what whisper heard into the input, after what was
// typed, without sending it.
func (a App) gotTalkTranscript(msg transcribedMsg) App {
	t := &a.talk
	if msg.try != t.try || t.phase != talkListening {
		return a // a transcript of an older recording
	}
	t.phase = talkChatting
	heard := strings.TrimSpace(msg.text)
	switch {
	case msg.err != nil:
		a.listen.notice = "Could not hear you: " + msg.err.Error()
	case heard == "":
		a.listen.notice = heardNothing
	case strings.TrimSpace(t.input) == "":
		t.input = heard
	case strings.HasSuffix(t.input, " "):
		t.input += heard
	default:
		t.input += " " + heard
	}
	return a
}

// talkVoiceHints are the voice keys of a conversation, each only when the
// app has what it needs: a speaker for ctrl+p and ctrl+t, whisper and a
// recorder for ctrl+r.
func (a App) talkVoiceHints() []string {
	var hints []string
	if a.recorder != nil && a.transcriber != nil {
		hints = append(hints, "ctrl+r speak")
	}
	if a.speaker != nil {
		mode := "ctrl+t voice on"
		if a.talkQuiet {
			mode = "ctrl+t voice off"
		}
		hints = append(hints, "ctrl+p replay", mode)
	}
	return hints
}
