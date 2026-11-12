package main

import (
	"bytes"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// isolate points the data and config folders and HOME at a temp folder,
// so no test touches the learner's real files.
func isolate(t *testing.T) (data, config string) {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	t.Setenv("XDG_DATA_HOME", filepath.Join(dir, "data"))
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(dir, "config"))
	return filepath.Join(dir, "data"), filepath.Join(dir, "config", "english-teacher", "config.json")
}

// fakeVoiceServer serves the two voice files and counts the requests.
func fakeVoiceServer(t *testing.T, status int) *int {
	t.Helper()
	requests := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		if status != http.StatusOK {
			http.Error(w, "nope", status)
			return
		}
		switch r.URL.Path {
		case "/en/en_US/lessac/medium/en_US-lessac-medium.onnx":
			w.Write([]byte("ONNX"))
		case "/en/en_US/lessac/medium/en_US-lessac-medium.onnx.json":
			w.Write([]byte(`{"audio":{}}`))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)
	orig := voiceBaseURL
	voiceBaseURL = srv.URL
	t.Cleanup(func() { voiceBaseURL = orig })
	return &requests
}

func TestSetupVoiceDownloadsThenIsIdempotent(t *testing.T) {
	data, _ := isolate(t)
	requests := fakeVoiceServer(t, http.StatusOK)
	voices := filepath.Join(data, "english-teacher", "voices")
	model := filepath.Join(voices, "en_US-lessac-medium.onnx")

	var out bytes.Buffer
	if err := run([]string{"setup", "voice"}, &out); err != nil {
		t.Fatal(err)
	}

	want := "Downloading en_US-lessac-medium.onnx\nDownloading en_US-lessac-medium.onnx.json\nVoice saved: " + model + "\n"
	if out.String() != want {
		t.Errorf("stdout = %q, want %q", out.String(), want)
	}
	if got, _ := os.ReadFile(model); string(got) != "ONNX" {
		t.Errorf("model = %q", got)
	}
	if got, _ := os.ReadFile(model + ".json"); string(got) != `{"audio":{}}` {
		t.Errorf("model config = %q", got)
	}
	if entries, _ := os.ReadDir(voices); len(entries) != 2 {
		t.Errorf("voices folder has %d files, want 2 (no temp files)", len(entries))
	}

	out.Reset()
	if err := run([]string{"setup", "voice"}, &out); err != nil {
		t.Fatal(err)
	}
	if want := "Voice already installed: " + model + "\n"; out.String() != want {
		t.Errorf("second run stdout = %q, want %q", out.String(), want)
	}
	if *requests != 2 {
		t.Errorf("%d requests, want 2: the second run must not download", *requests)
	}
}

func TestSetupVoiceDownloadsOnlyWhatIsMissing(t *testing.T) {
	data, _ := isolate(t)
	requests := fakeVoiceServer(t, http.StatusOK)
	model := filepath.Join(data, "english-teacher", "voices", "en_US-lessac-medium.onnx")
	os.MkdirAll(filepath.Dir(model), 0o755)
	os.WriteFile(model, []byte("OLD"), 0o644)

	var out bytes.Buffer
	if err := run([]string{"setup", "voice"}, &out); err != nil {
		t.Fatal(err)
	}

	if want := "Downloading en_US-lessac-medium.onnx.json\nVoice saved: " + model + "\n"; out.String() != want {
		t.Errorf("stdout = %q, want %q", out.String(), want)
	}
	if got, _ := os.ReadFile(model); string(got) != "OLD" || *requests != 1 {
		t.Errorf("model = %q after %d requests; an existing file is kept", got, *requests)
	}
}

func TestSetupVoiceFailureLeavesNoFiles(t *testing.T) {
	data, _ := isolate(t)
	fakeVoiceServer(t, http.StatusNotFound)

	err := run([]string{"setup", "voice"}, &bytes.Buffer{})

	if err == nil || !strings.Contains(err.Error(), "404") {
		t.Errorf("err = %v, want the HTTP status", err)
	}
	if entries, _ := os.ReadDir(filepath.Join(data, "english-teacher", "voices")); len(entries) != 0 {
		t.Errorf("files left after a failed download: %v", entries)
	}
}

func TestSetupNeedsAKnownThing(t *testing.T) {
	isolate(t)
	for _, args := range [][]string{{"setup"}, {"setup", "music"}, {"setup", "voice", "x"}, {"setup", "whisper", "x"}, {"setup", "ai", "x"}} {
		if err := run(args, &bytes.Buffer{}); err == nil || !strings.Contains(err.Error(), "usage") {
			t.Errorf("%v: err = %v, want usage", args, err)
		}
	}
}

// fakePath replaces lookPath for one test with a PATH holding only found,
// and empties the Homebrew folders searched after PATH.
func fakePath(t *testing.T, found map[string]string) {
	t.Helper()
	origBrew := brewDirs
	brewDirs = []string{}
	t.Cleanup(func() { brewDirs = origBrew })
	orig := lookPath
	lookPath = func(file string) (string, error) {
		if p, ok := found[file]; ok {
			return p, nil
		}
		return "", errors.New("not found")
	}
	t.Cleanup(func() { lookPath = orig })
}

func TestDoctorWithEverythingInstalled(t *testing.T) {
	data, _ := isolate(t)
	model := filepath.Join(data, "english-teacher", "voices", "en_US-lessac-medium.onnx")
	os.MkdirAll(filepath.Dir(model), 0o755)
	os.WriteFile(model, nil, 0o644)
	whisperModel := filepath.Join(data, "english-teacher", "models", "ggml-base.en.bin")
	os.MkdirAll(filepath.Dir(whisperModel), 0o755)
	os.WriteFile(whisperModel, nil, 0o644)
	fakePath(t, map[string]string{"pi": "/bin/pi", "piper": "/bin/piper", "paplay": "/bin/paplay",
		"whisper-cli": "/bin/whisper-cli", "arecord": "/bin/arecord"})

	var out bytes.Buffer
	if err := run([]string{"doctor"}, &out); err != nil {
		t.Fatal(err)
	}

	want := "" +
		"✓ pi (AI)                    /bin/pi\n" +
		"✓ piper (speech)             /bin/piper\n" +
		"✓ voice                      " + model + "\n" +
		"✓ audio player               /bin/paplay\n" +
		"✓ whisper-cli (listening)    /bin/whisper-cli\n" +
		"✓ whisper model              " + whisperModel + "\n" +
		"✓ recorder (microphone)      /bin/arecord\n"
	if out.String() != want {
		t.Errorf("stdout:\n%s\nwant:\n%s", out.String(), want)
	}
}

func TestDoctorWithNothingInstalledStillSucceeds(t *testing.T) {
	data, _ := isolate(t)
	model := filepath.Join(data, "english-teacher", "voices", "en_US-lessac-medium.onnx")
	whisperModel := filepath.Join(data, "english-teacher", "models", "ggml-base.en.bin")
	fakePath(t, map[string]string{})

	var out bytes.Buffer
	if err := run([]string{"doctor"}, &out); err != nil {
		t.Fatalf("doctor must succeed when things are missing: %v", err)
	}

	want := "" +
		"✗ pi (AI)                    not found (pi): install Gentle Shell or set \"pi\" in the config\n" +
		"✗ piper (speech)             piper not found: install it with `uv tool install piper-tts`, then run `teacher setup voice`\n" +
		"✗ voice                      voice not found: " + model + ": run `teacher setup voice`\n" +
		"✗ audio player               no audio player found (pw-play, paplay, aplay, afplay): install pw-play (pipewire), paplay (pulseaudio) or aplay (alsa-utils)\n" +
		"✗ whisper-cli (listening)    whisper-cli not found: install whisper.cpp with `brew install whisper-cpp` (or see the whisper.cpp README), then run `teacher setup whisper`\n" +
		"✗ whisper model              whisper model not found: " + whisperModel + ": run `teacher setup whisper`\n" +
		"✗ recorder (microphone)      no recorder found (pw-record, arecord, sox): install pw-record (pipewire), arecord (alsa-utils) or sox\n"
	if out.String() != want {
		t.Errorf("stdout:\n%s\nwant:\n%s", out.String(), want)
	}
}

func TestDoctorUsesTheConfig(t *testing.T) {
	_, cfg := isolate(t)
	dir := t.TempDir()
	piper, voice := filepath.Join(dir, "piper"), filepath.Join(dir, "my.onnx")
	os.WriteFile(piper, nil, 0o755)
	os.WriteFile(voice, nil, 0o644)
	os.MkdirAll(filepath.Dir(cfg), 0o755)
	os.WriteFile(cfg, []byte(`{"pi":"/opt/pi","piper":"`+piper+`","voice":"`+voice+`"}`), 0o644)
	fakePath(t, map[string]string{"pi": "/bin/pi", "aplay": "/bin/aplay"})

	var out bytes.Buffer
	run([]string{"doctor"}, &out)

	for _, want := range []string{
		"✗ pi (AI)                    not found (/opt/pi)",
		"✓ piper (speech)             " + piper + "\n",
		"✓ voice                      " + voice + "\n",
		"✓ audio player               /bin/aplay\n",
	} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("stdout lacks %q:\n%s", want, out.String())
		}
	}
}

func TestDoctorFindsPiperInLocalBin(t *testing.T) {
	isolate(t)
	local := filepath.Join(os.Getenv("HOME"), ".local", "bin", "piper")
	os.MkdirAll(filepath.Dir(local), 0o755)
	os.WriteFile(local, nil, 0o755)
	fakePath(t, map[string]string{})

	var out bytes.Buffer
	run([]string{"doctor"}, &out)

	if want := "✓ piper (speech)             " + local + "\n"; !strings.Contains(out.String(), want) {
		t.Errorf("stdout lacks %q:\n%s", want, out.String())
	}
}

func TestConfigReadsPiperAndVoice(t *testing.T) {
	cfg := filepath.Join(t.TempDir(), "config.json")
	os.WriteFile(cfg, []byte(`{"piper":"/opt/piper","voice":"en_GB-alan-medium"}`), 0o644)

	c, err := loadConfig(cfg)

	if err != nil || c.Piper != "/opt/piper" || c.Voice != "en_GB-alan-medium" || c.Pi != "pi" {
		t.Errorf("config = %+v, %v", c, err)
	}
}

func TestSpeakerIsNilWithoutAudio(t *testing.T) {
	data, _ := isolate(t)
	fakePath(t, map[string]string{"piper": "/bin/piper"}) // no player
	if s := defaultConfig().speaker(); s != nil {
		t.Errorf("speaker = %v, want nil without a player", s)
	}

	model := filepath.Join(data, "english-teacher", "voices", "en_US-lessac-medium.onnx")
	os.MkdirAll(filepath.Dir(model), 0o755)
	os.WriteFile(model, nil, 0o644)
	fakePath(t, map[string]string{"piper": "/bin/piper", "pw-play": "/bin/pw-play"})
	if s := defaultConfig().speaker(); s == nil {
		t.Error("speaker = nil with piper, a voice and a player")
	}
}

func TestInstalledVoicesListsTheModelsInTheFolder(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{"en_US-lessac-medium.onnx", "en_US-lessac-medium.onnx.json", "en_US-amy-medium.onnx", "notes.txt"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if got := installedVoices(dir); strings.Join(got, ",") != "en_US-amy-medium,en_US-lessac-medium" {
		t.Errorf("installedVoices = %q", got)
	}
	if got := installedVoices(filepath.Join(dir, "missing")); len(got) != 0 {
		t.Errorf("a missing folder has voices %q", got)
	}
}

// fakeModelServer serves the whisper model and counts the requests.
func fakeModelServer(t *testing.T, status int) *int {
	t.Helper()
	requests := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		if status != http.StatusOK {
			http.Error(w, "nope", status)
			return
		}
		if r.URL.Path != "/ggml-base.en.bin" {
			http.NotFound(w, r)
			return
		}
		w.Write([]byte("GGML"))
	}))
	t.Cleanup(srv.Close)
	orig := whisperBaseURL
	whisperBaseURL = srv.URL
	t.Cleanup(func() { whisperBaseURL = orig })
	return &requests
}

func TestSetupWhisperDownloadsThenIsIdempotent(t *testing.T) {
	data, _ := isolate(t)
	requests := fakeModelServer(t, http.StatusOK)
	models := filepath.Join(data, "english-teacher", "models")
	model := filepath.Join(models, "ggml-base.en.bin")

	var out bytes.Buffer
	if err := run([]string{"setup", "whisper"}, &out); err != nil {
		t.Fatal(err)
	}

	if want := "Downloading ggml-base.en.bin (about 150 MB)\nWhisper model saved: " + model + "\n"; out.String() != want {
		t.Errorf("stdout = %q, want %q", out.String(), want)
	}
	if got, _ := os.ReadFile(model); string(got) != "GGML" {
		t.Errorf("model = %q", got)
	}
	if entries, _ := os.ReadDir(models); len(entries) != 1 {
		t.Errorf("models folder has %d files, want 1 (no temp files)", len(entries))
	}

	out.Reset()
	if err := run([]string{"setup", "whisper"}, &out); err != nil {
		t.Fatal(err)
	}
	if want := "Whisper model already installed: " + model + "\n"; out.String() != want {
		t.Errorf("second run stdout = %q, want %q", out.String(), want)
	}
	if *requests != 1 {
		t.Errorf("%d requests, want 1: the second run must not download", *requests)
	}
}

func TestSetupWhisperFailureLeavesNoFiles(t *testing.T) {
	data, _ := isolate(t)
	fakeModelServer(t, http.StatusNotFound)

	err := run([]string{"setup", "whisper"}, &bytes.Buffer{})

	if err == nil || !strings.Contains(err.Error(), "404") {
		t.Errorf("err = %v, want the HTTP status", err)
	}
	if entries, _ := os.ReadDir(filepath.Join(data, "english-teacher", "models")); len(entries) != 0 {
		t.Errorf("files left after a failed download: %v", entries)
	}
}

func TestDoctorFindsWhisperInTheBrewFolder(t *testing.T) {
	isolate(t)
	fakePath(t, map[string]string{})
	brew := t.TempDir()
	bin := filepath.Join(brew, "whisper-cli")
	os.WriteFile(bin, nil, 0o755)
	brewDirs = []string{brew}

	var out bytes.Buffer
	run([]string{"doctor"}, &out)

	if want := "✓ whisper-cli (listening)    " + bin + "\n"; !strings.Contains(out.String(), want) {
		t.Errorf("stdout lacks %q:\n%s", want, out.String())
	}
}

func TestRecorderAndTranscriberAreNilWithoutWhisper(t *testing.T) {
	data, _ := isolate(t)
	fakePath(t, map[string]string{"whisper-cli": "/bin/whisper-cli"}) // no model, no recorder
	c := defaultConfig()
	if r := c.recorder(); r != nil {
		t.Errorf("recorder = %v, want nil without a recorder program", r)
	}
	if tr := c.transcriber(); tr != nil {
		t.Errorf("transcriber = %v, want nil without the model", tr)
	}

	model := filepath.Join(data, "english-teacher", "models", "ggml-base.en.bin")
	os.MkdirAll(filepath.Dir(model), 0o755)
	os.WriteFile(model, nil, 0o644)
	fakePath(t, map[string]string{"whisper-cli": "/bin/whisper-cli", "pw-record": "/bin/pw-record"})
	if c.recorder() == nil || c.transcriber() == nil {
		t.Error("recorder or transcriber = nil with whisper-cli, the model and pw-record")
	}
}
