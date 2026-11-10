package main

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kevnnard/english-teacher/internal/domain"
	"github.com/kevnnard/english-teacher/internal/store"
)

// progressDir builds a copy of english-master's progress/ folder.
func progressDir(t *testing.T, withExtras bool) string {
	t.Helper()
	dir := t.TempDir()
	bank, err := os.ReadFile("../../internal/importer/testdata/vocabulary-bank.md")
	if err != nil {
		t.Fatal(err)
	}
	files := map[string]string{"vocabulary-bank.md": string(bank)}
	if withExtras {
		files["level.md"] = "**Current Level:** A1 (Beginner)\n**Started:** 2026-08-14\n"
		files["streak.md"] = "**Streak history:**\n- 2026-08-14 — Lesson 1 (Greetings) + Homework review ✓\n"
		files["daily-log.md"] = "## 2026-08-14 (Session 2)\n- x\n## 2026-08-13 (Session 1)\n- y\n"
	}
	for name, body := range files {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

func openDB(t *testing.T, path string) *store.Store {
	t.Helper()
	st, err := store.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	return st
}

func TestImportFolderReadsLevelAndHistory(t *testing.T) {
	db := filepath.Join(t.TempDir(), "teacher.db")
	dir := progressDir(t, true)
	var out bytes.Buffer

	if err := run([]string{"--db", db, "import", dir}, &out); err != nil {
		t.Fatalf("import: %v", err)
	}

	for _, want := range []string{"3 new, 0 updated, 3 total", "Level: A1 (Beginner), started 2026-08-14", "History: 2 new study days"} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("output %q lacks %q", out.String(), want)
		}
	}
	st := openDB(t, db)
	p, _ := st.Profile(context.Background())
	days, _ := st.Days(context.Background())
	if p.Level != domain.A1 || p.Started.String() != "2026-08-14" {
		t.Errorf("profile = %+v", p)
	}
	if len(days) != 2 || len(days[1].Activities) != 3 {
		t.Errorf("days = %+v, want 13th and 14th with 3 activities on the 14th", days)
	}
}

func TestImportFolderTwiceAddsNoDays(t *testing.T) {
	db := filepath.Join(t.TempDir(), "teacher.db")
	dir := progressDir(t, true)
	run([]string{"--db", db, "import", dir}, &bytes.Buffer{})
	var out bytes.Buffer

	if err := run([]string{"--db", db, "import", dir}, &out); err != nil {
		t.Fatal(err)
	}

	if !strings.Contains(out.String(), "History: 0 new study days") {
		t.Errorf("second import output = %q", out.String())
	}
}

func TestImportFolderKeepsNameAndGoal(t *testing.T) {
	db := filepath.Join(t.TempDir(), "teacher.db")
	if err := run([]string{"--db", db, "set", "name", "Kevin"}, &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	run([]string{"--db", db, "set", "goal", "25"}, &bytes.Buffer{})

	if err := run([]string{"--db", db, "import", progressDir(t, true)}, &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}

	p, _ := openDB(t, db).Profile(context.Background())
	if p.Name != "Kevin" || p.DailyGoalMinutes != 25 || p.Level != domain.A1 {
		t.Errorf("profile = %+v", p)
	}
}

func TestImportFolderSaysDayForOneDay(t *testing.T) {
	dir := progressDir(t, false)
	os.WriteFile(filepath.Join(dir, "streak.md"), []byte("- 2026-08-14 — Lesson 1 ✓\n"), 0o644)
	var out bytes.Buffer

	if err := run([]string{"--db", filepath.Join(t.TempDir(), "t.db"), "import", dir}, &out); err != nil {
		t.Fatal(err)
	}

	if !strings.Contains(out.String(), "History: 1 new study day\n") {
		t.Errorf("output = %q", out.String())
	}
}

func TestImportFolderWithOnlyTheBank(t *testing.T) {
	db := filepath.Join(t.TempDir(), "teacher.db")
	var out bytes.Buffer

	if err := run([]string{"--db", db, "import", progressDir(t, false)}, &out); err != nil {
		t.Fatalf("import: %v", err)
	}

	if strings.Contains(out.String(), "Level:") || strings.Contains(out.String(), "History:") {
		t.Errorf("missing optional files should be skipped silently, got %q", out.String())
	}
}

func TestImportFolderWithoutBank(t *testing.T) {
	err := run([]string{"--db", filepath.Join(t.TempDir(), "t.db"), "import", t.TempDir()}, &bytes.Buffer{})
	if err == nil || !strings.Contains(err.Error(), "vocabulary-bank.md") {
		t.Errorf("err = %v, want it to name vocabulary-bank.md", err)
	}
}

func TestSetNameAndGoal(t *testing.T) {
	db := filepath.Join(t.TempDir(), "teacher.db")
	var out bytes.Buffer

	if err := run([]string{"--db", db, "set", "name", "Kevin", "Nard"}, &out); err != nil {
		t.Fatal(err)
	}
	if err := run([]string{"--db", db, "set", "goal", "30"}, &out); err != nil {
		t.Fatal(err)
	}

	if !strings.Contains(out.String(), `Name set to "Kevin Nard"`) || !strings.Contains(out.String(), "Daily goal set to 30 min") {
		t.Errorf("output = %q", out.String())
	}
	p, _ := openDB(t, db).Profile(context.Background())
	if p.Name != "Kevin Nard" || p.DailyGoalMinutes != 30 {
		t.Errorf("profile = %+v", p)
	}
}

func TestSetErrorsLeaveTheProfileUnchanged(t *testing.T) {
	tests := []struct {
		args    []string
		wantErr string
	}{
		{[]string{"set"}, "usage"},
		{[]string{"set", "name"}, "usage"},
		{[]string{"set", "goal", "0"}, "goal must be between 1 and 240"},
		{[]string{"set", "goal", "abc"}, "goal must be between 1 and 240"},
		{[]string{"set", "color", "red"}, `unknown setting "color"`},
	}
	for _, tt := range tests {
		db := filepath.Join(t.TempDir(), "teacher.db")
		err := run(append([]string{"--db", db}, tt.args...), &bytes.Buffer{})
		if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
			t.Errorf("%v: err = %v, want %q", tt.args, err, tt.wantErr)
			continue
		}
		if _, statErr := os.Stat(db); statErr == nil {
			p, _ := openDB(t, db).Profile(context.Background())
			if p != domain.NewProfile() {
				t.Errorf("%v changed the profile to %+v", tt.args, p)
			}
		}
	}
}

func TestAppConfigLoadsProfileDaysAndRecords(t *testing.T) {
	db := filepath.Join(t.TempDir(), "teacher.db")
	if err := run([]string{"--db", db, "import", progressDir(t, true)}, &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	run([]string{"--db", db, "set", "name", "Kevin"}, &bytes.Buffer{})
	st := openDB(t, db)
	ctx := context.Background()

	cfg, err := appConfig(ctx, st, appOptions{dbPath: db, maxNew: 5})
	if err != nil {
		t.Fatal(err)
	}

	if cfg.Profile.Name != "Kevin" || cfg.Profile.Level != domain.A1 || len(cfg.Days) != 2 || len(cfg.Cards) != 6 {
		t.Errorf("cfg = profile %+v, %d days, %d cards", cfg.Profile, len(cfg.Days), len(cfg.Cards))
	}
	nov9 := domain.Date{Year: 2026, Month: 11, Day: 9}
	if err := cfg.Record(domain.Day{Date: nov9, Reviews: 1}); err != nil {
		t.Fatal(err)
	}
	days, _ := st.Days(ctx)
	if len(days) != 3 || days[2].Reviews != 1 {
		t.Errorf("Record did not reach the store: %+v", days)
	}
}

func TestReimportLevelWithoutStartClearsTheStart(t *testing.T) {
	db := filepath.Join(t.TempDir(), "teacher.db")
	dir := progressDir(t, true)
	run([]string{"--db", db, "import", dir}, &bytes.Buffer{})
	os.WriteFile(filepath.Join(dir, "level.md"), []byte("**Current Level:** A2 (Elementary)\n"), 0o644)
	var out bytes.Buffer

	if err := run([]string{"--db", db, "import", dir}, &out); err != nil {
		t.Fatal(err)
	}

	p, _ := openDB(t, db).Profile(context.Background())
	if p.Level != domain.A2 || p.Started != (domain.Date{}) {
		t.Errorf("profile = %+v, want A2 with no start date", p)
	}
	if !strings.Contains(out.String(), "Level: A2 (Elementary)\n") {
		t.Errorf("output = %q", out.String())
	}
}

func TestAppConfigWiresTheTeacherAndAdd(t *testing.T) {
	db := filepath.Join(t.TempDir(), "teacher.db")
	st := openDB(t, db)
	ctx := context.Background()
	cfgPath := filepath.Join(t.TempDir(), "config.json")

	cfg, err := appConfig(ctx, st, appOptions{dbPath: db, configPath: cfgPath, maxNew: 5})
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Teacher == nil {
		t.Error("the app must get a teacher")
	}
	n, err := cfg.Add([]domain.Card{domain.NewCard("teacher")})
	if err != nil || n != 1 {
		t.Fatalf("Add = %d, %v", n, err)
	}
	cards, _ := st.ListCards(ctx, domain.Recognition)
	if len(cards) != 1 || cards[0].Word != "teacher" {
		t.Errorf("cards = %+v", cards)
	}
}

func TestAppConfigReportsABrokenConfig(t *testing.T) {
	db := filepath.Join(t.TempDir(), "teacher.db")
	cfgPath := filepath.Join(t.TempDir(), "config.json")
	os.WriteFile(cfgPath, []byte("{"), 0o644)

	_, err := appConfig(context.Background(), openDB(t, db), appOptions{dbPath: db, configPath: cfgPath})

	if err == nil || !strings.Contains(err.Error(), cfgPath) {
		t.Errorf("err = %v", err)
	}
}
