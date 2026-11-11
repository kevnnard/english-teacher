package piper

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/kevnnard/english-teacher/internal/speech"
)

// call is one command the fake runner was asked to run.
type call struct {
	name  string
	args  []string
	stdin string
}

// fakeRunner records every command. For piper it writes the WAV file
// named by -f, the way the real piper does.
type fakeRunner struct {
	calls   []call
	wavSeen bool  // the WAV existed when the player ran
	fail    error // returned by every call when set
}

func (f *fakeRunner) run(_ context.Context, name string, args []string, stdin string) error {
	f.calls = append(f.calls, call{name, args, stdin})
	if f.fail != nil {
		return f.fail
	}
	if i := slices.Index(args, "-f"); i >= 0 {
		return os.WriteFile(args[i+1], []byte("RIFF"), 0o644)
	}
	if len(args) > 0 {
		_, err := os.Stat(args[len(args)-1])
		f.wavSeen = err == nil
	}
	return nil
}

// touch creates an empty file at path, with its parent directories.
func touch(t *testing.T, path string) string {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, nil, 0o755); err != nil {
		t.Fatal(err)
	}
	return path
}

// onPath is a fake exec.LookPath that finds only the given programs.
func onPath(found map[string]string) func(string) (string, error) {
	return func(file string) (string, error) {
		if p, ok := found[file]; ok {
			return p, nil
		}
		return "", errors.New("not found")
	}
}

// installed is a Setup where piper, the default voice and pw-play exist.
func installed(t *testing.T) Setup {
	dir := t.TempDir()
	voices := filepath.Join(dir, "voices")
	touch(t, filepath.Join(voices, DefaultVoice+".onnx"))
	return Setup{
		VoicesDir: voices,
		Home:      dir,
		LookPath:  onPath(map[string]string{"piper": "/bin/piper", "pw-play": "/bin/pw-play"}),
	}
}

func TestSaySynthesizesPlaysAndRemovesTheWAV(t *testing.T) {
	s, err := New(installed(t))
	if err != nil {
		t.Fatal(err)
	}
	f := &fakeRunner{}
	s.Run = f.run

	if err := s.Say(context.Background(), "I am 30 years old.", speech.Options{}); err != nil {
		t.Fatal(err)
	}

	if len(f.calls) != 2 {
		t.Fatalf("calls = %+v, want piper then the player", f.calls)
	}
	synth, play := f.calls[0], f.calls[1]
	wav := synth.args[3]
	wantSynth := []string{"-m", s.Voice, "-f", wav}
	if synth.name != "/bin/piper" || !slices.Equal(synth.args, wantSynth) || synth.stdin != "I am 30 years old." {
		t.Errorf("piper call = %+v, want args %q", synth, wantSynth)
	}
	if !strings.HasSuffix(wav, ".wav") {
		t.Errorf("output %q is not a .wav", wav)
	}
	if play.name != "/bin/pw-play" || !slices.Equal(play.args, []string{wav}) || !f.wavSeen {
		t.Errorf("player call = %+v (wav existed: %v)", play, f.wavSeen)
	}
	if _, err := os.Stat(wav); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("the temp WAV %s was not removed: %v", wav, err)
	}
}

func TestSaySlowStretchesThePhonemes(t *testing.T) {
	s, _ := New(installed(t))
	f := &fakeRunner{}
	s.Run = f.run

	s.Say(context.Background(), "ship", speech.Options{Slow: true})

	args := f.calls[0].args
	if i := slices.Index(args, "--length-scale"); i < 0 || args[i+1] != SlowLengthScale {
		t.Errorf("piper args = %q, want --length-scale %s", args, SlowLengthScale)
	}
}

func TestSayWithAnotherVoice(t *testing.T) {
	setup := installed(t)
	other := touch(t, filepath.Join(setup.VoicesDir, "en_GB-alan-medium.onnx"))
	s, _ := New(setup)
	f := &fakeRunner{}
	s.Run = f.run

	if err := s.Say(context.Background(), "sheep", speech.Options{Voice: "en_GB-alan-medium"}); err != nil {
		t.Fatal(err)
	}
	if got := f.calls[0].args[1]; got != other {
		t.Errorf("model = %q, want %q", got, other)
	}

	f.calls = nil
	err := s.Say(context.Background(), "sheep", speech.Options{Voice: "en_US-ryan-high"})
	if !errors.Is(err, speech.ErrUnavailable) || !strings.Contains(err.Error(), "en_US-ryan-high") || len(f.calls) != 0 {
		t.Errorf("missing voice: err = %v, calls = %+v", err, f.calls)
	}
}

func TestSayEmptyTextDoesNothing(t *testing.T) {
	s, _ := New(installed(t))
	f := &fakeRunner{}
	s.Run = f.run

	if err := s.Say(context.Background(), "  \n", speech.Options{}); err != nil || len(f.calls) != 0 {
		t.Errorf("err = %v, calls = %+v", err, f.calls)
	}
}

func TestSayRemovesTheWAVWhenPiperFails(t *testing.T) {
	s, _ := New(installed(t))
	tmp := t.TempDir()
	s.TempDir = tmp
	s.Run = (&fakeRunner{fail: errors.New("piper: boom")}).run

	err := s.Say(context.Background(), "hello", speech.Options{})

	if err == nil || !strings.Contains(err.Error(), "boom") {
		t.Errorf("err = %v", err)
	}
	if left, _ := os.ReadDir(tmp); len(left) != 0 {
		t.Errorf("temp files left behind: %v", left)
	}
}

func TestSayMissingProgramIsUnavailable(t *testing.T) {
	s, _ := New(installed(t))
	s.Run = (&fakeRunner{fail: os.ErrNotExist}).run

	err := s.Say(context.Background(), "hello", speech.Options{})

	if !errors.Is(err, speech.ErrUnavailable) || !strings.Contains(err.Error(), "teacher doctor") {
		t.Errorf("err = %v, want ErrUnavailable with a hint", err)
	}
}

func TestNewFindsPiper(t *testing.T) {
	t.Run("config path", func(t *testing.T) {
		setup := installed(t)
		setup.Piper = touch(t, filepath.Join(t.TempDir(), "my-piper"))
		s, err := New(setup)
		if err != nil || s.Piper != setup.Piper {
			t.Errorf("piper = %v, %v; want %s", s, err, setup.Piper)
		}
	})
	t.Run("PATH", func(t *testing.T) {
		s, err := New(installed(t))
		if err != nil || s.Piper != "/bin/piper" {
			t.Errorf("piper = %v, %v", s, err)
		}
	})
	t.Run("~/.local/bin", func(t *testing.T) {
		setup := installed(t)
		setup.LookPath = onPath(map[string]string{"aplay": "/bin/aplay"})
		local := touch(t, filepath.Join(setup.Home, ".local", "bin", "piper"))
		s, err := New(setup)
		if err != nil || s.Piper != local {
			t.Errorf("piper = %v, %v; want %s", s, err, local)
		}
	})
}

func TestNewFindsTheFirstPlayer(t *testing.T) {
	setup := installed(t)
	setup.LookPath = onPath(map[string]string{"piper": "/bin/piper", "aplay": "/bin/aplay", "paplay": "/bin/paplay", "afplay": "/bin/afplay"})

	s, err := New(setup)

	if err != nil || s.Player != "/bin/paplay" {
		t.Errorf("player = %v, %v; want paplay, the first one on PATH", s, err)
	}
}

func TestNewVoiceFromConfig(t *testing.T) {
	setup := installed(t)
	path := touch(t, filepath.Join(t.TempDir(), "custom.onnx"))
	for voice, want := range map[string]string{
		path:                   path,
		"":                     filepath.Join(setup.VoicesDir, DefaultVoice+".onnx"),
		DefaultVoice:           filepath.Join(setup.VoicesDir, DefaultVoice+".onnx"),
		DefaultVoice + ".onnx": filepath.Join(setup.VoicesDir, DefaultVoice+".onnx"),
	} {
		setup.Voice = voice
		s, err := New(setup)
		if err != nil || s.Voice != want {
			t.Errorf("voice %q: got %v, %v; want %s", voice, s, err, want)
		}
	}
}

func TestNewSaysHowToSetUpWhatIsMissing(t *testing.T) {
	tests := []struct {
		name   string
		breaks func(*Setup)
		want   []string
	}{
		{"no piper", func(s *Setup) { s.LookPath = onPath(map[string]string{"pw-play": "/bin/pw-play"}) },
			[]string{"piper not found", "uv tool install piper-tts", "teacher setup voice"}},
		{"configured piper missing", func(s *Setup) { s.Piper = "/nowhere/piper" },
			[]string{"/nowhere/piper", "uv tool install piper-tts"}},
		{"no voice", func(s *Setup) { s.VoicesDir = t.TempDir() },
			[]string{"voice not found", DefaultVoice, "teacher setup voice"}},
		{"no player", func(s *Setup) { s.LookPath = onPath(map[string]string{"piper": "/bin/piper"}) },
			[]string{"no audio player", "pw-play", "aplay"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			setup := installed(t)
			tt.breaks(&setup)

			s, err := New(setup)

			if s != nil || !errors.Is(err, speech.ErrUnavailable) {
				t.Fatalf("New = %v, %v; want ErrUnavailable", s, err)
			}
			for _, w := range tt.want {
				if !strings.Contains(err.Error(), w) {
					t.Errorf("err %q lacks %q", err, w)
				}
			}
		})
	}
}

func TestExecRunReportsStderrAndFeedsStdin(t *testing.T) {
	dir := t.TempDir()
	out := filepath.Join(dir, "stdin.txt")
	ok := touch(t, filepath.Join(dir, "ok"))
	os.WriteFile(ok, []byte("#!/bin/sh\ncat > \"$1\"\n"), 0o755)
	bad := touch(t, filepath.Join(dir, "bad"))
	os.WriteFile(bad, []byte("#!/bin/sh\necho warning >&2\necho 'Error: bad model' >&2\nexit 1\n"), 0o755)

	if err := execRun(context.Background(), ok, []string{out}, "hello"); err != nil {
		t.Fatal(err)
	}
	if got, _ := os.ReadFile(out); string(got) != "hello" {
		t.Errorf("stdin = %q", got)
	}
	err := execRun(context.Background(), bad, nil, "")
	if err == nil || !strings.HasSuffix(err.Error(), "Error: bad model") || strings.Contains(err.Error(), "warning") {
		t.Errorf("err = %v, want stderr's last line", err)
	}
	err = execRun(context.Background(), filepath.Join(dir, "nope"), nil, "")
	if !errors.Is(err, os.ErrNotExist) {
		t.Errorf("missing program: err = %v, want os.ErrNotExist", err)
	}
}
