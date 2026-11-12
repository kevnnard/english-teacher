package agy

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/kevnnard/english-teacher/internal/teacher"
)

const DefaultTimeout = 90 * time.Second

const listTimeout = 30 * time.Second

type Model struct {
	Command string
	Model   string
	Timeout time.Duration
}

type answer struct {
	Status   string `json:"status"`
	Response string `json:"response"`
}

func (m Model) Complete(ctx context.Context, r teacher.Request) (string, error) {
	command, timeout := orDefault(m.Command), m.Timeout
	if timeout == 0 {
		timeout = DefaultTimeout
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	dir, err := os.MkdirTemp("", "teacher-agy-")
	if err != nil {
		return "", fmt.Errorf("agy %s: %v", m.Model, err)
	}
	defer os.RemoveAll(dir)

	prompt := r.Prompt
	if r.System != "" {
		prompt = r.System + "\n\n---\n\n" + r.Prompt
	}
	args := []string{
		"--model", m.Model, "--sandbox", "--disable-slash-commands",
		"--output-format", "json", "--print-timeout", timeout.String(), "-p=" + prompt,
	}

	stdout, stderr, err := run(ctx, dir, command, args...)
	switch {
	case errors.Is(ctx.Err(), context.DeadlineExceeded):
		return "", fmt.Errorf("agy %s timed out after %v", m.Model, timeout)
	case notFound(err):
		return "", notFoundError(command)
	case err != nil:
		return "", fmt.Errorf("agy %s: %v: %s", m.Model, err, lastLine(stderr))
	}

	var a answer
	if err := json.Unmarshal([]byte(strings.TrimSpace(stdout)), &a); err != nil {
		return "", fmt.Errorf("agy %s gave invalid JSON: %v: %s", m.Model, err, lastLine(stdout))
	}
	if a.Status != "SUCCESS" {
		return "", fmt.Errorf("agy %s answered with status %s: %s", m.Model, a.Status, lastLine(a.Response+"\n"+stderr))
	}
	out := strings.TrimSpace(a.Response)
	if out == "" {
		return "", fmt.Errorf("agy %s gave an empty answer: %s", m.Model, lastLine(stderr))
	}
	return out, nil
}

func ListModels(ctx context.Context, command string) ([]string, error) {
	command = orDefault(command)
	ctx, cancel := context.WithTimeout(ctx, listTimeout)
	defer cancel()

	stdout, stderr, err := run(ctx, "", command, "models")
	switch {
	case errors.Is(ctx.Err(), context.DeadlineExceeded):
		return nil, fmt.Errorf("agy models timed out after %v", listTimeout)
	case notFound(err):
		return nil, notFoundError(command)
	case err != nil:
		return nil, fmt.Errorf("agy models: %v: %s", err, lastLine(stderr))
	}

	var ids []string
	for _, line := range strings.Split(stdout, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "Fetching available models") {
			continue
		}
		id, _, _ := strings.Cut(line, "\t")
		if id = strings.TrimSpace(id); id != "" {
			ids = append(ids, id)
		}
	}
	if len(ids) == 0 {
		return nil, fmt.Errorf("agy models listed no models: %s", lastLine(stderr))
	}
	return ids, nil
}

func run(ctx context.Context, dir, command string, args ...string) (string, string, error) {
	cmd := exec.CommandContext(ctx, command, args...)
	cmd.Dir = dir
	cmd.WaitDelay = time.Second
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	err := cmd.Run()
	return stdout.String(), stderr.String(), err
}

func orDefault(command string) string {
	if command == "" {
		return "agy"
	}
	return command
}

func notFound(err error) bool {
	return errors.Is(err, exec.ErrNotFound) || errors.Is(err, exec.ErrDot) || errors.Is(err, os.ErrNotExist)
}

func notFoundError(command string) error {
	return fmt.Errorf("agy not found (%s): install Antigravity and log in, or set its path in the config", command)
}

func lastLine(s string) string {
	lines := strings.Split(strings.TrimSpace(s), "\n")
	return strings.TrimSpace(lines[len(lines)-1])
}
