package whisper

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

// DefaultTimeout is how long transcribing one recording may take.
const DefaultTimeout = 2 * time.Minute

// Runner runs one program and returns what it printed on stdout, like
// execRun.
type Runner func(ctx context.Context, name string, args []string) (stdout string, err error)

// Transcriber turns a recording into text with whisper-cli. It
// implements speech.Transcriber.
type Transcriber struct {
	Whisper string        // the whisper-cli binary
	Model   string        // the model file
	Run     Runner        // execRun when nil; tests use a fake
	Timeout time.Duration // DefaultTimeout when zero
}

// nonSpeech are the marks whisper writes for sounds that are not words,
// like [BLANK_AUDIO] or (coughs).
var nonSpeech = regexp.MustCompile(`\[[^\]]*\]|\([^)]*\)`)

// Transcribe runs whisper-cli on the WAV file and removes it. The text is
// whisper's lines joined with single spaces, without non-speech marks;
// silence is "".
func (t *Transcriber) Transcribe(ctx context.Context, wavPath string) (string, error) {
	defer os.Remove(wavPath)
	run, timeout := t.Run, t.Timeout
	if run == nil {
		run = execRun
	}
	if timeout == 0 {
		timeout = DefaultTimeout
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	// -nt: no timestamps; -np: print only the text.
	out, err := run(ctx, t.Whisper, []string{"-m", t.Model, "-f", wavPath, "-nt", "-np"})
	if err != nil {
		return "", explain(err, WhisperHint)
	}
	return strings.Join(strings.Fields(nonSpeech.ReplaceAllString(out, " ")), " "), nil
}

// execRun runs name and returns its stdout. A missing program is an
// os.ErrNotExist error; a failure carries the last line of stderr.
func execRun(ctx context.Context, name string, args []string) (string, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.WaitDelay = time.Second
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr

	err := cmd.Run()
	switch {
	case errors.Is(ctx.Err(), context.DeadlineExceeded):
		return "", fmt.Errorf("%s timed out", filepath.Base(name))
	case errors.Is(err, exec.ErrNotFound):
		return "", fmt.Errorf("%s: %w", name, os.ErrNotExist)
	case errors.Is(err, os.ErrNotExist):
		return "", err
	case err != nil:
		return "", fmt.Errorf("%s: %v: %s", filepath.Base(name), err, lastLine(stderr.String()))
	}
	return stdout.String(), nil
}
