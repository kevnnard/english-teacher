package agy

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/kevnnard/english-teacher/internal/teacher"
)

func fakeAgy(t *testing.T, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "agy")
	if err := os.WriteFile(path, []byte("#!/bin/sh\n"+body), 0o755); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestCompletePassesExactArgs(t *testing.T) {
	log := filepath.Join(t.TempDir(), "args")
	bin := fakeAgy(t, `for a in "$@"; do printf '%s\n' "$a"; done > `+log+`; echo '{"status":"SUCCESS","response":"ok"}'`)
	m := Model{Command: bin, Model: "gemini-x", Timeout: 42 * time.Second}

	if _, err := m.Complete(context.Background(), teacher.Request{Prompt: "hello"}); err != nil {
		t.Fatal(err)
	}

	raw, err := os.ReadFile(log)
	if err != nil {
		t.Fatal(err)
	}
	got := strings.Split(strings.TrimSpace(string(raw)), "\n")
	want := []string{"--model", "gemini-x", "--sandbox", "--disable-slash-commands", "--output-format", "json", "--print-timeout", "42s", "-p=hello"}
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Errorf("args = %q, want %q", got, want)
	}
}

func TestCompletePrependsSystem(t *testing.T) {
	log := filepath.Join(t.TempDir(), "args")
	bin := fakeAgy(t, `for a in "$@"; do last="$a"; done; printf '%s' "$last" > `+log+`; echo '{"status":"SUCCESS","response":"ok"}'`)

	if _, err := (Model{Command: bin, Model: "m"}).Complete(context.Background(), teacher.Request{System: "You are a teacher.", Prompt: "Correct this."}); err != nil {
		t.Fatal(err)
	}

	raw, _ := os.ReadFile(log)
	p := strings.TrimPrefix(string(raw), "-p=")
	if !strings.HasPrefix(string(raw), "-p=") || !strings.HasPrefix(p, "You are a teacher.") || !strings.HasSuffix(p, "Correct this.") || !strings.Contains(p, "\n\n") {
		t.Errorf("prompt = %q", raw)
	}
}

func TestCompleteRunsInEmptyTempDirRemovedAfter(t *testing.T) {
	log := filepath.Join(t.TempDir(), "dir")
	bin := fakeAgy(t, `pwd > `+log+`; ls -A >> `+log+`; echo '{"status":"SUCCESS","response":"ok"}'`)

	if _, err := (Model{Command: bin, Model: "m"}).Complete(context.Background(), teacher.Request{Prompt: "hi"}); err != nil {
		t.Fatal(err)
	}

	raw, _ := os.ReadFile(log)
	lines := strings.Split(strings.TrimSpace(string(raw)), "\n")
	wd, _ := os.Getwd()
	if len(lines) != 1 || lines[0] == wd {
		t.Fatalf("dir listing = %q, want only a fresh empty dir", lines)
	}
	if _, err := os.Stat(lines[0]); !os.IsNotExist(err) {
		t.Errorf("temp dir %s still exists: %v", lines[0], err)
	}
}

func TestCompleteReturnsTrimmedResponse(t *testing.T) {
	bin := fakeAgy(t, `echo '{"status":"SUCCESS","response":"  OK\n"}'; echo noise >&2`)

	out, err := Model{Command: bin, Model: "m"}.Complete(context.Background(), teacher.Request{Prompt: "hi"})

	if err != nil || out != "OK" {
		t.Errorf("out = %q, err = %v", out, err)
	}
}

func TestCompleteErrors(t *testing.T) {
	cases := map[string]struct {
		body string
		want string
	}{
		"status":   {`echo '{"status":"ERROR","response":"quota exceeded"}'`, "ERROR"},
		"empty":    {`echo '{"status":"SUCCESS","response":"   "}'`, "empty"},
		"bad json": {`echo 'not json'`, "JSON"},
		"failing":  {`echo "not logged in" >&2; exit 1`, "not logged in"},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := Model{Command: fakeAgy(t, c.body), Model: "gemini-x"}.Complete(context.Background(), teacher.Request{Prompt: "hi"})
			if err == nil || !strings.Contains(err.Error(), c.want) || !strings.Contains(err.Error(), "agy") || !strings.Contains(err.Error(), "gemini-x") {
				t.Errorf("err = %v, want agy, the model and %q", err, c.want)
			}
		})
	}
}

func TestCompleteMissingCommand(t *testing.T) {
	_, err := Model{Command: filepath.Join(t.TempDir(), "nope"), Model: "m"}.Complete(context.Background(), teacher.Request{Prompt: "hi"})
	if err == nil || !strings.Contains(err.Error(), "agy not found") || !strings.Contains(err.Error(), "Antigravity") {
		t.Errorf("err = %v", err)
	}
}

func TestCompleteTimesOut(t *testing.T) {
	m := Model{Command: fakeAgy(t, `exec sleep 5`), Model: "gemini-x", Timeout: 100 * time.Millisecond}
	start := time.Now()

	_, err := m.Complete(context.Background(), teacher.Request{Prompt: "hi"})

	if err == nil || !strings.Contains(err.Error(), "timed out") || !strings.Contains(err.Error(), "agy gemini-x") || time.Since(start) > 3*time.Second {
		t.Errorf("err = %v after %v", err, time.Since(start))
	}
}

func TestListModels(t *testing.T) {
	bin := fakeAgy(t, `[ "$1" = models ] || exit 2
printf 'Fetching available models...\n\ngemini-3.8-pro-high\tGemini 3.8 Pro (High)\n  gemini-3.8-flash-low\tGemini 3.8 Flash (Low)  \n\n'`)

	ids, err := ListModels(context.Background(), bin)

	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(ids, "|") != "gemini-3.8-pro-high|gemini-3.8-flash-low" {
		t.Errorf("ids = %q", ids)
	}
}

func TestListModelsFails(t *testing.T) {
	_, err := ListModels(context.Background(), fakeAgy(t, `echo "please log in" >&2; exit 1`))
	if err == nil || !strings.Contains(err.Error(), "agy models") || !strings.Contains(err.Error(), "please log in") {
		t.Errorf("err = %v", err)
	}
	_, err = ListModels(context.Background(), filepath.Join(t.TempDir(), "nope"))
	if err == nil || !strings.Contains(err.Error(), "agy not found") {
		t.Errorf("err = %v", err)
	}
}
