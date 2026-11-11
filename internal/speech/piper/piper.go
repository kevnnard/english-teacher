// Package piper speaks through Piper, a local text-to-speech program the
// learner installs (`uv tool install piper-tts`). Piper writes a WAV file
// that the first audio player found on PATH plays; the file is removed
// afterwards. Nothing leaves the computer.
package piper

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/kevnnard/english-teacher/internal/speech"
)

// DefaultVoice is the voice `teacher setup voice` downloads.
const DefaultVoice = "en_US-lessac-medium"

// SlowLengthScale is piper's --length-scale for slow speech: each
// phoneme lasts 1.4 times as long.
const SlowLengthScale = "1.4"

// DefaultTimeout is how long synthesizing and playing one text may take.
const DefaultTimeout = 60 * time.Second

// Players are the audio players tried in order: PipeWire, PulseAudio,
// ALSA and macOS.
var Players = []string{"pw-play", "paplay", "aplay", "afplay"}

// Hints say how to set up each missing piece.
const (
	PiperHint  = "install it with `uv tool install piper-tts`, then run `teacher setup voice`"
	VoiceHint  = "run `teacher setup voice`"
	PlayerHint = "install pw-play (pipewire), paplay (pulseaudio) or aplay (alsa-utils)"
)

// Setup is where to find piper, a voice and a player.
type Setup struct {
	Piper     string                       // config "piper": the binary; empty looks on PATH, then ~/.local/bin
	Voice     string                       // config "voice": a model path or a voice name; empty is DefaultVoice
	VoicesDir string                       // where voice names are looked up
	Home      string                       // the home folder, for ~/.local/bin/piper
	LookPath  func(string) (string, error) // exec.LookPath when nil
}

func (s Setup) lookPath(file string) (string, error) {
	if s.LookPath == nil {
		return exec.LookPath(file)
	}
	return s.LookPath(file)
}

// FindPiper returns the piper binary: the configured one, else piper on
// PATH, else ~/.local/bin/piper, where uv installs tools.
func (s Setup) FindPiper() (string, error) {
	if s.Piper != "" {
		if strings.ContainsRune(s.Piper, os.PathSeparator) {
			if isFile(s.Piper) {
				return s.Piper, nil
			}
		} else if p, err := s.lookPath(s.Piper); err == nil {
			return p, nil
		}
		return "", speech.Unavailable("piper not found ("+s.Piper+")", PiperHint)
	}
	if p, err := s.lookPath("piper"); err == nil {
		return p, nil
	}
	if local := filepath.Join(s.Home, ".local", "bin", "piper"); s.Home != "" && isFile(local) {
		return local, nil
	}
	return "", speech.Unavailable("piper not found", PiperHint)
}

// FindVoice returns the model file of voice, a path or a name looked up
// in VoicesDir; the empty voice is the configured one.
func (s Setup) FindVoice(voice string) (string, error) {
	if voice == "" {
		voice = s.Voice
	}
	path := VoicePath(s.VoicesDir, voice)
	if !isFile(path) {
		return "", speech.Unavailable("voice not found: "+path, VoiceHint)
	}
	return path, nil
}

// FindPlayer returns the first of Players on PATH.
func (s Setup) FindPlayer() (string, error) {
	for _, name := range Players {
		if p, err := s.lookPath(name); err == nil {
			return p, nil
		}
	}
	return "", speech.Unavailable("no audio player found ("+strings.Join(Players, ", ")+")", PlayerHint)
}

// VoicePath is the model file of voice: voice itself when it is a path,
// else <dir>/<voice>.onnx. The empty voice is DefaultVoice.
func VoicePath(dir, voice string) string {
	if voice == "" {
		voice = DefaultVoice
	}
	if strings.ContainsRune(voice, os.PathSeparator) {
		return voice
	}
	return filepath.Join(dir, strings.TrimSuffix(voice, ".onnx")+".onnx")
}

// Runner runs one program with stdin as its input, like execRun.
type Runner func(ctx context.Context, name string, args []string, stdin string) error

// Speaker speaks with piper. It implements speech.Speaker.
type Speaker struct {
	Piper   string        // the piper binary
	Voice   string        // the default voice model file
	Player  string        // the audio player
	Setup   Setup         // to find voices passed in speech.Options
	Run     Runner        // execRun when nil; tests use a fake
	TempDir string        // where the WAV is written; os.TempDir() when empty
	Timeout time.Duration // DefaultTimeout when zero
}

// New finds piper, the voice and a player. When one is missing it
// returns an error matching speech.ErrUnavailable with a setup hint.
func New(s Setup) (*Speaker, error) {
	bin, err := s.FindPiper()
	if err != nil {
		return nil, err
	}
	voice, err := s.FindVoice("")
	if err != nil {
		return nil, err
	}
	player, err := s.FindPlayer()
	if err != nil {
		return nil, err
	}
	return &Speaker{Piper: bin, Voice: voice, Player: player, Setup: s}, nil
}

// Say synthesizes text to a temporary WAV, plays it and removes it.
func (s *Speaker) Say(ctx context.Context, text string, opts speech.Options) error {
	text = strings.TrimSpace(text)
	if text == "" {
		return nil
	}
	voice := s.Voice
	if opts.Voice != "" {
		v, err := s.Setup.FindVoice(opts.Voice)
		if err != nil {
			return err
		}
		voice = v
	}
	run, timeout := s.Run, s.Timeout
	if run == nil {
		run = execRun
	}
	if timeout == 0 {
		timeout = DefaultTimeout
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	f, err := os.CreateTemp(s.TempDir, "teacher-*.wav")
	if err != nil {
		return err
	}
	wav := f.Name()
	f.Close()
	defer os.Remove(wav)

	args := []string{"-m", voice, "-f", wav}
	if opts.Slow {
		args = append(args, "--length-scale", SlowLengthScale)
	}
	if err := run(ctx, s.Piper, args, text); err != nil {
		return explain(err, PiperHint)
	}
	if err := run(ctx, s.Player, []string{wav}, ""); err != nil {
		return explain(err, PlayerHint)
	}
	return nil
}

// explain turns a missing program into speech.ErrUnavailable.
func explain(err error, hint string) error {
	if errors.Is(err, os.ErrNotExist) {
		return speech.Unavailable(err.Error(), hint+"; see `teacher doctor`")
	}
	return err
}

// execRun runs name with stdin as its input. A missing program is an
// os.ErrNotExist error; a failure carries the last line of stderr, where
// piper prints the real error after its warnings.
func execRun(ctx context.Context, name string, args []string, stdin string) error {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.WaitDelay = time.Second // do not wait for children that keep the pipes open
	cmd.Stdin = strings.NewReader(stdin)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	err := cmd.Run()
	switch {
	case errors.Is(ctx.Err(), context.DeadlineExceeded):
		return fmt.Errorf("%s timed out", filepath.Base(name))
	case errors.Is(err, exec.ErrNotFound):
		return fmt.Errorf("%s: %w", name, os.ErrNotExist)
	case errors.Is(err, os.ErrNotExist):
		return err
	case err != nil:
		return fmt.Errorf("%s: %v: %s", filepath.Base(name), err, lastLine(stderr.String()))
	}
	return nil
}

func isFile(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}

// lastLine is the last non-empty line of s.
func lastLine(s string) string {
	lines := strings.Split(strings.TrimSpace(s), "\n")
	return strings.TrimSpace(lines[len(lines)-1])
}
