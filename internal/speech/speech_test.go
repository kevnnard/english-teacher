package speech

import (
	"errors"
	"testing"
)

func TestUnavailableCarriesTheHint(t *testing.T) {
	err := Unavailable("piper not found", "run `uv tool install piper-tts`")

	if !errors.Is(err, ErrUnavailable) {
		t.Errorf("errors.Is(%v, ErrUnavailable) = false", err)
	}
	var u *UnavailableError
	if !errors.As(err, &u) || u.Hint != "run `uv tool install piper-tts`" {
		t.Fatalf("errors.As = %+v", u)
	}
	if got, want := err.Error(), "piper not found: run `uv tool install piper-tts`"; got != want {
		t.Errorf("Error() = %q, want %q", got, want)
	}
}
