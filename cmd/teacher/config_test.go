package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDefaultConfigPath(t *testing.T) {
	if got := defaultConfigPath("/x/config", "/home/k"); got != "/x/config/english-teacher/config.json" {
		t.Errorf("got %q", got)
	}
	if got := defaultConfigPath("", "/home/k"); got != "/home/k/.config/english-teacher/config.json" {
		t.Errorf("got %q", got)
	}
}

func TestModelsListsTheDefaults(t *testing.T) {
	cfg := filepath.Join(t.TempDir(), "config.json")
	var out bytes.Buffer

	if err := run([]string{"--config", cfg, "models"}, &out); err != nil {
		t.Fatal(err)
	}

	for _, want := range []string{
		"reading       claude-bridge/claude-sonnet-5-5",
		"writing       claude-bridge/claude-sonnet-5-5",
		"conversation  claude-bridge/claude-sonnet-5-5",
		"quiz          claude-bridge/claude-haiku-5-5",
		"pi --list-models",
	} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("output lacks %q:\n%s", want, out.String())
		}
	}
	if _, err := os.Stat(cfg); err == nil {
		t.Error("listing models must not create the config file")
	}
}

func TestSetModelSavesOneCapability(t *testing.T) {
	cfg := filepath.Join(t.TempDir(), "nested", "config.json")
	var out bytes.Buffer

	if err := run([]string{"--config", cfg, "set", "model", "writing", "openai/gpt-5"}, &out); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "writing model set to openai/gpt-5") {
		t.Errorf("output = %q", out.String())
	}

	out.Reset()
	run([]string{"--config", cfg, "models"}, &out)
	if !strings.Contains(out.String(), "writing       openai/gpt-5") || !strings.Contains(out.String(), "reading       claude-bridge/claude-sonnet-5-5") {
		t.Errorf("models after set:\n%s", out.String())
	}
}

func TestSetModelErrorsLeaveNoConfig(t *testing.T) {
	tests := []struct {
		args    []string
		wantErr string
	}{
		{[]string{"set", "model", "writing"}, "usage"},
		{[]string{"set", "model", "dancing", "openai/gpt-5"}, `unknown capability "dancing"`},
		{[]string{"set", "model", "writing", "gpt-5"}, "provider/model"},
	}
	for _, tt := range tests {
		cfg := filepath.Join(t.TempDir(), "config.json")
		err := run(append([]string{"--config", cfg}, tt.args...), &bytes.Buffer{})
		if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
			t.Errorf("%v: err = %v, want %q", tt.args, err, tt.wantErr)
		}
		if _, err := os.Stat(cfg); err == nil {
			t.Errorf("%v created the config file", tt.args)
		}
	}
}

func TestBrokenConfigIsAnError(t *testing.T) {
	cfg := filepath.Join(t.TempDir(), "config.json")
	os.WriteFile(cfg, []byte("{not json"), 0o644)

	err := run([]string{"--config", cfg, "models"}, &bytes.Buffer{})

	if err == nil || !strings.Contains(err.Error(), cfg) {
		t.Errorf("err = %v, want it to name the file", err)
	}
}

func TestConfigBuildsATeacherForEveryCapability(t *testing.T) {
	c := defaultConfig()
	m := c.models()
	if m.Reading == nil || m.Writing == nil || m.Conversation == nil || m.Quiz == nil {
		t.Errorf("models = %+v", m)
	}
}
