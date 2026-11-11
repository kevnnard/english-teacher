// Package whisper listens to the learner through whisper.cpp, a local
// speech-to-text program the learner installs (`brew install
// whisper-cpp`), with the model `teacher setup whisper` downloads. A
// Recorder records the microphone with the first recorder program found
// (pw-record, arecord or sox) into a temporary WAV file; a Transcriber
// runs whisper-cli on it and removes it. Nothing leaves the computer.
package whisper

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/kevnnard/english-teacher/internal/speech"
)

// DefaultModel is the model `teacher setup whisper` downloads: base,
// English only, about 150 MB. ModelFile is its file name.
const (
	DefaultModel = "ggml-base.en"
	ModelFile    = DefaultModel + ".bin"
)

// Hints say how to set up each missing piece.
const (
	WhisperHint  = "install whisper.cpp with `brew install whisper-cpp` (or see the whisper.cpp README), then run `teacher setup whisper`"
	ModelHint    = "run `teacher setup whisper`"
	RecorderHint = "install pw-record (pipewire), arecord (alsa-utils) or sox"
)

// Program is the whisper.cpp program that transcribes a WAV file.
const Program = "whisper-cli"

// BrewDirs are where Homebrew puts programs, tried after PATH: brew's
// folder is not on PATH in every shell.
var BrewDirs = []string{"/home/linuxbrew/.linuxbrew/bin", "/opt/homebrew/bin", "/usr/local/bin"}

// Setup is where to find whisper-cli, the model and a recorder.
type Setup struct {
	Whisper   string                       // the whisper-cli binary; empty looks on PATH, then in Fallback
	ModelsDir string                       // where ModelFile is
	Home      string                       // the home folder, for ~/.linuxbrew/bin
	Fallback  []string                     // folders tried after PATH; nil is BrewDirs and ~/.linuxbrew/bin
	LookPath  func(string) (string, error) // exec.LookPath when nil
}

func (s Setup) lookPath(file string) (string, error) {
	if s.LookPath == nil {
		return exec.LookPath(file)
	}
	return s.LookPath(file)
}

// fallback are the folders tried after PATH.
func (s Setup) fallback() []string {
	if s.Fallback != nil {
		return s.Fallback
	}
	dirs := BrewDirs
	if s.Home != "" {
		dirs = append([]string{filepath.Join(s.Home, ".linuxbrew", "bin")}, dirs...)
	}
	return dirs
}

// FindWhisper returns the whisper-cli binary: the configured one, else
// whisper-cli on PATH, else the first in a Fallback folder.
func (s Setup) FindWhisper() (string, error) {
	if s.Whisper != "" {
		if strings.ContainsRune(s.Whisper, os.PathSeparator) {
			if isFile(s.Whisper) {
				return s.Whisper, nil
			}
		} else if p, err := s.lookPath(s.Whisper); err == nil {
			return p, nil
		}
		return "", speech.Unavailable(Program+" not found ("+s.Whisper+")", WhisperHint)
	}
	if p, err := s.lookPath(Program); err == nil {
		return p, nil
	}
	for _, dir := range s.fallback() {
		if p := filepath.Join(dir, Program); isFile(p) {
			return p, nil
		}
	}
	return "", speech.Unavailable(Program+" not found", WhisperHint)
}

// ModelPath is where the model is: <dir>/ggml-base.en.bin.
func ModelPath(dir string) string { return filepath.Join(dir, ModelFile) }

// FindModel returns the model file in ModelsDir.
func (s Setup) FindModel() (string, error) {
	path := ModelPath(s.ModelsDir)
	if !isFile(path) {
		return "", speech.Unavailable("whisper model not found: "+path, ModelHint)
	}
	return path, nil
}

// Recorders are the recorder programs tried in order: PipeWire, ALSA,
// and sox (macOS and others).
var Recorders = []string{"pw-record", "arecord", "sox"}

// FindRecorder returns the first of Recorders on PATH.
func (s Setup) FindRecorder() (string, error) {
	for _, name := range Recorders {
		if p, err := s.lookPath(name); err == nil {
			return p, nil
		}
	}
	return "", speech.Unavailable("no recorder found ("+strings.Join(Recorders, ", ")+")", RecorderHint)
}

// NewRecorder finds a recorder program.
func NewRecorder(s Setup) (*Recorder, error) {
	p, err := s.FindRecorder()
	if err != nil {
		return nil, err
	}
	return &Recorder{Program: p}, nil
}

// NewTranscriber finds whisper-cli and the model.
func NewTranscriber(s Setup) (*Transcriber, error) {
	bin, err := s.FindWhisper()
	if err != nil {
		return nil, err
	}
	model, err := s.FindModel()
	if err != nil {
		return nil, err
	}
	return &Transcriber{Whisper: bin, Model: model}, nil
}

// explain turns a missing program into speech.ErrUnavailable.
func explain(err error, hint string) error {
	if errors.Is(err, os.ErrNotExist) || errors.Is(err, exec.ErrNotFound) {
		return speech.Unavailable(err.Error(), hint+"; see `teacher doctor`")
	}
	return err
}

func isFile(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}

// lastLine is the last non-empty line of s, where programs print the
// real error after their warnings.
func lastLine(s string) string {
	lines := strings.Split(strings.TrimSpace(s), "\n")
	return strings.TrimSpace(lines[len(lines)-1])
}
