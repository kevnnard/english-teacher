package main

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"

	"github.com/kevnnard/english-teacher/internal/llm/agy"
)

const agyHelp = `Install Antigravity (it provides the agy command) and log in to it once with your own account, ` +
	`check that "agy models" lists your models, then run "teacher setup ai" again. ` +
	`If agy is not on your PATH, set its path as "agy" in the config.`

var (
	strongHints = []string{"high", "pro"}
	fastHints   = []string{"low", "flash", "fast", "lite", "mini"}
)

func setupAI(path string, in io.Reader, stdout io.Writer) error {
	c, err := loadConfig(path)
	if err != nil {
		return err
	}
	ids, err := agy.ListModels(context.Background(), c.Agy)
	if err != nil {
		return fmt.Errorf("%v; nothing was saved\n%s", err, agyHelp)
	}

	fmt.Fprintln(stdout, "Your Antigravity models (from `agy models`):")
	for i, id := range ids {
		fmt.Fprintf(stdout, "  %d. %s\n", i+1, id)
	}
	fmt.Fprintln(stdout, "\nPick a model for each capability; Enter keeps the suggested one.")

	r := bufio.NewReader(in)
	chosen := map[string]string{}
	for _, cap := range capabilities {
		id, err := askModel(r, stdout, cap, ids)
		if err != nil {
			return err
		}
		chosen[cap] = "agy/" + id
	}

	for cap, model := range chosen {
		c.Models[cap] = model
	}
	if err := saveConfig(path, c); err != nil {
		return err
	}
	fmt.Fprintf(stdout, "\nSaved to %s:\n", path)
	printModels(stdout, c)
	return nil
}

func askModel(r *bufio.Reader, stdout io.Writer, cap string, ids []string) (string, error) {
	def := suggestModel(cap, ids)
	for {
		fmt.Fprintf(stdout, "%s [%d. %s]: ", cap, def+1, ids[def])
		line, err := r.ReadString('\n')
		if err != nil && !errors.Is(err, io.EOF) {
			return "", err
		}
		answer := strings.TrimSpace(line)
		if answer == "" && err != nil {
			fmt.Fprintln(stdout)
			return "", fmt.Errorf("no answer for %s; nothing was saved", cap)
		}
		if answer == "" {
			return ids[def], nil
		}
		if n, convErr := strconv.Atoi(answer); convErr == nil && n >= 1 && n <= len(ids) {
			return ids[n-1], nil
		}
		fmt.Fprintf(stdout, "Pick a number from 1 to %d, or press Enter for the suggested one.\n", len(ids))
		if err != nil {
			return "", fmt.Errorf("no answer for %s; nothing was saved", cap)
		}
	}
}

func suggestModel(cap string, ids []string) int {
	hints := strongHints
	if cap == "quiz" {
		hints = fastHints
	}
	for _, hint := range hints {
		for i, id := range ids {
			if strings.Contains(strings.ToLower(id), hint) {
				return i
			}
		}
	}
	return 0
}
