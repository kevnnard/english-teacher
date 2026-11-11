package main

import (
	"bytes"
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/kevnnard/english-teacher/internal/domain"
	"github.com/kevnnard/english-teacher/internal/store"
)

const shownRemote = "mac:~/.local/share/english-teacher/teacher.db"

type fakeRemote struct {
	home    string
	calls   [][]string
	fail    func(script string) bool
	lost    func(script string) bool
	corrupt bool
}

func useFakeRemote(t *testing.T) *fakeRemote {
	t.Helper()
	f := &fakeRemote{home: t.TempDir()}
	orig := runCommand
	runCommand = func(stdin io.Reader, stdout io.Writer, name string, args ...string) error {
		f.calls = append(f.calls, append([]string{name}, args...))
		if name != "ssh" || len(args) != 3 || args[0] != "--" {
			return fmt.Errorf("unexpected command %q %q", name, args)
		}
		script := args[2]
		if f.fail != nil && f.fail(script) {
			return errors.New("connection lost")
		}
		if f.corrupt && strings.Contains(script, "cat >") {
			stdin = io.MultiReader(stdin, strings.NewReader("damage"))
		}
		cmd := exec.Command("sh", "-c", script)
		cmd.Dir = f.home
		cmd.Stdin, cmd.Stdout = stdin, stdout
		err := cmd.Run()
		if err == nil && f.lost != nil && f.lost(script) {
			return errors.New("connection lost")
		}
		return err
	}
	t.Cleanup(func() { runCommand = orig })
	return f
}

func (f *fakeRemote) db() string {
	return filepath.Join(f.home, ".local", "share", "english-teacher", "teacher.db")
}

func fixSyncClock(t *testing.T) string {
	t.Helper()
	orig := now
	now = func() time.Time { return time.Date(2026, 11, 11, 15, 30, 0, 0, time.Local) }
	t.Cleanup(func() { now = orig })
	return "20261111-153000"
}

func makeStudyDB(t *testing.T, path string, days ...string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	st, err := store.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	for _, d := range days {
		date, err := domain.ParseDate(d)
		if err != nil {
			t.Fatal(err)
		}
		if err := st.RecordStudy(context.Background(), domain.Day{Date: date, Minutes: 10}); err != nil {
			t.Fatal(err)
		}
	}
}

func execSQL(t *testing.T, path string, stmts ...string) {
	t.Helper()
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	for _, s := range stmts {
		if _, err := db.Exec(s); err != nil {
			t.Fatal(err)
		}
	}
}

func studyDays(t *testing.T, path string) []string {
	t.Helper()
	st, err := store.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	days, err := st.Days(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	var out []string
	for _, d := range days {
		out = append(out, d.Date.String())
	}
	return out
}

func dirState(t *testing.T, dir string) map[string]string {
	t.Helper()
	state := map[string]string{}
	err := filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		b, err := os.ReadFile(p)
		state[p] = string(b)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	return state
}

func assertSame(t *testing.T, label string, before, after map[string]string) {
	t.Helper()
	if len(before) != len(after) {
		t.Errorf("%s: files changed from %d to %d", label, len(before), len(after))
	}
	for p, b := range before {
		if after[p] != b {
			t.Errorf("%s: %s changed", label, p)
		}
	}
}

func runCLI(args ...string) (int, string, string) {
	var out, errOut bytes.Buffer
	code := cli(args, &out, &errOut)
	return code, out.String(), errOut.String()
}

func assertCLI(t *testing.T, code int, out, errOut string, wantCode int, wantOut, wantErr string) {
	t.Helper()
	if code != wantCode {
		t.Errorf("exit code = %d, want %d", code, wantCode)
	}
	if out != wantOut {
		t.Errorf("stdout = %q\nwant     %q", out, wantOut)
	}
	if errOut != wantErr {
		t.Errorf("stderr = %q\nwant     %q", errOut, wantErr)
	}
}

func TestSyncWithoutArgumentsPrintsUsage(t *testing.T) {
	f := useFakeRemote(t)

	code, out, errOut := runCLI("--db", filepath.Join(t.TempDir(), "teacher.db"), "sync")

	assertCLI(t, code, out, errOut, 1, "", "teacher: "+syncUsage+"\n")
	if len(f.calls) != 0 {
		t.Errorf("ran %v, want nothing", f.calls)
	}
}

func TestSyncRejectsHostStartingWithDash(t *testing.T) {
	f := useFakeRemote(t)
	local := filepath.Join(t.TempDir(), "teacher.db")
	makeStudyDB(t, local, "2026-11-10")
	before := dirState(t, filepath.Dir(local))

	for _, action := range []string{"push", "pull"} {
		code, out, errOut := runCLI("--db", local, "sync", action, "--", "-oProxyCommand=touch pwned")

		assertCLI(t, code, out, errOut, 1, "", "teacher: host \"-oProxyCommand=touch pwned\" must not start with \"-\"\n")
	}
	if len(f.calls) != 0 {
		t.Errorf("ran %v, want nothing", f.calls)
	}
	assertSame(t, "local", before, dirState(t, filepath.Dir(local)))
}

func TestSyncPullReplacesLocalAndKeepsBackup(t *testing.T) {
	stamp := fixSyncClock(t)
	f := useFakeRemote(t)
	makeStudyDB(t, f.db(), "2026-11-10", "2026-11-11")
	local := filepath.Join(t.TempDir(), "teacher.db")
	makeStudyDB(t, local, "2026-11-10")
	old, _ := os.ReadFile(local)

	code, out, errOut := runCLI("--db", local, "sync", "pull", "mac")

	assertCLI(t, code, out, errOut, 0,
		"Fetching "+shownRemote+"\n"+
			"Backup: "+local+".bak-"+stamp+"\n"+
			"Pulled "+shownRemote+" (last study 2026-11-11) into "+local+"\n", "")
	if got := studyDays(t, local); strings.Join(got, ",") != "2026-11-10,2026-11-11" {
		t.Errorf("local days = %v", got)
	}
	if backup, _ := os.ReadFile(local + ".bak-" + stamp); !bytes.Equal(backup, old) {
		t.Error("backup is not the old database")
	}
	if n := len(dirState(t, filepath.Dir(local))); n != 2 {
		t.Errorf("local dir has %d files, want the database and its backup", n)
	}
}

func TestSyncPullCreatesMissingLocalDatabase(t *testing.T) {
	fixSyncClock(t)
	f := useFakeRemote(t)
	makeStudyDB(t, f.db(), "2026-11-11")
	local := filepath.Join(t.TempDir(), "new", "teacher.db")

	code, out, errOut := runCLI("--db", local, "sync", "pull", "mac")

	assertCLI(t, code, out, errOut, 0,
		"Fetching "+shownRemote+"\n"+
			"No database at "+local+" yet; creating it\n"+
			"Pulled "+shownRemote+" (last study 2026-11-11) into "+local+"\n", "")
	if got := studyDays(t, local); strings.Join(got, ",") != "2026-11-11" {
		t.Errorf("local days = %v", got)
	}
}

func TestSyncPullRefusesWhenLocalHasNewerActivity(t *testing.T) {
	fixSyncClock(t)
	f := useFakeRemote(t)
	makeStudyDB(t, f.db(), "2026-11-10")
	local := filepath.Join(t.TempDir(), "teacher.db")
	makeStudyDB(t, local, "2026-11-11")
	beforeLocal, beforeRemote := dirState(t, filepath.Dir(local)), dirState(t, f.home)

	code, out, errOut := runCLI("--db", local, "sync", "pull", "mac")

	assertCLI(t, code, out, errOut, 1, "Fetching "+shownRemote+"\n",
		"teacher: "+local+" has newer study activity (2026-11-11) than "+shownRemote+" (2026-11-10); nothing changed (use --force to replace it anyway)\n")
	assertSame(t, "local", beforeLocal, dirState(t, filepath.Dir(local)))
	assertSame(t, "remote", beforeRemote, dirState(t, f.home))
}

func TestSyncPullRefusesSameDayWithLaterReview(t *testing.T) {
	fixSyncClock(t)
	f := useFakeRemote(t)
	makeStudyDB(t, f.db(), "2026-11-11")
	local := filepath.Join(t.TempDir(), "teacher.db")
	makeStudyDB(t, local, "2026-11-11")
	execSQL(t, local,
		`INSERT INTO cards (id, word) VALUES (1, 'ship')`,
		`INSERT INTO reviews (card_id, direction, last_reviewed) VALUES (1, 'recognition', '2026-11-11T18:45:00-05:00')`)
	before := dirState(t, filepath.Dir(local))

	code, out, errOut := runCLI("--db", local, "sync", "pull", "mac")

	assertCLI(t, code, out, errOut, 1, "Fetching "+shownRemote+"\n",
		"teacher: "+local+" has newer study activity (2026-11-11 18:45) than "+shownRemote+" (2026-11-11); nothing changed (use --force to replace it anyway)\n")
	assertSame(t, "local", before, dirState(t, filepath.Dir(local)))
}

func TestSyncPullForceReplacesNewerLocal(t *testing.T) {
	stamp := fixSyncClock(t)
	f := useFakeRemote(t)
	makeStudyDB(t, f.db(), "2026-11-10")
	local := filepath.Join(t.TempDir(), "teacher.db")
	makeStudyDB(t, local, "2026-11-11")

	code, out, errOut := runCLI("--db", local, "sync", "pull", "mac", "--force")

	assertCLI(t, code, out, errOut, 0,
		"Fetching "+shownRemote+"\n"+
			"Backup: "+local+".bak-"+stamp+"\n"+
			"Pulled "+shownRemote+" (last study 2026-11-10) into "+local+"\n", "")
	if got := studyDays(t, local); strings.Join(got, ",") != "2026-11-10" {
		t.Errorf("local days = %v", got)
	}
	if got := studyDays(t, local+".bak-"+stamp); strings.Join(got, ",") != "2026-11-11" {
		t.Errorf("backup days = %v", got)
	}
}

func TestSyncPullRefusesNewerSchema(t *testing.T) {
	fixSyncClock(t)
	f := useFakeRemote(t)
	makeStudyDB(t, f.db(), "2026-11-11")
	execSQL(t, f.db(), `PRAGMA user_version = 99`)
	local := filepath.Join(t.TempDir(), "teacher.db")
	makeStudyDB(t, local, "2026-11-10")
	beforeLocal, beforeRemote := dirState(t, filepath.Dir(local)), dirState(t, f.home)

	code, out, errOut := runCLI("--db", local, "sync", "pull", "mac", "--force")

	assertCLI(t, code, out, errOut, 1, "Fetching "+shownRemote+"\n",
		fmt.Sprintf("teacher: %s has schema version 99; this teacher reads up to %d (upgrade teacher here first); nothing changed\n", shownRemote, store.SchemaVersion()))
	assertSame(t, "local", beforeLocal, dirState(t, filepath.Dir(local)))
	assertSame(t, "remote", beforeRemote, dirState(t, f.home))
}

func TestSyncPullRefusesFileThatIsNotATeacherDatabase(t *testing.T) {
	tests := []struct {
		name  string
		write func(t *testing.T, path string)
	}{
		{"garbage", func(t *testing.T, path string) {
			if err := os.WriteFile(path, bytes.Repeat([]byte("not sqlite "), 500), 0o644); err != nil {
				t.Fatal(err)
			}
		}},
		{"other sqlite database", func(t *testing.T, path string) {
			execSQL(t, path, `CREATE TABLE notes (body TEXT)`, `PRAGMA user_version = 3`)
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fixSyncClock(t)
			f := useFakeRemote(t)
			if err := os.MkdirAll(filepath.Dir(f.db()), 0o755); err != nil {
				t.Fatal(err)
			}
			tt.write(t, f.db())
			local := filepath.Join(t.TempDir(), "teacher.db")
			makeStudyDB(t, local, "2026-11-10")
			beforeLocal, beforeRemote := dirState(t, filepath.Dir(local)), dirState(t, f.home)

			code, out, errOut := runCLI("--db", local, "sync", "pull", "mac", "--force")

			assertCLI(t, code, out, errOut, 1, "Fetching "+shownRemote+"\n",
				"teacher: "+shownRemote+" is not a teacher database; nothing changed\n")
			assertSame(t, "local", beforeLocal, dirState(t, filepath.Dir(local)))
			assertSame(t, "remote", beforeRemote, dirState(t, f.home))
		})
	}
}

func TestSyncPullRefusesMissingRemoteDatabase(t *testing.T) {
	useFakeRemote(t)
	local := filepath.Join(t.TempDir(), "teacher.db")
	makeStudyDB(t, local, "2026-11-10")
	before := dirState(t, filepath.Dir(local))

	code, out, errOut := runCLI("--db", local, "sync", "pull", "mac")

	assertCLI(t, code, out, errOut, 1, "Fetching "+shownRemote+"\n",
		"teacher: no database at "+shownRemote+"; nothing changed\n")
	assertSame(t, "local", before, dirState(t, filepath.Dir(local)))
}

func TestSyncPullRefusesRemoteInUse(t *testing.T) {
	f := useFakeRemote(t)
	makeStudyDB(t, f.db(), "2026-11-11")
	if err := os.WriteFile(f.db()+"-journal", []byte("hot"), 0o644); err != nil {
		t.Fatal(err)
	}
	local := filepath.Join(t.TempDir(), "teacher.db")
	makeStudyDB(t, local, "2026-11-10")
	beforeLocal, beforeRemote := dirState(t, filepath.Dir(local)), dirState(t, f.home)

	code, out, errOut := runCLI("--db", local, "sync", "pull", "mac")

	assertCLI(t, code, out, errOut, 1, "Fetching "+shownRemote+"\n",
		"teacher: "+shownRemote+" is in use (close teacher there and try again); nothing changed\n")
	assertSame(t, "local", beforeLocal, dirState(t, filepath.Dir(local)))
	assertSame(t, "remote", beforeRemote, dirState(t, f.home))
}

func TestSyncPullLeavesLocalUnchangedWhenTheConnectionFails(t *testing.T) {
	f := useFakeRemote(t)
	f.fail = func(string) bool { return true }
	local := filepath.Join(t.TempDir(), "teacher.db")
	makeStudyDB(t, local, "2026-11-10")
	before := dirState(t, filepath.Dir(local))

	code, out, errOut := runCLI("--db", local, "sync", "pull", "kev@mac")

	assertCLI(t, code, out, errOut, 1, "Fetching kev@mac:~/.local/share/english-teacher/teacher.db\n",
		"teacher: ssh kev@mac: connection lost\n")
	assertSame(t, "local", before, dirState(t, filepath.Dir(local)))
}

func TestSyncPushReplacesRemoteAndKeepsBackup(t *testing.T) {
	stamp := fixSyncClock(t)
	f := useFakeRemote(t)
	makeStudyDB(t, f.db(), "2026-11-10")
	old, _ := os.ReadFile(f.db())
	local := filepath.Join(t.TempDir(), "teacher.db")
	makeStudyDB(t, local, "2026-11-10", "2026-11-11")
	beforeLocal := dirState(t, filepath.Dir(local))

	code, out, errOut := runCLI("--db", local, "sync", "push", "mac")

	assertCLI(t, code, out, errOut, 0,
		"Checking "+shownRemote+"\n"+
			"Sending "+local+" (last study 2026-11-11)\n"+
			"Backup: "+shownRemote+".bak-"+stamp+"\n"+
			"Pushed "+local+" to "+shownRemote+"\n", "")
	if got := studyDays(t, f.db()); strings.Join(got, ",") != "2026-11-10,2026-11-11" {
		t.Errorf("remote days = %v", got)
	}
	if backup, _ := os.ReadFile(f.db() + ".bak-" + stamp); !bytes.Equal(backup, old) {
		t.Error("remote backup is not the old database")
	}
	if n := len(dirState(t, f.home)); n != 2 {
		t.Errorf("remote has %d files, want the database and its backup", n)
	}
	assertSame(t, "local", beforeLocal, dirState(t, filepath.Dir(local)))
}

func TestSyncPushSendsASnapshotNotTheLiveFile(t *testing.T) {
	fixSyncClock(t)
	f := useFakeRemote(t)
	local := filepath.Join(t.TempDir(), "teacher.db")
	makeStudyDB(t, local, "2026-11-11")
	execSQL(t, local,
		`CREATE TABLE junk (b BLOB)`,
		`INSERT INTO junk VALUES (zeroblob(400000))`,
		`DROP TABLE junk`)
	live, _ := os.ReadFile(local)

	code, _, errOut := runCLI("--db", local, "sync", "push", "mac")

	if code != 0 {
		t.Fatalf("exit %d: %s", code, errOut)
	}
	sent, _ := os.ReadFile(f.db())
	if len(sent) >= len(live) {
		t.Errorf("sent %d bytes, live file has %d: want a vacuumed snapshot", len(sent), len(live))
	}
	if got := studyDays(t, f.db()); strings.Join(got, ",") != "2026-11-11" {
		t.Errorf("remote days = %v", got)
	}
}

func TestSyncPushCreatesMissingRemoteDatabase(t *testing.T) {
	fixSyncClock(t)
	f := useFakeRemote(t)
	local := filepath.Join(t.TempDir(), "teacher.db")
	makeStudyDB(t, local, "2026-11-11")

	code, out, errOut := runCLI("--db", local, "sync", "push", "mac")

	assertCLI(t, code, out, errOut, 0,
		"Checking "+shownRemote+"\n"+
			"No database at "+shownRemote+" yet; creating it\n"+
			"Sending "+local+" (last study 2026-11-11)\n"+
			"Pushed "+local+" to "+shownRemote+"\n", "")
	if got := studyDays(t, f.db()); strings.Join(got, ",") != "2026-11-11" {
		t.Errorf("remote days = %v", got)
	}
}

func TestSyncPushRefusesWhenRemoteHasNewerActivity(t *testing.T) {
	fixSyncClock(t)
	f := useFakeRemote(t)
	makeStudyDB(t, f.db(), "2026-11-11")
	local := filepath.Join(t.TempDir(), "teacher.db")
	makeStudyDB(t, local, "2026-11-10")
	beforeLocal, beforeRemote := dirState(t, filepath.Dir(local)), dirState(t, f.home)

	code, out, errOut := runCLI("--db", local, "sync", "push", "mac")

	assertCLI(t, code, out, errOut, 1, "Checking "+shownRemote+"\n",
		"teacher: "+shownRemote+" has newer study activity (2026-11-11) than "+local+" (2026-11-10); nothing changed (use --force to replace it anyway)\n")
	assertSame(t, "local", beforeLocal, dirState(t, filepath.Dir(local)))
	assertSame(t, "remote", beforeRemote, dirState(t, f.home))
}

func TestSyncPushForceReplacesNewerRemote(t *testing.T) {
	stamp := fixSyncClock(t)
	f := useFakeRemote(t)
	makeStudyDB(t, f.db(), "2026-11-11")
	local := filepath.Join(t.TempDir(), "teacher.db")
	makeStudyDB(t, local, "2026-11-10")

	code, out, errOut := runCLI("--db", local, "sync", "--force", "push", "mac")

	assertCLI(t, code, out, errOut, 0,
		"Checking "+shownRemote+"\n"+
			"Sending "+local+" (last study 2026-11-10)\n"+
			"Backup: "+shownRemote+".bak-"+stamp+"\n"+
			"Pushed "+local+" to "+shownRemote+"\n", "")
	if got := studyDays(t, f.db()); strings.Join(got, ",") != "2026-11-10" {
		t.Errorf("remote days = %v", got)
	}
}

func TestSyncPushRefusesRemoteWithOlderSchema(t *testing.T) {
	fixSyncClock(t)
	f := useFakeRemote(t)
	makeStudyDB(t, f.db(), "2026-11-10")
	execSQL(t, f.db(), `PRAGMA user_version = 5`)
	local := filepath.Join(t.TempDir(), "teacher.db")
	makeStudyDB(t, local, "2026-11-11")
	beforeLocal, beforeRemote := dirState(t, filepath.Dir(local)), dirState(t, f.home)

	code, out, errOut := runCLI("--db", local, "sync", "push", "mac", "--force")

	assertCLI(t, code, out, errOut, 1, "Checking "+shownRemote+"\n",
		fmt.Sprintf("teacher: %s has schema version 5 but %s has %d; upgrade teacher on mac and open it once, then push again; nothing changed\n", shownRemote, local, store.SchemaVersion()))
	assertSame(t, "local", beforeLocal, dirState(t, filepath.Dir(local)))
	assertSame(t, "remote", beforeRemote, dirState(t, f.home))
}

func TestSyncPushRefusesMissingLocalDatabase(t *testing.T) {
	f := useFakeRemote(t)
	makeStudyDB(t, f.db(), "2026-11-10")
	local := filepath.Join(t.TempDir(), "teacher.db")
	beforeRemote := dirState(t, f.home)

	code, out, errOut := runCLI("--db", local, "sync", "push", "mac")

	assertCLI(t, code, out, errOut, 1, "", "teacher: no database at "+local+"; nothing changed\n")
	if _, err := os.Stat(local); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("local database was created: %v", err)
	}
	assertSame(t, "remote", beforeRemote, dirState(t, f.home))
}

func TestSyncPushLeavesRemoteUnchangedWhenTheCopyArrivesDamaged(t *testing.T) {
	fixSyncClock(t)
	f := useFakeRemote(t)
	f.corrupt = true
	makeStudyDB(t, f.db(), "2026-11-10")
	local := filepath.Join(t.TempDir(), "teacher.db")
	makeStudyDB(t, local, "2026-11-11")
	beforeLocal, beforeRemote := dirState(t, filepath.Dir(local)), dirState(t, f.home)

	code, out, errOut := runCLI("--db", local, "sync", "push", "mac")

	assertCLI(t, code, out, errOut, 1,
		"Checking "+shownRemote+"\n"+
			"Sending "+local+" (last study 2026-11-11)\n",
		"teacher: the copy received by mac does not match the one sent; nothing changed\n")
	assertSame(t, "local", beforeLocal, dirState(t, filepath.Dir(local)))
	assertSame(t, "remote", beforeRemote, dirState(t, f.home))
}

func TestSyncPushLeavesRemoteUnchangedWhenTheReplaceFails(t *testing.T) {
	fixSyncClock(t)
	f := useFakeRemote(t)
	f.fail = func(script string) bool { return strings.Contains(script, "mv -f") }
	makeStudyDB(t, f.db(), "2026-11-10")
	local := filepath.Join(t.TempDir(), "teacher.db")
	makeStudyDB(t, local, "2026-11-11")
	beforeLocal, beforeRemote := dirState(t, filepath.Dir(local)), dirState(t, f.home)

	code, out, errOut := runCLI("--db", local, "sync", "push", "mac")

	assertCLI(t, code, out, errOut, 1,
		"Checking "+shownRemote+"\n"+
			"Sending "+local+" (last study 2026-11-11)\n",
		"teacher: ssh mac: connection lost\n")
	assertSame(t, "local", beforeLocal, dirState(t, filepath.Dir(local)))
	assertSame(t, "remote", beforeRemote, dirState(t, f.home))
}

func TestSyncQuotesRemotePathsForTheRemoteShell(t *testing.T) {
	stamp := fixSyncClock(t)
	f := useFakeRemote(t)
	remote := filepath.Join(f.home, "it's $(touch pwned) `touch pwned2`; dir", "teacher.db")
	makeStudyDB(t, remote, "2026-11-10")
	local := filepath.Join(t.TempDir(), "teacher.db")
	makeStudyDB(t, local, "2026-11-11")

	code, out, errOut := runCLI("--db", local, "sync", "push", "--remote-db", remote, "kev@mac")

	shown := "kev@mac:" + remote
	assertCLI(t, code, out, errOut, 0,
		"Checking "+shown+"\n"+
			"Sending "+local+" (last study 2026-11-11)\n"+
			"Backup: "+shown+".bak-"+stamp+"\n"+
			"Pushed "+local+" to "+shown+"\n", "")
	if got := studyDays(t, remote); strings.Join(got, ",") != "2026-11-11" {
		t.Errorf("remote days = %v", got)
	}
	for _, name := range []string{"pwned", "pwned2"} {
		if _, err := os.Stat(filepath.Join(f.home, name)); err == nil {
			t.Errorf("remote shell ran an injected command: %s exists", name)
		}
	}
	for _, c := range f.calls {
		if c[0] != "ssh" || c[1] != "--" || c[2] != "kev@mac" {
			t.Errorf("call %q, want ssh -- kev@mac <script>", c)
		}
	}
}

func TestCLIExistingCommandsBehaveAsBefore(t *testing.T) {
	db := filepath.Join(t.TempDir(), "teacher.db")

	code, out, errOut := runCLI("--db", db, "set", "goal", "30")
	assertCLI(t, code, out, errOut, 0, "Daily goal set to 30 min\n", "")

	code, out, errOut = runCLI("--db", db, "dance")
	assertCLI(t, code, out, errOut, 1, "", "teacher: unknown command \"dance\"\n"+usage+"\n")

	code, out, errOut = runCLI("--version")
	assertCLI(t, code, out, errOut, 0, "teacher "+version+"\n", "")
}

func newerSchemaErr(shown string) string {
	return fmt.Sprintf("teacher: %s has schema version 99; this teacher reads up to %d (upgrade teacher here first); nothing changed\n", shown, store.SchemaVersion())
}

func TestSyncRefusesDestinationWithNewerSchemaEvenWithNewerActivity(t *testing.T) {
	for _, force := range []bool{false, true} {
		t.Run(fmt.Sprintf("push force=%v", force), func(t *testing.T) {
			fixSyncClock(t)
			f := useFakeRemote(t)
			makeStudyDB(t, f.db(), "2026-11-11")
			execSQL(t, f.db(), `PRAGMA user_version = 99`)
			local := filepath.Join(t.TempDir(), "teacher.db")
			makeStudyDB(t, local, "2026-11-10")
			beforeLocal, beforeRemote := dirState(t, filepath.Dir(local)), dirState(t, f.home)
			args := []string{"--db", local, "sync", "push", "mac"}
			if force {
				args = append(args, "--force")
			}

			code, out, errOut := runCLI(args...)

			assertCLI(t, code, out, errOut, 1, "Checking "+shownRemote+"\n", newerSchemaErr(shownRemote))
			assertSame(t, "local", beforeLocal, dirState(t, filepath.Dir(local)))
			assertSame(t, "remote", beforeRemote, dirState(t, f.home))
		})
		t.Run(fmt.Sprintf("pull force=%v", force), func(t *testing.T) {
			fixSyncClock(t)
			f := useFakeRemote(t)
			makeStudyDB(t, f.db(), "2026-11-10")
			local := filepath.Join(t.TempDir(), "teacher.db")
			makeStudyDB(t, local, "2026-11-11")
			execSQL(t, local, `PRAGMA user_version = 99`)
			beforeLocal, beforeRemote := dirState(t, filepath.Dir(local)), dirState(t, f.home)
			args := []string{"--db", local, "sync", "pull", "mac"}
			if force {
				args = append(args, "--force")
			}

			code, out, errOut := runCLI(args...)

			assertCLI(t, code, out, errOut, 1, "Fetching "+shownRemote+"\n", newerSchemaErr(local))
			assertSame(t, "local", beforeLocal, dirState(t, filepath.Dir(local)))
			assertSame(t, "remote", beforeRemote, dirState(t, f.home))
		})
	}
}

func TestSyncTwiceInTheSameSecondKeepsBothBackups(t *testing.T) {
	t.Run("push", func(t *testing.T) {
		stamp := fixSyncClock(t)
		f := useFakeRemote(t)
		makeStudyDB(t, f.db(), "2026-11-10")
		original, _ := os.ReadFile(f.db())
		local := filepath.Join(t.TempDir(), "teacher.db")
		makeStudyDB(t, local, "2026-11-10", "2026-11-11")

		code, _, errOut := runCLI("--db", local, "sync", "push", "mac")
		if code != 0 {
			t.Fatalf("first push: exit %d: %s", code, errOut)
		}
		first, _ := os.ReadFile(f.db())
		code, out, errOut := runCLI("--db", local, "sync", "push", "mac")

		assertCLI(t, code, out, errOut, 0,
			"Checking "+shownRemote+"\n"+
				"Sending "+local+" (last study 2026-11-11)\n"+
				"Backup: "+shownRemote+".bak-"+stamp+"-1\n"+
				"Pushed "+local+" to "+shownRemote+"\n", "")
		if b, _ := os.ReadFile(f.db() + ".bak-" + stamp); !bytes.Equal(b, original) {
			t.Error("first backup is not the original remote database")
		}
		if b, _ := os.ReadFile(f.db() + ".bak-" + stamp + "-1"); !bytes.Equal(b, first) {
			t.Error("second backup is not the database left by the first push")
		}
		if n := len(dirState(t, f.home)); n != 3 {
			t.Errorf("remote has %d files, want the database and two backups", n)
		}
	})
	t.Run("pull", func(t *testing.T) {
		stamp := fixSyncClock(t)
		f := useFakeRemote(t)
		makeStudyDB(t, f.db(), "2026-11-10", "2026-11-11")
		local := filepath.Join(t.TempDir(), "teacher.db")
		makeStudyDB(t, local, "2026-11-10")
		original, _ := os.ReadFile(local)

		code, _, errOut := runCLI("--db", local, "sync", "pull", "mac")
		if code != 0 {
			t.Fatalf("first pull: exit %d: %s", code, errOut)
		}
		first, _ := os.ReadFile(local)
		code, out, errOut := runCLI("--db", local, "sync", "pull", "mac")

		assertCLI(t, code, out, errOut, 0,
			"Fetching "+shownRemote+"\n"+
				"Backup: "+local+".bak-"+stamp+"-1\n"+
				"Pulled "+shownRemote+" (last study 2026-11-11) into "+local+"\n", "")
		if b, _ := os.ReadFile(local + ".bak-" + stamp); !bytes.Equal(b, original) {
			t.Error("first backup is not the original local database")
		}
		if b, _ := os.ReadFile(local + ".bak-" + stamp + "-1"); !bytes.Equal(b, first) {
			t.Error("second backup is not the database left by the first pull")
		}
		if n := len(dirState(t, filepath.Dir(local))); n != 3 {
			t.Errorf("local dir has %d files, want the database and two backups", n)
		}
	})
}

func TestSyncRefusesMalformedTeacherSchemaEvenWithForce(t *testing.T) {
	malformed := []struct {
		name  string
		write func(t *testing.T, path string)
	}{
		{"cards without id", func(t *testing.T, path string) {
			if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
				t.Fatal(err)
			}
			execSQL(t, path, `CREATE TABLE cards (wrong TEXT)`, `PRAGMA user_version = 1`)
		}},
		{"current version without reviews", func(t *testing.T, path string) {
			makeStudyDB(t, path, "2026-11-12")
			execSQL(t, path, `DROP TABLE reviews`)
		}},
	}
	for _, m := range malformed {
		t.Run("pull "+m.name, func(t *testing.T) {
			fixSyncClock(t)
			f := useFakeRemote(t)
			m.write(t, f.db())
			local := filepath.Join(t.TempDir(), "teacher.db")
			makeStudyDB(t, local, "2026-11-10")
			beforeLocal, beforeRemote := dirState(t, filepath.Dir(local)), dirState(t, f.home)

			code, out, errOut := runCLI("--db", local, "sync", "pull", "mac", "--force")

			assertCLI(t, code, out, errOut, 1, "Fetching "+shownRemote+"\n",
				"teacher: "+shownRemote+" is not a teacher database; nothing changed\n")
			assertSame(t, "local", beforeLocal, dirState(t, filepath.Dir(local)))
			assertSame(t, "remote", beforeRemote, dirState(t, f.home))
		})
		t.Run("push "+m.name, func(t *testing.T) {
			fixSyncClock(t)
			f := useFakeRemote(t)
			makeStudyDB(t, f.db(), "2026-11-10")
			local := filepath.Join(t.TempDir(), "teacher.db")
			m.write(t, local)
			beforeLocal, beforeRemote := dirState(t, filepath.Dir(local)), dirState(t, f.home)

			code, out, errOut := runCLI("--db", local, "sync", "push", "mac", "--force")

			assertCLI(t, code, out, errOut, 1, "",
				"teacher: "+local+" is not a teacher database; nothing changed\n")
			assertSame(t, "local", beforeLocal, dirState(t, filepath.Dir(local)))
			assertSame(t, "remote", beforeRemote, dirState(t, f.home))
		})
	}
}

func TestSyncPushRefusesSourceWithUnsupportedSchemaToMissingRemote(t *testing.T) {
	fixSyncClock(t)
	f := useFakeRemote(t)
	local := filepath.Join(t.TempDir(), "teacher.db")
	makeStudyDB(t, local, "2026-11-11")
	execSQL(t, local, `PRAGMA user_version = 99`)
	before := dirState(t, filepath.Dir(local))

	code, out, errOut := runCLI("--db", local, "sync", "push", "mac", "--force")

	assertCLI(t, code, out, errOut, 1, "", newerSchemaErr(local))
	if len(f.calls) != 0 {
		t.Errorf("ran %v, want nothing", f.calls)
	}
	if n := len(dirState(t, f.home)); n != 0 {
		t.Errorf("remote has %d files, want none", n)
	}
	assertSame(t, "local", before, dirState(t, filepath.Dir(local)))
}

func TestSyncPushReportsATempFileItCouldNotRemove(t *testing.T) {
	stamp := fixSyncClock(t)
	f := useFakeRemote(t)
	f.lost = func(script string) bool { return strings.Contains(script, "cat >") }
	f.fail = func(script string) bool { return strings.Contains(script, "rm -f") }
	makeStudyDB(t, f.db(), "2026-11-10")
	old, _ := os.ReadFile(f.db())
	local := filepath.Join(t.TempDir(), "teacher.db")
	makeStudyDB(t, local, "2026-11-11")

	code, out, errOut := runCLI("--db", local, "sync", "push", "mac")

	assertCLI(t, code, out, errOut, 1,
		"Checking "+shownRemote+"\n"+
			"Sending "+local+" (last study 2026-11-11)\n",
		"teacher: ssh mac: connection lost; "+shownRemote+" was not replaced, but the temporary file "+
			shownRemote+".sync-"+stamp+" is left there and could not be removed (ssh mac: connection lost)\n")
	if got, _ := os.ReadFile(f.db()); !bytes.Equal(got, old) {
		t.Error("remote database changed")
	}
	if _, err := os.Stat(f.db() + ".sync-" + stamp); err != nil {
		t.Errorf("the reported temp file: %v", err)
	}
}

func TestSyncPushDoesNotClaimAnUnconfirmedBackup(t *testing.T) {
	stamp := fixSyncClock(t)
	f := useFakeRemote(t)
	final := false
	f.fail = func(script string) bool {
		if strings.Contains(script, "mv -f") {
			final = true
		}
		return final
	}
	makeStudyDB(t, f.db(), "2026-11-10")
	old, _ := os.ReadFile(f.db())
	local := filepath.Join(t.TempDir(), "teacher.db")
	makeStudyDB(t, local, "2026-11-11")

	code, out, errOut := runCLI("--db", local, "sync", "push", "mac")

	assertCLI(t, code, out, errOut, 1,
		"Checking "+shownRemote+"\n"+
			"Sending "+local+" (last study 2026-11-11)\n",
		"teacher: ssh mac: connection lost; the connection dropped at the final step, so "+shownRemote+
			" may already be replaced by "+local+"; if it was, its previous database was backed up first as "+
			shownRemote+".bak-"+stamp+" or the same name with a -N suffix\n")
	if b, _ := os.ReadFile(f.db()); !bytes.Equal(b, old) {
		t.Error("remote database changed")
	}
	if m, _ := filepath.Glob(f.db() + ".bak-*"); len(m) != 0 {
		t.Errorf("backups = %v, want none", m)
	}
}

func TestSyncPushReportsAPossibleReplaceWhenTheConnectionDropsAtTheEnd(t *testing.T) {
	stamp := fixSyncClock(t)
	f := useFakeRemote(t)
	gone := false
	f.lost = func(script string) bool {
		if strings.Contains(script, "mv -f") {
			gone = true
		}
		return gone
	}
	f.fail = func(string) bool { return gone }
	makeStudyDB(t, f.db(), "2026-11-10")
	old, _ := os.ReadFile(f.db())
	local := filepath.Join(t.TempDir(), "teacher.db")
	makeStudyDB(t, local, "2026-11-11")

	code, out, errOut := runCLI("--db", local, "sync", "push", "mac")

	assertCLI(t, code, out, errOut, 1,
		"Checking "+shownRemote+"\n"+
			"Sending "+local+" (last study 2026-11-11)\n",
		"teacher: ssh mac: connection lost; the connection dropped at the final step, so "+shownRemote+
			" may already be replaced by "+local+"; its previous database is kept as "+shownRemote+".bak-"+stamp+"\n")
	if b, _ := os.ReadFile(f.db() + ".bak-" + stamp); !bytes.Equal(b, old) {
		t.Error("backup is not the old remote database")
	}
	if got := studyDays(t, f.db()); strings.Join(got, ",") != "2026-11-11" {
		t.Errorf("remote days = %v", got)
	}
}
