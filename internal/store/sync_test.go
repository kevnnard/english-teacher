package store

import (
	"bytes"
	"context"
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func makeAtVersion(t *testing.T, path string, version int) {
	t.Helper()
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	db.SetMaxOpenConns(1)
	if err := (&Store{db: db}).migrateTo(version); err != nil {
		t.Fatal(err)
	}
}

func TestInspectAcceptsEveryGenuineOlderVersionWithoutChangingIt(t *testing.T) {
	for v := 1; v <= SchemaVersion(); v++ {
		path := filepath.Join(t.TempDir(), "teacher.db")
		makeAtVersion(t, path, v)
		before, _ := os.ReadFile(path)

		info, err := Inspect(context.Background(), path)

		if err != nil || info.Version != v {
			t.Errorf("version %d: Inspect = %+v, %v", v, info, err)
		}
		if after, _ := os.ReadFile(path); !bytes.Equal(before, after) {
			t.Errorf("version %d: Inspect changed the file", v)
		}
	}
}

func TestInspectRefusesASchemaThatDoesNotMatchItsVersion(t *testing.T) {
	path := filepath.Join(t.TempDir(), "teacher.db")
	makeAtVersion(t, path, SchemaVersion())
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`PRAGMA user_version = 5`); err != nil {
		t.Fatal(err)
	}
	db.Close()
	before, _ := os.ReadFile(path)

	info, err := Inspect(context.Background(), path)

	if !errors.Is(err, ErrNotTeacherDB) || info.Version != 5 {
		t.Errorf("Inspect = %+v, %v; want version 5 and ErrNotTeacherDB", info, err)
	}
	if after, _ := os.ReadFile(path); !bytes.Equal(before, after) {
		t.Error("Inspect changed the file")
	}
}
