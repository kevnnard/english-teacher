package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const fakeModelList = `Fetching available models...
alpha-low	Alpha (low)
beta-high	Beta (high)
gamma	Gamma
`

func fakeAgyModels(t *testing.T, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "agy")
	if err := os.WriteFile(path, []byte("#!/bin/sh\n"+body), 0o755); err != nil {
		t.Fatal(err)
	}
	return path
}

func listingAgy(t *testing.T) string {
	return fakeAgyModels(t, "[ \"$1\" = models ] || exit 2\nprintf '%s' '"+fakeModelList+"'\n")
}

func answers(t *testing.T, s string) {
	t.Helper()
	orig := stdin
	stdin = strings.NewReader(s)
	t.Cleanup(func() { stdin = orig })
}

func writeConfig(t *testing.T, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.json")
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func savedModels(t *testing.T, path string) (map[string]string, map[string]any) {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var c config
	var raw map[string]any
	if err := json.Unmarshal(data, &c); err != nil {
		t.Fatal(err)
	}
	json.Unmarshal(data, &raw)
	return c.Models, raw
}

func TestSetupAIAcceptsTheSuggestedModelsWithEnter(t *testing.T) {
	bin := listingAgy(t)
	cfg := writeConfig(t, `{"agy":"`+bin+`","voice":"my-voice","models":{"reading":"claude-bridge/x"}}`)
	answers(t, "\n\n\n\n")

	code, out, errOut := runCLI("--config", cfg, "setup", "ai")

	if code != 0 || errOut != "" {
		t.Fatalf("exit %d, stderr %q", code, errOut)
	}
	for _, want := range []string{
		"  1. alpha-low\n  2. beta-high\n  3. gamma\n",
		"reading [2. beta-high]: ",
		"quiz [1. alpha-low]: ",
		"Saved to " + cfg + ":\n",
		"  reading       agy/beta-high\n",
		"  writing       agy/beta-high\n",
		"  conversation  agy/beta-high\n",
		"  quiz          agy/alpha-low\n",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("stdout lacks %q:\n%s", want, out)
		}
	}
	models, raw := savedModels(t, cfg)
	want := map[string]string{"reading": "agy/beta-high", "writing": "agy/beta-high", "conversation": "agy/beta-high", "quiz": "agy/alpha-low"}
	for k, v := range want {
		if models[k] != v {
			t.Errorf("saved %s = %q, want %q", k, models[k], v)
		}
	}
	if raw["voice"] != "my-voice" || raw["agy"] != bin {
		t.Errorf("setup ai lost other settings: %v", raw)
	}
}

func TestSetupAIPicksByNumberAndAsksAgainOnBadInput(t *testing.T) {
	bin := listingAgy(t)
	cfg := writeConfig(t, `{"agy":"`+bin+`"}`)
	answers(t, "3\nzz\n9\n1\n\n 2 \n")

	code, out, errOut := runCLI("--config", cfg, "setup", "ai")

	if code != 0 || errOut != "" {
		t.Fatalf("exit %d, stderr %q", code, errOut)
	}
	if n := strings.Count(out, "Pick a number from 1 to 3, or press Enter for the suggested one.\n"); n != 2 {
		t.Errorf("asked again %d times, want 2:\n%s", n, out)
	}
	models, _ := savedModels(t, cfg)
	want := map[string]string{"reading": "agy/gamma", "writing": "agy/alpha-low", "conversation": "agy/beta-high", "quiz": "agy/beta-high"}
	for k, v := range want {
		if models[k] != v {
			t.Errorf("saved %s = %q, want %q", k, models[k], v)
		}
	}
}

func TestSetupAIFallsBackToTheFirstModel(t *testing.T) {
	bin := fakeAgyModels(t, "printf 'one\\tOne\\ntwo\\tTwo\\n'\n")
	cfg := writeConfig(t, `{"agy":"`+bin+`"}`)
	answers(t, "\n\n\n\n")

	if code, _, errOut := runCLI("--config", cfg, "setup", "ai"); code != 0 {
		t.Fatalf("exit %d, stderr %q", code, errOut)
	}

	models, _ := savedModels(t, cfg)
	for _, cap := range capabilities {
		if models[cap] != "agy/one" {
			t.Errorf("saved %s = %q, want agy/one", cap, models[cap])
		}
	}
}

func TestSetupAIEndOfInputSavesNothing(t *testing.T) {
	bin := listingAgy(t)
	body := `{"agy":"` + bin + `","models":{"reading":"claude-bridge/x"}}`
	cfg := writeConfig(t, body)
	answers(t, "1\n")

	code, _, errOut := runCLI("--config", cfg, "setup", "ai")

	if code != 1 || !strings.Contains(errOut, "no answer for writing; nothing was saved") {
		t.Errorf("exit %d, stderr %q", code, errOut)
	}
	if data, _ := os.ReadFile(cfg); string(data) != body {
		t.Errorf("config changed: %s", data)
	}
}

func TestSetupAIWithoutAgySaysHowToInstallAndSavesNothing(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	cfg := filepath.Join(t.TempDir(), "config.json")
	answers(t, "\n\n\n\n")

	code, out, errOut := runCLI("--config", cfg, "setup", "ai")

	if code != 1 || out != "" {
		t.Errorf("exit %d, stdout %q", code, out)
	}
	for _, want := range []string{"agy not found", "install Antigravity", "log in", "agy models", "nothing was saved"} {
		if !strings.Contains(errOut, want) {
			t.Errorf("stderr lacks %q: %q", want, errOut)
		}
	}
	if _, err := os.Stat(cfg); err == nil {
		t.Error("setup ai created the config without agy")
	}
}

func TestSetupAIWhenAgyModelsFailsSavesNothing(t *testing.T) {
	bin := fakeAgyModels(t, "echo 'please sign in first' >&2\nexit 1\n")
	body := `{"agy":"` + bin + `","models":{"quiz":"claude-bridge/x"}}`
	cfg := writeConfig(t, body)
	answers(t, "\n\n\n\n")

	code, _, errOut := runCLI("--config", cfg, "setup", "ai")

	if code != 1 {
		t.Errorf("exit %d", code)
	}
	for _, want := range []string{"please sign in first", "log in", "agy models", "nothing was saved"} {
		if !strings.Contains(errOut, want) {
			t.Errorf("stderr lacks %q: %q", want, errOut)
		}
	}
	if data, _ := os.ReadFile(cfg); string(data) != body {
		t.Errorf("config changed: %s", data)
	}
}
