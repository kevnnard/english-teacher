package whisper

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/kevnnard/english-teacher/internal/speech"
)

// touch creates a file at path with data, with its parent directories.
func touch(t *testing.T, path, data string) string {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(data), 0o755); err != nil {
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

// mustBeUnavailable checks err matches speech.ErrUnavailable with a hint
// holding every one of wants.
func mustBeUnavailable(t *testing.T, err error, wants ...string) {
	t.Helper()
	var u *speech.UnavailableError
	if !errors.Is(err, speech.ErrUnavailable) || !errors.As(err, &u) {
		t.Fatalf("err = %v, want speech.ErrUnavailable", err)
	}
	for _, w := range wants {
		if !strings.Contains(u.Hint, w) {
			t.Errorf("hint %q lacks %q", u.Hint, w)
		}
	}
}

func TestFindWhisperOnPathThenInTheBrewFolders(t *testing.T) {
	dir := t.TempDir()
	s := Setup{LookPath: onPath(map[string]string{"whisper-cli": "/bin/whisper-cli"}), Fallback: []string{dir}}
	if p, err := s.FindWhisper(); err != nil || p != "/bin/whisper-cli" {
		t.Errorf("on PATH: %q, %v", p, err)
	}

	brew := touch(t, filepath.Join(dir, "whisper-cli"), "")
	s.LookPath = onPath(nil)
	if p, err := s.FindWhisper(); err != nil || p != brew {
		t.Errorf("fallback: %q, %v; want %q", p, err, brew)
	}
}

func TestFindWhisperMissingSaysHowToInstall(t *testing.T) {
	s := Setup{LookPath: onPath(nil), Fallback: []string{t.TempDir()}}
	_, err := s.FindWhisper()
	mustBeUnavailable(t, err, "brew install whisper-cpp", "whisper.cpp README", "teacher setup whisper")
}

func TestDefaultFallbackHasLinuxbrewAndTheHomeBrew(t *testing.T) {
	got := Setup{Home: "/home/ana"}.fallback()
	for _, want := range []string{"/home/linuxbrew/.linuxbrew/bin", "/home/ana/.linuxbrew/bin", "/opt/homebrew/bin"} {
		if !slices.Contains(got, want) {
			t.Errorf("fallback %q lacks %q", got, want)
		}
	}
}

func TestFindModel(t *testing.T) {
	dir := t.TempDir()
	s := Setup{ModelsDir: dir}
	_, err := s.FindModel()
	mustBeUnavailable(t, err, "teacher setup whisper")

	model := touch(t, filepath.Join(dir, "ggml-base.en.bin"), "GGML")
	if p, err := s.FindModel(); err != nil || p != model || ModelPath(dir) != model {
		t.Errorf("FindModel = %q, %v; want %q", p, err, model)
	}
}

func TestFindRecorderTakesTheFirstFound(t *testing.T) {
	tests := []struct {
		found map[string]string
		want  string
	}{
		{map[string]string{"pw-record": "/bin/pw-record", "arecord": "/bin/arecord"}, "/bin/pw-record"},
		{map[string]string{"arecord": "/bin/arecord", "sox": "/bin/sox"}, "/bin/arecord"},
		{map[string]string{"sox": "/bin/sox"}, "/bin/sox"},
	}
	for _, tt := range tests {
		r, err := NewRecorder(Setup{LookPath: onPath(tt.found)})
		if err != nil || r.Program != tt.want {
			t.Errorf("found %v: recorder %+v, %v; want %s", tt.found, r, err, tt.want)
		}
	}
	_, err := NewRecorder(Setup{LookPath: onPath(nil)})
	mustBeUnavailable(t, err, "pw-record", "arecord")
}

func TestRecorderArgs(t *testing.T) {
	for name, want := range map[string][]string{
		"/bin/pw-record": {"--rate", "16000", "--channels", "1", "--format", "s16", "x.wav"},
		"/bin/arecord":   {"-q", "-f", "S16_LE", "-r", "16000", "-c", "1", "x.wav"},
		"/bin/sox":       {"-q", "-d", "-r", "16000", "-c", "1", "-b", "16", "x.wav"},
	} {
		if got := recorderArgs(name, "x.wav"); !slices.Equal(got, want) {
			t.Errorf("%s: args %q, want %q", name, got, want)
		}
	}
}

// fakeProcess is a recorder program that never touches a microphone.
// Interrupted, it writes the WAV and ends, like pw-record on SIGINT.
type fakeProcess struct {
	wav        string
	stubborn   bool   // ignores the interrupt, so only a kill ends it
	unkillable bool   // ignores the kill too: Wait never returns
	data       string // what an interrupt writes to the WAV
	mu         sync.Mutex
	events     []string
	exit       chan struct{}
	endOnce    sync.Once
	waitResult error
}

func (p *fakeProcess) log(e string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.events = append(p.events, e)
}

func (p *fakeProcess) end() { p.endOnce.Do(func() { close(p.exit) }) }

func (p *fakeProcess) Interrupt() error {
	p.log("interrupt")
	if !p.stubborn {
		os.WriteFile(p.wav, []byte(p.data), 0o644)
		p.end()
	}
	return nil
}

func (p *fakeProcess) Kill() error {
	p.log("kill")
	if !p.unkillable {
		p.end()
	}
	return nil
}

func (p *fakeProcess) Wait() error { <-p.exit; return p.waitResult }

func (p *fakeProcess) seen() []string {
	p.mu.Lock()
	defer p.mu.Unlock()
	return slices.Clone(p.events)
}

// launches records each launch and hands out a fakeProcess.
type launches struct {
	name string
	args []string
	proc *fakeProcess
	tmpl fakeProcess // copied into each process
	err  error
}

func (l *launches) launch(_ context.Context, name string, args []string) (Process, error) {
	if l.err != nil {
		return nil, l.err
	}
	l.name, l.args = name, args
	l.proc = &fakeProcess{wav: args[len(args)-1], stubborn: l.tmpl.stubborn, unkillable: l.tmpl.unkillable, data: l.tmpl.data, exit: make(chan struct{})}
	return l.proc, nil
}

// newFakeRecorder is a pw-record recorder writing into a temp folder.
func newFakeRecorder(t *testing.T, l *launches) *Recorder {
	return &Recorder{Program: "/bin/pw-record", Launch: l.launch, TempDir: t.TempDir(), StopTimeout: 20 * time.Millisecond}
}

// someAudio is more than a WAV header: something was recorded.
var someAudio = strings.Repeat("A", 200)

func TestRecordingStopInterruptsAndReturnsTheWAV(t *testing.T) {
	l := &launches{tmpl: fakeProcess{data: someAudio}}
	r := newFakeRecorder(t, l)

	rec, err := r.Start(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	wav := l.args[len(l.args)-1]
	want := []string{"--rate", "16000", "--channels", "1", "--format", "s16", wav}
	if l.name != "/bin/pw-record" || !slices.Equal(l.args, want) {
		t.Errorf("launched %s %q, want %q", l.name, l.args, want)
	}
	if filepath.Dir(wav) != r.TempDir || !strings.HasSuffix(wav, ".wav") {
		t.Errorf("wav %q is not a .wav in the temp folder", wav)
	}

	got, err := rec.Stop()
	if err != nil || got != wav {
		t.Fatalf("Stop = %q, %v; want %q", got, err, wav)
	}
	if ev := l.proc.seen(); !slices.Equal(ev, []string{"interrupt"}) {
		t.Errorf("events %q, want only an interrupt", ev)
	}
	if data, _ := os.ReadFile(wav); string(data) != someAudio {
		t.Errorf("the WAV was not kept for the transcriber")
	}
}

func TestRecordingStopKillsAStubbornRecorder(t *testing.T) {
	l := &launches{tmpl: fakeProcess{stubborn: true}}
	r := newFakeRecorder(t, l)
	rec, _ := r.Start(context.Background())

	_, err := rec.Stop()

	if ev := l.proc.seen(); !slices.Equal(ev, []string{"interrupt", "kill"}) {
		t.Errorf("events %q, want interrupt then kill after the timeout", ev)
	}
	if err == nil {
		t.Error("an empty recording must be an error")
	}
	if entries, _ := os.ReadDir(r.TempDir); len(entries) != 0 {
		t.Errorf("temp WAV left: %v", entries)
	}
}

// endsWithin fails the test when f does not return within d.
func endsWithin(t *testing.T, d time.Duration, what string, f func()) {
	t.Helper()
	done := make(chan struct{})
	go func() { f(); close(done) }()
	select {
	case <-done:
	case <-time.After(d):
		t.Fatalf("%s hung", what)
	}
}

func TestRecordingStopGivesUpOnARecorderThatSurvivesTheKill(t *testing.T) {
	l := &launches{tmpl: fakeProcess{stubborn: true, unkillable: true}}
	r := newFakeRecorder(t, l)
	rec, _ := r.Start(context.Background())

	var err error
	endsWithin(t, time.Second, "Stop", func() { _, err = rec.Stop() })

	if err == nil || !strings.Contains(err.Error(), "did not stop") {
		t.Errorf("err = %v, want the recorder did not stop", err)
	}
	if entries, _ := os.ReadDir(r.TempDir); len(entries) != 0 {
		t.Errorf("temp WAV left: %v", entries)
	}
}

func TestRecordingCancelGivesUpOnARecorderThatSurvivesTheKill(t *testing.T) {
	l := &launches{tmpl: fakeProcess{unkillable: true}}
	r := newFakeRecorder(t, l)
	rec, _ := r.Start(context.Background())

	endsWithin(t, time.Second, "Cancel", rec.Cancel)

	if entries, _ := os.ReadDir(r.TempDir); len(entries) != 0 {
		t.Errorf("temp WAV left: %v", entries)
	}
}

func TestRecordingStopWithNothingRecorded(t *testing.T) {
	l := &launches{tmpl: fakeProcess{data: "RIFF"}} // only a header
	r := newFakeRecorder(t, l)
	rec, _ := r.Start(context.Background())

	if _, err := rec.Stop(); err == nil || !strings.Contains(err.Error(), "nothing was recorded") {
		t.Errorf("err = %v, want nothing was recorded", err)
	}
	if entries, _ := os.ReadDir(r.TempDir); len(entries) != 0 {
		t.Errorf("temp WAV left: %v", entries)
	}
}

func TestRecordingThatEndedByItselfIsAnError(t *testing.T) {
	l := &launches{tmpl: fakeProcess{data: someAudio}}
	r := newFakeRecorder(t, l)
	rec, _ := r.Start(context.Background())
	l.proc.waitResult = errors.New("exit status 1: no microphone")
	l.proc.end()
	time.Sleep(5 * time.Millisecond) // let the waiter see it

	if _, err := rec.Stop(); err == nil || !strings.Contains(err.Error(), "no microphone") || !strings.Contains(err.Error(), "is one plugged in?") {
		t.Errorf("err = %v, want the recorder's error", err)
	}
	if entries, _ := os.ReadDir(r.TempDir); len(entries) != 0 {
		t.Errorf("temp WAV left: %v", entries)
	}
}

func TestRecordingCancelKillsAndRemovesTheWAV(t *testing.T) {
	l := &launches{tmpl: fakeProcess{data: someAudio}}
	r := newFakeRecorder(t, l)
	rec, _ := r.Start(context.Background())

	rec.Cancel()
	rec.Cancel() // twice is harmless

	if ev := l.proc.seen(); !slices.Equal(ev, []string{"kill"}) {
		t.Errorf("events %q, want one kill", ev)
	}
	if entries, _ := os.ReadDir(r.TempDir); len(entries) != 0 {
		t.Errorf("temp WAV left: %v", entries)
	}
	if _, err := rec.Stop(); err == nil {
		t.Error("Stop after Cancel must fail")
	}
}

func TestRecorderMissingProgramIsUnavailable(t *testing.T) {
	l := &launches{err: &os.PathError{Op: "exec", Path: "pw-record", Err: os.ErrNotExist}}
	r := newFakeRecorder(t, l)

	_, err := r.Start(context.Background())

	mustBeUnavailable(t, err, "pw-record")
	if entries, _ := os.ReadDir(r.TempDir); len(entries) != 0 {
		t.Errorf("temp WAV left: %v", entries)
	}
}

// runs records whisper runs and answers with stdout.
type runs struct {
	name   string
	args   []string
	stdout string
	err    error
	wavWas bool // the WAV existed while whisper ran
}

func (r *runs) run(_ context.Context, name string, args []string) (string, error) {
	r.name, r.args = name, args
	if i := slices.Index(args, "-f"); i >= 0 {
		_, err := os.Stat(args[i+1])
		r.wavWas = err == nil
	}
	return r.stdout, r.err
}

func TestTranscribeRunsWhisperAndRemovesTheWAV(t *testing.T) {
	wav := touch(t, filepath.Join(t.TempDir(), "rec.wav"), someAudio)
	f := &runs{stdout: "\n My name is Anna.\n I am 30 years old.\n\n"}
	tr := &Transcriber{Whisper: "/bin/whisper-cli", Model: "/m/ggml-base.en.bin", Run: f.run}

	text, err := tr.Transcribe(context.Background(), wav)

	if err != nil || text != "My name is Anna. I am 30 years old." {
		t.Errorf("Transcribe = %q, %v", text, err)
	}
	want := []string{"-m", "/m/ggml-base.en.bin", "-f", wav, "-nt", "-np"}
	if f.name != "/bin/whisper-cli" || !slices.Equal(f.args, want) || !f.wavWas {
		t.Errorf("ran %s %q (wav there: %v), want %q", f.name, f.args, f.wavWas, want)
	}
	if _, err := os.Stat(wav); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("the temp WAV was not removed: %v", err)
	}
}

func TestTranscribeDropsNonSpeechMarks(t *testing.T) {
	wav := touch(t, filepath.Join(t.TempDir(), "rec.wav"), someAudio)
	tr := &Transcriber{Run: (&runs{stdout: " [BLANK_AUDIO]\n"}).run}
	if text, err := tr.Transcribe(context.Background(), wav); err != nil || text != "" {
		t.Errorf("silence = %q, %v; want empty", text, err)
	}
	wav = touch(t, filepath.Join(t.TempDir(), "rec.wav"), someAudio)
	if text, _ := (&Transcriber{Run: (&runs{stdout: "(coughs) Hello [MUSIC] there."}).run}).Transcribe(context.Background(), wav); text != "Hello there." {
		t.Errorf("text = %q", text)
	}
}

func TestTranscribeFailureStillRemovesTheWAV(t *testing.T) {
	wav := touch(t, filepath.Join(t.TempDir(), "rec.wav"), someAudio)
	tr := &Transcriber{Whisper: "whisper-cli", Run: (&runs{err: &os.PathError{Op: "exec", Path: "whisper-cli", Err: os.ErrNotExist}}).run}

	_, err := tr.Transcribe(context.Background(), wav)

	mustBeUnavailable(t, err, "brew install whisper-cpp")
	if _, err := os.Stat(wav); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("the temp WAV was not removed: %v", err)
	}
}

func TestNewTranscriberNeedsWhisperAndTheModel(t *testing.T) {
	dir := t.TempDir()
	s := Setup{ModelsDir: dir, LookPath: onPath(map[string]string{"whisper-cli": "/bin/whisper-cli"}), Fallback: []string{}}
	_, err := NewTranscriber(s)
	mustBeUnavailable(t, err, "teacher setup whisper")

	model := touch(t, filepath.Join(dir, ModelFile), "GGML")
	tr, err := NewTranscriber(s)
	if err != nil || tr.Whisper != "/bin/whisper-cli" || tr.Model != model {
		t.Errorf("NewTranscriber = %+v, %v", tr, err)
	}
}

func TestLastLine(t *testing.T) {
	if got := lastLine("warning: x\n\nerror: no device\n"); got != "error: no device" {
		t.Errorf("lastLine = %q", got)
	}
}
