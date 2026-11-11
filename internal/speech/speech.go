// Package speech is the audio port of the app: text-to-speech, recording
// and speech-to-text behind interfaces, so screens can speak and listen
// without knowing which programs do it. Audio is optional. When it is not
// set up, a Speaker, Recorder or Transcriber returns an error that says
// how to set it up, and nothing else changes.
package speech

import (
	"context"
	"errors"
)

// Speaker reads text aloud.
type Speaker interface {
	// Say speaks text and returns when it has been played.
	Say(ctx context.Context, text string, opts Options) error
}

// Recorder records the learner's voice from the microphone.
type Recorder interface {
	// Start begins a recording, which runs until it is stopped or
	// cancelled.
	Start(ctx context.Context) (Recording, error)
}

// Recording is one recording in progress.
type Recording interface {
	// Stop ends the recording and returns the WAV file it wrote: 16 kHz,
	// mono, for a Transcriber. The file is temporary; Transcribe removes it.
	Stop() (wavPath string, err error)
	// Cancel ends the recording and throws it away.
	Cancel()
}

// Transcriber turns recorded speech into text.
type Transcriber interface {
	// Transcribe returns the words spoken in the WAV file at wavPath, and
	// removes the file: recordings are temporary.
	Transcribe(ctx context.Context, wavPath string) (string, error)
}

// Options change how one text is spoken.
type Options struct {
	Slow  bool   // speak more slowly, for dictation and hard words
	Voice string // a voice name or model path; empty is the configured voice
}

// ErrUnavailable means audio is not set up. Errors that match it with
// errors.Is are *UnavailableError values with a setup hint.
var ErrUnavailable = errors.New("audio unavailable")

// UnavailableError says what is missing and how to set it up.
type UnavailableError struct {
	Reason string // what is missing, like "piper not found"
	Hint   string // how to set it up, like "run `teacher setup voice`"
}

// Unavailable returns an error that matches ErrUnavailable.
func Unavailable(reason, hint string) error {
	return &UnavailableError{Reason: reason, Hint: hint}
}

// Error is the reason followed by the hint.
func (e *UnavailableError) Error() string { return e.Reason + ": " + e.Hint }

// Is makes errors.Is(err, ErrUnavailable) true.
func (e *UnavailableError) Is(target error) bool { return target == ErrUnavailable }
