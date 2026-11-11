package main

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/kevnnard/english-teacher/internal/speech"
	"github.com/kevnnard/english-teacher/internal/speech/piper"
	"github.com/kevnnard/english-teacher/internal/speech/whisper"
)

// lookPath finds a program on PATH. Tests replace it.
var lookPath = exec.LookPath

// voiceBaseURL is where the Piper voices are published. Tests replace it.
var voiceBaseURL = "https://huggingface.co/rhasspy/piper-voices/resolve/main"

// whisperBaseURL is where the whisper.cpp models are published. Tests
// replace it.
var whisperBaseURL = "https://huggingface.co/ggerganov/whisper.cpp/resolve/main"

// brewDirs are the Homebrew folders searched for whisper-cli after PATH.
// Tests replace it.
var brewDirs = whisper.BrewDirs

// defaultVoiceFolder is the voice's folder under voiceBaseURL.
const defaultVoiceFolder = "en/en_US/lessac/medium"

// voicesDir follows the XDG spec: $XDG_DATA_HOME, else ~/.local/share.
func voicesDir(xdgData, home string) string {
	return filepath.Join(filepath.Dir(defaultDBPath(xdgData, home)), "voices")
}

// modelsDir is where the whisper model goes, next to the voices.
func modelsDir(xdgData, home string) string {
	return filepath.Join(filepath.Dir(defaultDBPath(xdgData, home)), "models")
}

// whisperSetup tells the whisper adapter where to look: whisper-cli on
// PATH, else in ~/.linuxbrew/bin or brewDirs.
func whisperSetup() whisper.Setup {
	home, _ := os.UserHomeDir()
	fallback := slices.Clone(brewDirs)
	if home != "" {
		fallback = append([]string{filepath.Join(home, ".linuxbrew", "bin")}, fallback...)
	}
	return whisper.Setup{
		ModelsDir: modelsDir(os.Getenv("XDG_DATA_HOME"), home),
		Home:      home,
		Fallback:  fallback,
		LookPath:  lookPath,
	}
}

// recorder records the microphone, or is nil when no recorder program
// is found: Speak then says how to install one.
func (c config) recorder() speech.Recorder {
	r, err := whisper.NewRecorder(whisperSetup())
	if err != nil {
		return nil
	}
	return r
}

// transcriber is whisper, or nil when whisper-cli or its model is
// missing: Speak then says how to set it up.
func (c config) transcriber() speech.Transcriber {
	t, err := whisper.NewTranscriber(whisperSetup())
	if err != nil {
		return nil
	}
	return t
}

// speechSetup tells the Piper adapter where to look, from the config and
// the environment.
func (c config) speechSetup() piper.Setup {
	home, _ := os.UserHomeDir()
	return piper.Setup{
		Piper:     c.Piper,
		Voice:     c.Voice,
		VoicesDir: voicesDir(os.Getenv("XDG_DATA_HOME"), home),
		Home:      home,
		LookPath:  lookPath,
	}
}

// speaker is the Piper speaker, or nil when audio is not set up: the
// app then works without sound.
func (c config) speaker() speech.Speaker {
	s, err := piper.New(c.speechSetup())
	if err != nil {
		return nil
	}
	return s
}

// installedVoices are the names of the voice models in dir (each
// <name>.onnx), sorted; none when dir is missing. Pairs alternates them
// when there are two or more.
func installedVoices(dir string) []string {
	files, err := filepath.Glob(filepath.Join(dir, "*.onnx"))
	if err != nil {
		return nil
	}
	names := make([]string, 0, len(files))
	for _, f := range files {
		names = append(names, strings.TrimSuffix(filepath.Base(f), ".onnx"))
	}
	return names // Glob returns sorted paths
}

// voices are the installed voice names, from the voices folder.
func (c config) voices() []string {
	return installedVoices(c.speechSetup().VoicesDir)
}

// runSetup sets up one optional capability: "voice" (speech) or
// "whisper" (listening to the learner).
func runSetup(args []string, stdout io.Writer) error {
	if len(args) != 1 {
		return errors.New(usage)
	}
	home, _ := os.UserHomeDir()
	switch args[0] {
	case "voice":
		return setupVoice(voicesDir(os.Getenv("XDG_DATA_HOME"), home), stdout)
	case "whisper":
		return setupWhisper(modelsDir(os.Getenv("XDG_DATA_HOME"), home), stdout)
	}
	return errors.New(usage)
}

// setupWhisper downloads the whisper model into dir. A model already
// there is kept, so running it again downloads nothing.
func setupWhisper(dir string, stdout io.Writer) error {
	model := whisper.ModelPath(dir)
	if _, err := os.Stat(model); err == nil {
		fmt.Fprintln(stdout, "Whisper model already installed:", model)
		return nil
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	fmt.Fprintln(stdout, "Downloading", whisper.ModelFile, "(about 150 MB)")
	if err := download(whisperBaseURL+"/"+whisper.ModelFile, model); err != nil {
		return err
	}
	fmt.Fprintln(stdout, "Whisper model saved:", model)
	return nil
}

// setupVoice downloads the default voice model and its config into dir.
// Files already there are kept, so running it again downloads nothing.
func setupVoice(dir string, stdout io.Writer) error {
	model := filepath.Join(dir, piper.DefaultVoice+".onnx")
	files := []string{model, model + ".json"}
	var missing []string
	for _, f := range files {
		if _, err := os.Stat(f); err != nil {
			missing = append(missing, f)
		}
	}
	if len(missing) == 0 {
		fmt.Fprintln(stdout, "Voice already installed:", model)
		return nil
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	for _, f := range missing {
		name := filepath.Base(f)
		fmt.Fprintln(stdout, "Downloading", name)
		if err := download(voiceBaseURL+"/"+defaultVoiceFolder+"/"+name, f); err != nil {
			return err
		}
	}
	fmt.Fprintln(stdout, "Voice saved:", model)
	return nil
}

// downloadClient gives up on a download after 10 minutes, so a stalled
// connection ends with an error instead of hanging forever.
var downloadClient = &http.Client{Timeout: 10 * time.Minute}

// download saves url to path through a temp file in the same folder, so
// a failed download never leaves a half-written file at path.
func download(url, path string) error {
	resp, err := downloadClient.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("download %s: %s", url, resp.Status)
	}

	tmp, err := os.CreateTemp(filepath.Dir(path), filepath.Base(path)+".*.part")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name()) // a no-op after the rename
	if _, err := io.Copy(tmp, resp.Body); err != nil {
		tmp.Close()
		return fmt.Errorf("download %s: %w", url, err)
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), path)
}

// check is one line of `teacher doctor`.
type check struct {
	name  string
	found string // the path found; empty when missing
	err   error  // why it is missing and how to set it up
}

// runDoctor prints whether each optional program is found. Missing ones
// are not an error: the app works without them.
func runDoctor(path string, stdout io.Writer) error {
	c, err := loadConfig(path)
	if err != nil {
		return err
	}
	setup := c.speechSetup()
	piperPath, piperErr := setup.FindPiper()
	voicePath, voiceErr := setup.FindVoice("")
	playerPath, playerErr := setup.FindPlayer()
	piPath, piErr := findProgram(c.Pi, "install Gentle Shell or set \"pi\" in the config")
	ws := whisperSetup()
	whisperPath, whisperErr := ws.FindWhisper()
	modelPath, modelErr := ws.FindModel()
	recorderPath, recorderErr := ws.FindRecorder()

	for _, ch := range []check{
		{"pi (AI)", piPath, piErr},
		{"piper (speech)", piperPath, piperErr},
		{"voice", voicePath, voiceErr},
		{"audio player", playerPath, playerErr},
		{"whisper-cli (listening)", whisperPath, whisperErr},
		{"whisper model", modelPath, modelErr},
		{"recorder (microphone)", recorderPath, recorderErr},
	} {
		mark, detail := "✓", ch.found
		if ch.err != nil {
			mark, detail = "✗", ch.err.Error()
		}
		fmt.Fprintf(stdout, "%s %-26s %s\n", mark, ch.name, detail)
	}
	return nil
}

// findProgram finds name, a path or a program on PATH.
func findProgram(name, hint string) (string, error) {
	if strings.ContainsRune(name, os.PathSeparator) {
		if info, err := os.Stat(name); err == nil && !info.IsDir() {
			return name, nil
		}
	} else if p, err := lookPath(name); err == nil {
		return p, nil
	}
	return "", fmt.Errorf("not found (%s): %s", name, hint)
}
