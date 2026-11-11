package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/kevnnard/english-teacher/internal/domain"
)

var (
	ErrNotTeacherDB = errors.New("not a teacher database")
	ErrIntegrity    = errors.New("integrity check failed")
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
		return info, nil
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
