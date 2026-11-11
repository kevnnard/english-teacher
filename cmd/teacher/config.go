package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/kevnnard/english-teacher/internal/llm/pi"
	"github.com/kevnnard/english-teacher/internal/teacher"
)

// capabilities are the jobs that can each use their own model, in the
// order `teacher models` lists them.
var capabilities = []string{"reading", "writing", "conversation", "quiz"}

// config is the optional config.json. Anything missing takes its default.
type config struct {
	Pi     string            `json:"pi,omitempty"`    // the pi binary; "pi" from PATH by default
	Piper  string            `json:"piper,omitempty"` // the piper binary; found on PATH or in ~/.local/bin by default
	Voice  string            `json:"voice,omitempty"` // a voice model path or name; en_US-lessac-medium by default
	Models map[string]string `json:"models"`          // capability -> provider/model
}

// defaultConfig uses a strong model for the jobs where quality matters
// most and a fast one for quizzes. Haiku corrected "I has 30 years" as
// "I have 30 years" with a loose prompt, so correction uses Sonnet.
func defaultConfig() config {
	return config{Pi: "pi", Models: map[string]string{
		"reading":      "claude-bridge/claude-sonnet-5-5",
		"writing":      "claude-bridge/claude-sonnet-5-5",
		"conversation": "claude-bridge/claude-sonnet-5-5",
		"quiz":         "claude-bridge/claude-haiku-5-5",
	}}
}

// defaultConfigPath follows the XDG spec: $XDG_CONFIG_HOME, else ~/.config.
func defaultConfigPath(xdgConfig, home string) string {
	if xdgConfig == "" {
		xdgConfig = filepath.Join(home, ".config")
	}
	return filepath.Join(xdgConfig, "english-teacher", "config.json")
}

// loadConfig reads path over the defaults; a missing file means defaults.
func loadConfig(path string) (config, error) {
	c := defaultConfig()
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return c, nil
	}
	if err != nil {
		return c, err
	}
	var file config
	if err := json.Unmarshal(data, &file); err != nil {
		return c, fmt.Errorf("%s: %w", path, err)
	}
	if file.Pi != "" {
		c.Pi = file.Pi
	}
	c.Piper, c.Voice = file.Piper, file.Voice
	for k, v := range file.Models {
		c.Models[k] = v
	}
	return c, nil
}

func saveConfig(path string, c config) error {
	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, append(data, '\n'), 0o644)
}

// models builds one pi model per capability.
func (c config) models() teacher.Models {
	m := func(cap string) teacher.Model { return pi.Model{Command: c.Pi, Model: c.Models[cap]} }
	return teacher.Models{Reading: m("reading"), Writing: m("writing"), Conversation: m("conversation"), Quiz: m("quiz")}
}

// runModels prints the model each capability uses.
func runModels(path string, stdout io.Writer) error {
	c, err := loadConfig(path)
	if err != nil {
		return err
	}
	fmt.Fprintf(stdout, "Models (%s):\n", path)
	for _, cap := range capabilities {
		fmt.Fprintf(stdout, "  %-12s  %s\n", cap, c.Models[cap])
	}
	fmt.Fprintln(stdout, "\nChange one with `teacher set model <capability> <provider/model>`; see `pi --list-models`.")
	return nil
}

// setModel validates and saves the model of one capability.
func setModel(path string, args []string, stdout io.Writer) error {
	if len(args) != 2 {
		return errors.New(usage)
	}
	cap, model := args[0], strings.TrimSpace(args[1])
	known := false
	for _, c := range capabilities {
		known = known || c == cap
	}
	if !known {
		return fmt.Errorf("unknown capability %q (want %s)", cap, strings.Join(capabilities, ", "))
	}
	if provider, id, ok := strings.Cut(model, "/"); !ok || provider == "" || id == "" {
		return fmt.Errorf("model %q must be provider/model, like claude-bridge/claude-sonnet-5-5", model)
	}

	c, err := loadConfig(path)
	if err != nil {
		return err
	}
	c.Models[cap] = model
	if err := saveConfig(path, c); err != nil {
		return err
	}
	fmt.Fprintf(stdout, "%s model set to %s\n", cap, model)
	return nil
}
