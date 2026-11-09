// Command teacher is a personal English teacher in the terminal.
//
// Usage:
//
//	teacher [--db path] [--new n]                        open the app on Home
//	teacher [--db path] [--new n] review                 open the app on Review
//	teacher [--db path] import <vocabulary-bank.md>      import the vocabulary bank
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/kevnnard/english-teacher/internal/domain"
	"github.com/kevnnard/english-teacher/internal/importer"
	"github.com/kevnnard/english-teacher/internal/store"
	"github.com/kevnnard/english-teacher/internal/tui"
)

const usage = `usage:
  teacher [--db path] [--new n]                    open the app on Home
  teacher [--db path] [--new n] review             open the app on Review
  teacher [--db path] import <vocabulary-bank.md>  import the vocabulary bank

  --new n  most never-reviewed cards per session (default 20)`

func main() {
	if err := run(os.Args[1:], os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, "teacher:", err)
		os.Exit(1)
	}
}

// run parses args and executes one subcommand, writing messages to stdout.
func run(args []string, stdout io.Writer) error {
	home, _ := os.UserHomeDir()

	fs := flag.NewFlagSet("teacher", flag.ContinueOnError)
	fs.SetOutput(io.Discard) // errors are returned, not printed twice
	dbPath := fs.String("db", defaultDBPath(os.Getenv("XDG_DATA_HOME"), home), "path to the SQLite database")
	maxNew := fs.Int("new", tui.DefaultMaxNew, "most never-reviewed cards per session")
	if err := fs.Parse(args); err != nil {
		return fmt.Errorf("%w\n%s", err, usage)
	}
	if *maxNew < 0 {
		return fmt.Errorf("--new must be 0 or more, got %d", *maxNew)
	}

	opts := appOptions{dbPath: *dbPath, start: tui.Home, maxNew: *maxNew}
	if fs.NArg() == 0 {
		return startApp(opts)
	}
	cmd, rest := fs.Arg(0), fs.Args()[1:]

	switch cmd {
	case "import":
		if len(rest) != 1 {
			return errors.New(usage)
		}
		return runImport(*dbPath, rest[0], stdout)
	case "review":
		if len(rest) != 0 {
			return errors.New(usage)
		}
		opts.start = tui.Review
		return startApp(opts)
	default:
		return fmt.Errorf("unknown command %q\n%s", cmd, usage)
	}
}

// defaultDBPath follows the XDG spec: $XDG_DATA_HOME, else ~/.local/share.
func defaultDBPath(xdgData, home string) string {
	if xdgData == "" {
		xdgData = filepath.Join(home, ".local", "share")
	}
	return filepath.Join(xdgData, "english-teacher", "teacher.db")
}

// openStore creates the database directory if needed and opens the store.
func openStore(path string) (*store.Store, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, err
	}
	return store.Open(path)
}

func runImport(dbPath, bankPath string, stdout io.Writer) error {
	f, err := os.Open(bankPath)
	if err != nil {
		return err
	}
	defer f.Close()

	cards, err := importer.ParseVocabularyBank(f)
	if err != nil {
		return fmt.Errorf("%s: %w", bankPath, err)
	}

	st, err := openStore(dbPath)
	if err != nil {
		return err
	}
	defer st.Close()

	ctx := context.Background()
	res, err := st.UpsertCards(ctx, cards)
	if err != nil {
		return err
	}
	all, err := st.ListCards(ctx, domain.Recognition) // one card per word
	if err != nil {
		return err
	}
	fmt.Fprintf(stdout, "Imported %s: %d new, %d updated, %d total\n", bankPath, res.Added, res.Updated, len(all))
	return nil
}

// appOptions is how the app was asked to open.
type appOptions struct {
	dbPath string
	start  tui.Screen
	maxNew int
}

// loadCards reads every word in both directions: recognition first, which
// is the order the Words screen lists them in.
func loadCards(ctx context.Context, st *store.Store) ([]domain.Card, error) {
	var all []domain.Card
	for _, dir := range []domain.Direction{domain.Recognition, domain.Production} {
		cards, err := st.ListCards(ctx, dir)
		if err != nil {
			return nil, err
		}
		all = append(all, cards...)
	}
	return all, nil
}

// startApp runs the TUI. It is a variable so tests can check how run calls
// it without opening a terminal.
var startApp = runApp

func runApp(opts appOptions) error {
	st, err := openStore(opts.dbPath)
	if err != nil {
		return err
	}
	defer st.Close()

	ctx := context.Background()
	cards, err := loadCards(ctx, st)
	if err != nil {
		return err
	}

	app := tui.NewApp(tui.Config{
		Cards:  cards,
		Save:   func(c domain.Card) error { return st.SaveCard(ctx, c) },
		Now:    time.Now,
		Start:  opts.start,
		MaxNew: opts.maxNew,
	})
	final, err := tea.NewProgram(app).Run()
	if err != nil {
		return err
	}
	return final.(tui.App).Err()
}
