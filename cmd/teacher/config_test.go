package main

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kevnnard/english-teacher/internal/domain"
)

func TestDefaultConfigPath(t *testing.T) {
	if got := defaultConfigPath("/x/config", "/home/k"); got != "/x/config/english-teacher/config.json" {
		t.Errorf("got %q", got)
	}
	if got := defaultConfigPath("", "/home/k"); got != "/home/k/.config/english-teacher/config.json" {
		t.Errorf("got %q", got)
	}
}

func TestModelsShowsUnsetModelsAndHowToSetThem(t *testing.T) {
	cfg := filepath.Join(t.TempDir(), "config.json")
	var out bytes.Buffer

	if err := run([]string{"--config", cfg, "models"}, &out); err != nil {
		t.Fatal(err)
	}

	for _, want := range []string{
		"reading       (not set)",
		"writing       (not set)",
		"conversation  (not set)",
		"quiz          (not set)",
		"teacher setup ai",
		"agy models",
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
	if !strings.Contains(out.String(), "writing       openai/gpt-5") || !strings.Contains(out.String(), "reading       (not set)") {
		t.Errorf("models after set:\n%s", out.String())
	}
}

func TestSetModelAcceptsAnAgyModel(t *testing.T) {
	cfg := filepath.Join(t.TempDir(), "config.json")
	var out bytes.Buffer

	if err := run([]string{"--config", cfg, "set", "model", "quiz", "agy/model-x-low"}, &out); err != nil {
		t.Fatal(err)
	}

	out.Reset()
	run([]string{"--config", cfg, "models"}, &out)
	if !strings.Contains(out.String(), "quiz          agy/model-x-low") {
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
		{[]string{"set", "model", "writing", "agy/"}, "provider/model"},
		{[]string{"set", "model", "writing", "/model-x"}, "provider/model"},
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

func fakeAI(t *testing.T, dir, name, answer string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	script := "#!/bin/sh\nprintf '%s\\n' \"$@\" > " + path + ".args\nprintf '%s\\n' '" + answer + "'\n"
	if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	return path
}

func fakeAgyAnswer(text string) string {
	return `{"status":"SUCCESS","response":"{\"title\":\"T\",\"text\":\"` + text + `\",\"glossary\":[]}"}`
}

func readingWith(t *testing.T, configJSON string) (string, error) {
	t.Helper()
	cfgPath := filepath.Join(t.TempDir(), "config.json")
	if err := os.WriteFile(cfgPath, []byte(configJSON), 0o644); err != nil {
		t.Fatal(err)
	}
	db := filepath.Join(t.TempDir(), "teacher.db")
	cfg, err := appConfig(context.Background(), openDB(t, db), appOptions{dbPath: db, configPath: cfgPath})
	if err != nil {
		t.Fatal(err)
	}
	r, err := cfg.Teacher.Reading(context.Background(), domain.A1, []string{"hello"})
	return r.Text, err
}

func calledWith(t *testing.T, bin string) string {
	t.Helper()
	args, err := os.ReadFile(bin + ".args")
	if err != nil {
		return ""
	}
	return string(args)
}

func TestAgyModelsGoToAgyAndOthersToPi(t *testing.T) {
	dir := t.TempDir()
	agyBin := fakeAI(t, dir, "agy", fakeAgyAnswer("From agy"))
	piBin := fakeAI(t, dir, "pi", `{"title":"T","text":"From pi","glossary":[]}`)

	text, err := readingWith(t, `{"pi":"`+piBin+`","agy":"`+agyBin+`","models":{"reading":"agy/model-x-high"}}`)

	if err != nil || text != "From agy" {
		t.Fatalf("reading = %q, %v", text, err)
	}
	if args := calledWith(t, agyBin); !strings.Contains(args, "--model\nmodel-x-high\n") {
		t.Errorf("agy args = %q, want the model id without the agy/ prefix", args)
	}
	if calledWith(t, piBin) != "" {
		t.Error("pi was called for an agy model")
	}
}

func TestAnExistingPiConfigKeepsWorking(t *testing.T) {
	dir := t.TempDir()
	agyBin := fakeAI(t, dir, "agy", fakeAgyAnswer("From agy"))
	piBin := fakeAI(t, dir, "pi", `{"title":"T","text":"From pi","glossary":[]}`)

	text, err := readingWith(t, `{"pi":"`+piBin+`","agy":"`+agyBin+`","models":{"reading":"claude-bridge/x"}}`)

	if err != nil || text != "From pi" {
		t.Fatalf("reading = %q, %v", text, err)
	}
	if args := calledWith(t, piBin); !strings.Contains(args, "--model\nclaude-bridge/x\n") {
		t.Errorf("pi args = %q", args)
	}
	if calledWith(t, agyBin) != "" {
		t.Error("agy was called for a pi model")
	}
}

func TestAgyIsFoundOnPathByDefault(t *testing.T) {
	dir := t.TempDir()
	agyBin := fakeAI(t, dir, "agy", fakeAgyAnswer("From agy on PATH"))
	t.Setenv("PATH", dir)

	text, err := readingWith(t, `{"models":{"reading":"agy/model-x-high"}}`)

	if err != nil || text != "From agy on PATH" {
		t.Fatalf("reading = %q, %v", text, err)
	}
	if calledWith(t, agyBin) == "" {
		t.Error("the agy on PATH was not called")
	}
}

func TestAgyPathFromTheConfig(t *testing.T) {
	onPath, other := t.TempDir(), t.TempDir()
	pathBin := fakeAI(t, onPath, "agy", fakeAgyAnswer("From PATH"))
	configBin := fakeAI(t, other, "my-agy", fakeAgyAnswer("From config"))
	t.Setenv("PATH", onPath)

	text, err := readingWith(t, `{"agy":"`+configBin+`","models":{"reading":"agy/model-x-high"}}`)

	if err != nil || text != "From config" {
		t.Fatalf("reading = %q, %v", text, err)
	}
	if calledWith(t, pathBin) != "" {
		t.Error("the agy on PATH was called instead of the configured one")
	}
}

func TestALessonWithoutAModelSaysToRunSetupAI(t *testing.T) {
	dir := t.TempDir()
	piBin := fakeAI(t, dir, "pi", `{"title":"T","text":"From pi","glossary":[]}`)
	agyBin := fakeAI(t, dir, "agy", fakeAgyAnswer("From agy"))

	_, err := readingWith(t, `{"pi":"`+piBin+`","agy":"`+agyBin+`","models":{"quiz":"agy/model-x-low"}}`)

	if err == nil || !strings.Contains(err.Error(), `no AI model set for reading: run "teacher setup ai"`) {
		t.Errorf("err = %v", err)
	}
	if calledWith(t, piBin) != "" || calledWith(t, agyBin) != "" {
		t.Error("an AI program was called without a model")
	}
}
