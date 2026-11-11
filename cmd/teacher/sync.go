package main

import (
	"bytes"
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"strings"

	"github.com/kevnnard/english-teacher/internal/store"
)

const syncUsage = `usage:
  teacher [--db path] sync push [user@]host  send this database to host, replacing the one there
  teacher [--db path] sync pull [user@]host  bring the database of host here, replacing this one

  --remote-db path  the database on host (default ~/.local/share/english-teacher/teacher.db)
  --force           replace a database that has newer study activity

Close teacher on both machines before syncing.`

const (
	defaultRemoteDB = "~/.local/share/english-teacher/teacher.db"
	stampLayout     = "20060102-150405"
	exitMissing     = 7
	exitInUse       = 8
)

type runner func(stdin io.Reader, stdout io.Writer, name string, args ...string) error

var runCommand runner = execCommand

func execCommand(stdin io.Reader, stdout io.Writer, name string, args ...string) error {
	cmd := exec.Command(name, args...)
	cmd.Stdin, cmd.Stdout = stdin, stdout
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	err := cmd.Run()
	if msg := strings.TrimSpace(stderr.String()); err != nil && msg != "" {
		return fmt.Errorf("%w: %s", err, msg)
	}
	return err
}

func exitCode(err error) int {
	var e interface{ ExitCode() int }
	if errors.As(err, &e) {
		return e.ExitCode()
	}
	return -1
}

func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

type remote struct {
	host  string
	path  string
	shown string
}

func (r remote) String() string {
	return r.host + ":" + r.shown
}

func (r remote) ssh(stdin io.Reader, stdout io.Writer, script string) error {
	if err := runCommand(stdin, stdout, "ssh", "--", r.host, script); err != nil {
		return fmt.Errorf("ssh %s: %w", r.host, err)
	}
	return nil
}

func (r remote) fetch(dst string) (bool, error) {
	f, err := os.Create(dst)
	if err != nil {
		return false, err
	}
	defer f.Close()
	p := shellQuote(r.path)
	script := fmt.Sprintf("test -e %s || exit %d; test -e %s && exit %d; test -e %s && exit %d; cat -- %s",
		p, exitMissing, shellQuote(r.path+"-journal"), exitInUse, shellQuote(r.path+"-wal"), exitInUse, p)
	err = r.ssh(nil, f, script)
	switch exitCode(err) {
	case exitMissing:
		return false, nil
	case exitInUse:
		return false, fmt.Errorf("%s is in use (close teacher there and try again); nothing changed", r)
	}
	if err != nil {
		return false, err
	}
	return true, f.Close()
}

type replaceUnknown struct {
	err    error
	backup string
}

func (e *replaceUnknown) Error() string { return e.err.Error() }

func (e *replaceUnknown) Unwrap() error { return e.err }

func (r remote) replace(snapshot []byte, stamp string) (string, error) {
	p := shellQuote(r.path)
	next := shellQuote(r.path + ".sync-" + stamp)
	backup := shellQuote(r.path + ".bak-" + stamp)
	discard := func(err error) error {
		if rmErr := r.ssh(nil, nil, "rm -f -- "+next); rmErr != nil {
			return fmt.Errorf("%w; %s was not replaced, but the temporary file %s.sync-%s is left there and could not be removed (%v)",
				err, r, r, stamp, rmErr)
		}
		return err
	}

	if err := r.ssh(bytes.NewReader(snapshot), nil, "mkdir -p -- "+shellQuote(path.Dir(r.path))+" && cat > "+next); err != nil {
		return "", discard(err)
	}
	var got bytes.Buffer
	if err := r.ssh(nil, &got, "cat -- "+next); err != nil {
		return "", discard(err)
	}
	if !bytes.Equal(got.Bytes(), snapshot) {
		return "", discard(fmt.Errorf("the copy received by %s does not match the one sent; nothing changed", r.host))
	}
	script := "if test -e " + p + "; then s=; i=0; while test -e " + backup + `"$s"; do i=$((i+1)); s=-$i; done; ` +
		"(set -C; : > " + backup + `"$s") && cp -p -- ` + p + " " + backup + `"$s" && printf 'backup%s
' "$s" || exit 1; fi; ` +
		"mv -f -- " + next + " " + p
	var out bytes.Buffer
	err := r.ssh(nil, &out, script)
	saved := ""
	if line, ok := strings.CutPrefix(strings.TrimSpace(out.String()), "backup"); ok {
		saved = r.String() + ".bak-" + stamp + line
	}
	if err == nil {
		return saved, nil
	}
	if r.ssh(nil, nil, "test -e "+next) == nil {
		return "", discard(err)
	}
	return saved, &replaceUnknown{err: err, backup: saved}
}

func parseSync(args []string) (string, remote, bool, error) {
	fs := flag.NewFlagSet("sync", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	remoteDB := fs.String("remote-db", defaultRemoteDB, "the database on host")
	force := fs.Bool("force", false, "replace a database that has newer study activity")
	var pos []string
	for {
		if err := fs.Parse(args); err != nil {
			return "", remote{}, false, fmt.Errorf("%w\n%s", err, syncUsage)
		}
		if fs.NArg() == 0 {
			break
		}
		pos = append(pos, fs.Arg(0))
		args = fs.Args()[1:]
	}
	if len(pos) != 2 || pos[1] == "" {
		return "", remote{}, false, errors.New(syncUsage)
	}
	action, host := pos[0], pos[1]
	if action != "push" && action != "pull" {
		return "", remote{}, false, fmt.Errorf("unknown sync action %q (want push or pull)\n%s", action, syncUsage)
	}
	if strings.HasPrefix(host, "-") {
		return "", remote{}, false, fmt.Errorf("host %q must not start with \"-\"", host)
	}
	if *remoteDB == "" {
		return "", remote{}, false, fmt.Errorf("--remote-db must not be empty\n%s", syncUsage)
	}
	return action, remote{host: host, path: strings.TrimPrefix(*remoteDB, "~/"), shown: *remoteDB}, *force, nil
}

func runSync(dbPath string, args []string, stdout io.Writer) error {
	action, r, force, err := parseSync(args)
	if err != nil {
		return err
	}
	tmp, err := os.MkdirTemp("", "teacher-sync-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tmp)

	ctx := context.Background()
	if action == "push" {
		return syncPush(ctx, dbPath, r, force, tmp, stdout)
	}
	return syncPull(ctx, dbPath, r, force, tmp, stdout)
}

func syncPull(ctx context.Context, dbPath string, r remote, force bool, tmp string, stdout io.Writer) error {
	fmt.Fprintf(stdout, "Fetching %s\n", r)
	incoming := filepath.Join(tmp, "incoming.db")
	found, err := r.fetch(incoming)
	if err != nil {
		return err
	}
	if !found {
		return fmt.Errorf("no database at %s; nothing changed", r)
	}
	in, err := inspectDB(ctx, incoming, r.String())
	if err != nil {
		return err
	}
	busy, err := inUse(dbPath)
	if err != nil {
		return err
	}
	if busy {
		return fmt.Errorf("%s is in use (close teacher here and try again); nothing changed", dbPath)
	}
	exists, err := fileExists(dbPath)
	if err != nil {
		return err
	}
	if exists {
		cur, err := inspectDB(ctx, dbPath, dbPath)
		if err != nil {
			return err
		}
		if !force && cur.Latest.After(in.Latest) {
			return newerActivity(dbPath, cur.Latest, r.String(), in.Latest)
		}
	} else {
		fmt.Fprintf(stdout, "No database at %s yet; creating it\n", dbPath)
	}
	if err := replaceLocal(ctx, incoming, dbPath, exists, stdout); err != nil {
		return err
	}
	fmt.Fprintf(stdout, "Pulled %s (last study %s) into %s\n", r, in.Latest, dbPath)
	return nil
}

func syncPush(ctx context.Context, dbPath string, r remote, force bool, tmp string, stdout io.Writer) error {
	exists, err := fileExists(dbPath)
	if err != nil {
		return err
	}
	if !exists {
		return fmt.Errorf("no database at %s; nothing changed", dbPath)
	}
	outgoing := filepath.Join(tmp, "outgoing.db")
	if err := store.Snapshot(ctx, dbPath, outgoing); err != nil {
		return fmt.Errorf("snapshot of %s: %w; nothing changed", dbPath, err)
	}
	out, err := inspectDB(ctx, outgoing, dbPath)
	if err != nil {
		return err
	}

	fmt.Fprintf(stdout, "Checking %s\n", r)
	current := filepath.Join(tmp, "current.db")
	found, err := r.fetch(current)
	if err != nil {
		return err
	}
	if found {
		cur, err := inspectDB(ctx, current, r.String())
		if cur.Version >= 1 && out.Version > cur.Version {
			return fmt.Errorf("%s has schema version %d but %s has %d; upgrade teacher on %s and open it once, then push again; nothing changed",
				r, cur.Version, dbPath, out.Version, r.host)
		}
		if err != nil {
			return err
		}
		if !force && cur.Latest.After(out.Latest) {
			return newerActivity(r.String(), cur.Latest, dbPath, out.Latest)
		}
	} else {
		fmt.Fprintf(stdout, "No database at %s yet; creating it\n", r)
	}

	fmt.Fprintf(stdout, "Sending %s (last study %s)\n", dbPath, out.Latest)
	snapshot, err := os.ReadFile(outgoing)
	if err != nil {
		return err
	}
	stamp := now().Format(stampLayout)
	backup, err := r.replace(snapshot, stamp)
	var unknown *replaceUnknown
	if errors.As(err, &unknown) {
		kept := "there was no database there before"
		switch {
		case unknown.backup != "":
			kept = "its previous database is kept as " + unknown.backup
		case found:
			kept = fmt.Sprintf("if it was, its previous database was backed up first as %s.bak-%s or the same name with a -N suffix", r, stamp)
		}
		return fmt.Errorf("%w; the connection dropped at the final step, so %s may already be replaced by %s; %s",
			unknown.err, r, dbPath, kept)
	}
	if err != nil {
		return err
	}
	if backup != "" {
		fmt.Fprintf(stdout, "Backup: %s\n", backup)
	}
	fmt.Fprintf(stdout, "Pushed %s to %s\n", dbPath, r)
	return nil
}

func replaceLocal(ctx context.Context, src, dst string, exists bool, stdout io.Writer) error {
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	stamp := now().Format(stampLayout)
	next := dst + ".sync-" + stamp
	if err := store.Snapshot(ctx, src, next); err != nil {
		os.Remove(next)
		return fmt.Errorf("write %s: %w; nothing changed", next, err)
	}
	defer os.Remove(next)
	if _, err := inspectDB(ctx, next, next); err != nil {
		return err
	}
	if exists {
		backup, err := backupLocal(dst, dst+".bak-"+stamp)
		if err != nil {
			return fmt.Errorf("backup %s: %w; nothing changed", backup, err)
		}
		fmt.Fprintf(stdout, "Backup: %s\n", backup)
	}
	return os.Rename(next, dst)
}

func backupLocal(src, base string) (string, error) {
	name := base
	for i := 1; ; i++ {
		err := copyFile(src, name)
		if !errors.Is(err, os.ErrExist) {
			return name, err
		}
		name = fmt.Sprintf("%s-%d", base, i)
	}
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	info, err := in.Stat()
	if err != nil {
		return err
	}
	out, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_EXCL, info.Mode().Perm())
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		os.Remove(dst)
		return err
	}
	if err := out.Sync(); err != nil {
		out.Close()
		os.Remove(dst)
		return err
	}
	return out.Close()
}

func inspectDB(ctx context.Context, path, shown string) (store.Info, error) {
	info, err := store.Inspect(ctx, path)
	switch {
	case errors.Is(err, store.ErrNotTeacherDB):
		return info, fmt.Errorf("%s is not a teacher database; nothing changed", shown)
	case errors.Is(err, store.ErrNewerSchema):
		return info, fmt.Errorf("%s has schema version %d; this teacher reads up to %d (upgrade teacher here first); nothing changed",
			shown, info.Version, store.SchemaVersion())
	case errors.Is(err, store.ErrIntegrity):
		return info, fmt.Errorf("%s failed the integrity check; nothing changed", shown)
	case err != nil:
		return info, fmt.Errorf("%s: %w; nothing changed", shown, err)
	}
	return info, nil
}

func newerActivity(dst string, dstAt store.Activity, src string, srcAt store.Activity) error {
	return fmt.Errorf("%s has newer study activity (%s) than %s (%s); nothing changed (use --force to replace it anyway)",
		dst, dstAt, src, srcAt)
}

func inUse(dbPath string) (bool, error) {
	for _, suffix := range []string{"-journal", "-wal"} {
		ok, err := fileExists(dbPath + suffix)
		if ok || err != nil {
			return ok, err
		}
	}
	return false, nil
}

func fileExists(p string) (bool, error) {
	_, err := os.Stat(p)
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	return err == nil, err
}
