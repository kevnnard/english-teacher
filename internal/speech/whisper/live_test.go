package whisper

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kevnnard/english-teacher/internal/check"
	"github.com/kevnnard/english-teacher/internal/speech/piper"
)

// TestLiveTranscription speaks a sentence with the real piper, converts
// it to 16 kHz mono with ffmpeg and transcribes it with the real
// whisper-cli. It never plays or records anything. It only runs with
// TEACHER_LIVE=1, so the normal test suite needs no audio setup.
func TestLiveTranscription(t *testing.T) {
	if os.Getenv("TEACHER_LIVE") != "1" {
		t.Skip("set TEACHER_LIVE=1 to run the real piper, ffmpeg and whisper-cli")
	}
	home, _ := os.UserHomeDir()
	data := os.Getenv("XDG_DATA_HOME")
	if data == "" {
		data = filepath.Join(home, ".local", "share")
	}
	app := filepath.Join(data, "english-teacher")
	ps := piper.Setup{VoicesDir: filepath.Join(app, "voices"), Home: home}
	piperBin, err := ps.FindPiper()
	if err != nil {
		t.Fatal(err)
	}
	voice, err := ps.FindVoice("")
	if err != nil {
		t.Fatal(err)
	}
	tr, err := NewTranscriber(Setup{ModelsDir: filepath.Join(app, "models"), Home: home})
	if err != nil {
		t.Fatal(err)
	}
	ffmpeg, err := exec.LookPath("ffmpeg")
	if err != nil {
		t.Fatal(err)
	}

	const sentence = "My name is Anna. I am thirty years old."
	dir := t.TempDir()
	raw, wav := filepath.Join(dir, "raw.wav"), filepath.Join(dir, "16k.wav")
	synth := exec.Command(piperBin, "-m", voice, "-f", raw)
	synth.Stdin = strings.NewReader(sentence)
	if out, err := synth.CombinedOutput(); err != nil {
		t.Fatalf("piper: %v: %s", err, out)
	}
	if out, err := exec.Command(ffmpeg, "-loglevel", "error", "-i", raw, "-ar", "16000", "-ac", "1", wav).CombinedOutput(); err != nil {
		t.Fatalf("ffmpeg: %v: %s", err, out)
	}

	text, err := tr.Transcribe(context.Background(), wav)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("whisper heard %q", text)
	for _, m := range check.Understood(sentence, text) {
		if !m.Understood && m.Word != "Anna." { // whisper may spell the name its own way
			t.Errorf("%q not understood in %q", m.Word, text)
		}
	}
	if _, err := os.Stat(wav); !os.IsNotExist(err) {
		t.Errorf("the WAV was not removed: %v", err)
	}
}
