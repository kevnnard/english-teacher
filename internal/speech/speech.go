// Package speech is the audio port of the app: text-to-speech behind an
// interface, so screens can speak without knowing which program does it.
// Audio is optional. When it is not set up, a Speaker returns an error
// that says how to set it up, and nothing else changes.
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
