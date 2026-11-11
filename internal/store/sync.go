package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/kevnnard/english-teacher/internal/domain"
)

var (
	ErrNotTeacherDB = errors.New("not a teacher database")
	ErrIntegrity    = errors.New("integrity check failed")
	ErrNewerSchema  = errors.New("newer schema than this teacher reads")
)

func SchemaVersion() int {
	return len(migrations)
}

type Activity struct {
	Day domain.Date
	At  time.Time
}

func (a Activity) After(b Activity) bool {
	if a.Day != b.Day {
		return b.Day.Before(a.Day)
	}
	return a.At.After(b.At)
}

func (a Activity) String() string {
	if a.Day == (domain.Date{}) {
		return "none"
	}
	if !a.At.IsZero() && domain.DateOf(a.At) == a.Day {
		return a.At.Format("2006-01-02 15:04")
	}
	return a.Day.String()
}

type Info struct {
	Version int
	Latest  Activity
}

func Inspect(ctx context.Context, path string) (Info, error) {
	if _, err := os.Stat(path); err != nil {
		return Info{}, err
	}
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return Info{}, err
	}
	defer db.Close()
	conn, err := db.Conn(ctx)
	if err != nil {
		return Info{}, err
	}
	defer conn.Close()

	if _, err := conn.ExecContext(ctx, `PRAGMA query_only = ON`); err != nil {
		return Info{}, ErrNotTeacherDB
	}
	var check string
	if err := conn.QueryRowContext(ctx, `PRAGMA integrity_check`).Scan(&check); err != nil {
		return Info{}, ErrNotTeacherDB
	}
	if check != "ok" {
		return Info{}, ErrIntegrity
	}
	var info Info
	if err := conn.QueryRowContext(ctx, `PRAGMA user_version`).Scan(&info.Version); err != nil {
		return Info{}, ErrNotTeacherDB
	}
	tables, err := tableNames(ctx, conn)
	if err != nil {
		return Info{}, ErrNotTeacherDB
	}
	if info.Version < 1 || !tables["cards"] {
		return Info{}, ErrNotTeacherDB
	}
	if info.Version > SchemaVersion() {
		return info, ErrNewerSchema
	}
	if err := validate(ctx, path, info.Version); err != nil {
		return Info{Version: info.Version}, err
	}
	info.Latest, err = latestActivity(ctx, conn, tables)
	return info, err
}

func Snapshot(ctx context.Context, src, dst string) error {
	if _, err := os.Stat(src); err != nil {
		return err
	}
	db, err := sql.Open("sqlite", src)
	if err != nil {
		return err
	}
	defer db.Close()
	_, err = db.ExecContext(ctx, `VACUUM INTO ?`, dst)
	return err
}

func tableNames(ctx context.Context, conn *sql.Conn) (map[string]bool, error) {
	rows, err := conn.QueryContext(ctx, `SELECT name FROM sqlite_master WHERE type = 'table'`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	tables := map[string]bool{}
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, err
		}
		tables[name] = true
	}
	return tables, rows.Err()
}

func latestActivity(ctx context.Context, conn *sql.Conn, tables map[string]bool) (Activity, error) {
	var a Activity
	if tables["days"] {
		var day sql.NullString
		if err := conn.QueryRowContext(ctx, `SELECT MAX(date) FROM days`).Scan(&day); err != nil {
			return a, err
		}
		if day.Valid {
			d, err := domain.ParseDate(day.String)
			if err != nil {
				return a, fmt.Errorf("days: %w", err)
			}
			a.Day = d
		}
	}
	if tables["reviews"] {
		rows, err := conn.QueryContext(ctx,
			`SELECT last_reviewed FROM reviews WHERE last_reviewed IS NOT NULL AND last_reviewed <> ''`)
		if err != nil {
			return a, err
		}
		defer rows.Close()
		for rows.Next() {
			var s string
			if err := rows.Scan(&s); err != nil {
				return a, err
			}
			t, err := parseTime(s)
			if err != nil {
				return a, fmt.Errorf("reviews: last_reviewed: %w", err)
			}
			if t.After(a.At) {
				a.At = t
			}
		}
		if err := rows.Err(); err != nil {
			return a, err
		}
	}
	if !a.At.IsZero() && a.Day.Before(domain.DateOf(a.At)) {
		a.Day = domain.DateOf(a.At)
	}
	return a, nil
}

func validate(ctx context.Context, path string, version int) error {
	dir, err := os.MkdirTemp("", "teacher-inspect-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(dir)
	check := filepath.Join(dir, "check.db")
	if err := copyExclusive(path, check); err != nil {
		return err
	}
	if err := sameColumns(ctx, check, filepath.Join(dir, "then.db"), version); err != nil {
		return err
	}
	got, err := Open(check)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrNotTeacherDB, err)
	}
	defer got.Close()
	if err := sameColumns(ctx, check, filepath.Join(dir, "now.db"), len(migrations)); err != nil {
		return err
	}
	for _, dir := range []domain.Direction{domain.Recognition, domain.Production} {
		if _, err := got.ListCards(ctx, dir); err != nil {
			return fmt.Errorf("%w: %v", ErrNotTeacherDB, err)
		}
	}
	if _, err := got.Days(ctx); err != nil {
		return fmt.Errorf("%w: %v", ErrNotTeacherDB, err)
	}
	return nil
}

func sameColumns(ctx context.Context, path, refPath string, version int) error {
	ref, err := sql.Open("sqlite", refPath)
	if err != nil {
		return err
	}
	defer ref.Close()
	ref.SetMaxOpenConns(1)
	if err := (&Store{db: ref}).migrateTo(version); err != nil {
		return err
	}
	want, err := columns(ctx, ref)
	if err != nil {
		return err
	}
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return err
	}
	defer db.Close()
	got, err := columns(ctx, db)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrNotTeacherDB, err)
	}
	for col := range want {
		if !got[col] {
			return fmt.Errorf("%w: missing %s", ErrNotTeacherDB, col)
		}
	}
	return nil
}

func columns(ctx context.Context, db *sql.DB) (map[string]bool, error) {
	rows, err := db.QueryContext(ctx,
		`SELECT m.name, c.name FROM sqlite_master m, pragma_table_info(m.name) c WHERE m.type = 'table'`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	cols := map[string]bool{}
	for rows.Next() {
		var table, col string
		if err := rows.Scan(&table, &col); err != nil {
			return nil, err
		}
		cols[table+"."+col] = true
	}
	return cols, rows.Err()
}

func copyExclusive(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		return err
	}
	return out.Close()
}
