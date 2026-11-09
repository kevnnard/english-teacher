package main

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kevnnard/english-teacher/internal/domain"
	"github.com/kevnnard/english-teacher/internal/tui"
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

// fakeApp replaces startApp for one test and records how it was called.
func fakeApp(t *testing.T) *appCall {
	t.Helper()
	call := &appCall{}
	orig := startApp
	startApp = func(opts appOptions) error {
		call.opts, call.called = opts, true
		return nil
	}
	t.Cleanup(func() { startApp = orig })
	return call
}

type appCall struct {
	opts   appOptions
	called bool
}

func TestRunOpensTheApp(t *testing.T) {
	db := filepath.Join(t.TempDir(), "teacher.db")
	tests := []struct {
		name string
		args []string
		want tui.Screen
	}{
		{name: "no command opens Home", args: []string{"--db", db}, want: tui.Home},
		{name: "review opens Review", args: []string{"--db", db, "review"}, want: tui.Review},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			call := fakeApp(t)

			if err := run(tt.args, &bytes.Buffer{}); err != nil {
				t.Fatalf("run: %v", err)
			}

			if !call.called || call.opts.start != tt.want || call.opts.dbPath != db || call.opts.maxNew != tui.DefaultMaxNew {
				t.Errorf("startApp called=%v with %+v, want start %v on %s", call.called, call.opts, tt.want, db)
			}
		})
	}
}

func TestRunNewFlagCapsNewCards(t *testing.T) {
	call := fakeApp(t)

	if err := run([]string{"--db", filepath.Join(t.TempDir(), "teacher.db"), "--new", "5", "review"}, &bytes.Buffer{}); err != nil {
		t.Fatalf("run: %v", err)
	}

	if call.opts.maxNew != 5 || call.opts.start != tui.Review {
		t.Errorf("startApp called with %+v, want maxNew 5 on Review", call.opts)
	}
}

func TestLoadCardsReadsBothDirections(t *testing.T) {
	db := filepath.Join(t.TempDir(), "teacher.db")
	bank := "../../internal/importer/testdata/vocabulary-bank.md"
	if err := run([]string{"--db", db, "import", bank}, &bytes.Buffer{}); err != nil {
		t.Fatalf("import: %v", err)
	}
	st, err := openStore(db)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	cards, err := loadCards(context.Background(), st)
	if err != nil {
		t.Fatal(err)
	}

	dirs := map[domain.Direction]int{}
	for _, c := range cards {
		dirs[c.Direction]++
	}
	if dirs[domain.Recognition] == 0 || dirs[domain.Recognition] != dirs[domain.Production] {
		t.Errorf("loaded %v, want every word in both directions", dirs)
	}
}

func TestRunUsageErrors(t *testing.T) {
	tests := []struct {
		name    string
		args    []string
		wantErr string
	}{
		{name: "review with extra args", args: []string{"review", "now"}, wantErr: "usage"},
		{name: "unknown command", args: []string{"dance"}, wantErr: `unknown command "dance"`},
		{name: "import without path", args: []string{"import"}, wantErr: "usage"},
		{name: "negative new cap", args: []string{"--new", "-1"}, wantErr: "--new"},
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
