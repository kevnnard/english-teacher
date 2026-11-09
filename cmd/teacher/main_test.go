package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDefaultDBPath(t *testing.T) {
	tests := []struct {
		name    string
		xdgData string
		home    string
		want    string
	}{
		{name: "XDG_DATA_HOME set", xdgData: "/data", home: "/home/k", want: "/data/english-teacher/teacher.db"},
		{name: "fallback to ~/.local/share", xdgData: "", home: "/home/k", want: "/home/k/.local/share/english-teacher/teacher.db"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := defaultDBPath(tt.xdgData, tt.home)
			if got != tt.want {
				t.Errorf("defaultDBPath = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestRunImportTwiceDoesNotDuplicate(t *testing.T) {
	db := filepath.Join(t.TempDir(), "nested", "dir", "teacher.db") // parent dirs are created
	bank := "../../internal/importer/testdata/vocabulary-bank.md"

	var first, second bytes.Buffer
	if err := run([]string{"--db", db, "import", bank}, &first); err != nil {
		t.Fatalf("first import: %v", err)
	}
	if err := run([]string{"--db", db, "import", bank}, &second); err != nil {
		t.Fatalf("second import: %v", err)
	}

	if !strings.Contains(first.String(), "3 new, 0 updated, 3 total") {
		t.Errorf("first import output = %q", first.String())
	}
	if !strings.Contains(second.String(), "0 new, 3 updated, 3 total") {
		t.Errorf("second import output = %q", second.String())
	}
	if _, err := os.Stat(db); err != nil {
		t.Errorf("database not created: %v", err)
	}
}

func TestRunUsageErrors(t *testing.T) {
	tests := []struct {
		name    string
		args    []string
		wantErr string
	}{
		{name: "no command", args: nil, wantErr: "usage"},
		{name: "unknown command", args: []string{"dance"}, wantErr: `unknown command "dance"`},
		{name: "import without path", args: []string{"import"}, wantErr: "usage"},
		{name: "import missing file", args: []string{"import", "does-not-exist.md"}, wantErr: "does-not-exist.md"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			args := append([]string{"--db", filepath.Join(t.TempDir(), "teacher.db")}, tt.args...)

			err := run(args, &bytes.Buffer{})

			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Errorf("run(%v) error = %v, want it to contain %q", tt.args, err, tt.wantErr)
			}
		})
	}
}
