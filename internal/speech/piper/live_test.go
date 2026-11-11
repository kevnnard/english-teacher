package piper

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

// TestLiveSynthesis runs the real piper with the installed voice and
// checks it wrote a WAV. It never plays the file. It only runs with
// TEACHER_LIVE=1, so the normal test suite needs no audio setup.
func TestLiveSynthesis(t *testing.T) {
	if os.Getenv("TEACHER_LIVE") != "1" {
		t.Skip("set TEACHER_LIVE=1 to run the real piper")
	}
	home, _ := os.UserHomeDir()
	data := os.Getenv("XDG_DATA_HOME")
	if data == "" {
		data = filepath.Join(home, ".local", "share")
	}
	setup := Setup{VoicesDir: filepath.Join(data, "english-teacher", "voices"), Home: home}
	bin, err := setup.FindPiper()
	if err != nil {
		t.Fatal(err)
	}
	voice, err := setup.FindVoice("")
	if err != nil {
		t.Fatal(err)
	}

	wav := filepath.Join(t.TempDir(), "out.wav")
	if err := execRun(context.Background(), bin, []string{"-m", voice, "-f", wav}, "I am thirty years old."); err != nil {
		t.Fatal(err)
	}
	wavData, err := os.ReadFile(wav)
	if err != nil || len(wavData) < 1000 || string(wavData[:4]) != "RIFF" {
		t.Errorf("no WAV written: %d bytes, %v", len(wavData), err)
	}
}
