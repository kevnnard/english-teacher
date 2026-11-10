package main

import (
	"bytes"
	"testing"
)

func TestVersionFlagPrintsTheVersion(t *testing.T) {
	var out bytes.Buffer
	if err := run([]string{"--version"}, &out); err != nil {
		t.Fatal(err)
	}
	if got := out.String(); got != "teacher 0.1.0\n" {
		t.Errorf("--version printed %q, want %q", got, "teacher 0.1.0\n")
	}
}
