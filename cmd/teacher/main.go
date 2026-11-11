// Command teacher is a personal English teacher in the terminal.
//
// Usage:
//
//	teacher [--db path] [--new n]                        open the app on Home
//	teacher [--db path] [--new n] review                 open the app on Review
//	teacher [--db path] import <vocabulary-bank.md>      import the vocabulary bank
//	teacher [--db path] import <progress-folder>         import bank, level and history
//	teacher [--db path] set name <name>                  set the learner's name
//	teacher [--db path] set goal <minutes>               set the daily goal
//	teacher [--config path] models                       list the model of each capability
//	teacher [--config path] set model <capability> <m>   set the model of one capability
//	teacher setup voice                                  download the default voice for speech
//	teacher [--config path] doctor                       check pi, piper, the voice, a player and whisper
//	teacher --version                                    print the version
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/kevnnard/english-teacher/internal/domain"
	"github.com/kevnnard/english-teacher/internal/importer"
	"github.com/kevnnard/english-teacher/internal/store"
	"github.com/kevnnard/english-teacher/internal/teacher"
	"github.com/kevnnard/english-teacher/internal/tui"
)

const usage = `usage:
  teacher [--db path] [--new n]                    open the app on Home
  teacher [--db path] [--new n] review             open the app on Review
  teacher [--db path] import <vocabulary-bank.md>  import the vocabulary bank
  teacher [--db path] import <progress-folder>     import the bank, level.md and history
  teacher [--db path] set name <name>              set your name
  teacher [--db path] set goal <minutes>           set the daily goal (default 20)
  teacher models                                   list the AI model of each capability
  teacher set model <capability> <provider/model>  reading, writing, conversation or quiz
  teacher setup voice                              download the default voice for speech
  teacher doctor                                   check pi, piper, the voice, a player and whisper
  teacher --version                                print the version

  --new n  most never-reviewed cards per session (default 20)`

// version is the released version of teacher, printed by --version.
// Release builds set it with -ldflags "-X main.version=X.Y.Z"; the default
// is the current release, so `go install` builds print it too.
var version = "0.1.0"

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
	showVersion := fs.Bool("version", false, "print the version and exit")
	configPath := fs.String("config", defaultConfigPath(os.Getenv("XDG_CONFIG_HOME"), home), "path to config.json")
	if err := fs.Parse(args); err != nil {
		return fmt.Errorf("%w\n%s", err, usage)
	}
	if *maxNew < 0 {
		return fmt.Errorf("--new must be 0 or more, got %d", *maxNew)
	}

	if *showVersion {
		fmt.Fprintln(stdout, "teacher", version)
		return nil
	}

	opts := appOptions{dbPath: *dbPath, configPath: *configPath, start: tui.Home, maxNew: *maxNew}
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
	case "set":
		if len(rest) > 0 && rest[0] == "model" {
			return setModel(*configPath, rest[1:], stdout)
		}
		return runSet(*dbPath, rest, stdout)
	case "models":
		if len(rest) != 0 {
			return errors.New(usage)
		}
		return runModels(*configPath, stdout)
	case "setup":
		return runSetup(rest, stdout)
	case "doctor":
		if len(rest) != 0 {
			return errors.New(usage)
		}
		return runDoctor(*configPath, stdout)
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

// runImport imports a vocabulary bank file, or a whole english-master
// progress folder: its vocabulary-bank.md (required) plus level.md,
// streak.md and daily-log.md when they exist. Importing again is safe.
func runImport(dbPath, path string, stdout io.Writer) error {
	info, err := os.Stat(path)
	if err != nil {
		return err
	}
	bankPath, folder := path, ""
	if info.IsDir() {
		folder, bankPath = path, filepath.Join(path, "vocabulary-bank.md")
		if _, err := os.Stat(bankPath); err != nil {
			return fmt.Errorf("%s has no vocabulary-bank.md", path)
		}
	}

	st, err := openStore(dbPath)
	if err != nil {
		return err
	}
	defer st.Close()

	ctx := context.Background()
	if err := importBank(ctx, st, bankPath, stdout); err != nil {
		return err
	}
	if folder == "" {
		return nil
	}
	if err := importLevel(ctx, st, filepath.Join(folder, "level.md"), stdout); err != nil {
		return err
	}
	return importHistory(ctx, st, folder, stdout)
}

func importBank(ctx context.Context, st *store.Store, bankPath string, stdout io.Writer) error {
	f, err := os.Open(bankPath)
	if err != nil {
		return err
	}
	defer f.Close()

	cards, err := importer.ParseVocabularyBank(f)
	if err != nil {
		return fmt.Errorf("%s: %w", bankPath, err)
	}
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

// importLevel copies the level and start date of level.md into the
// profile, keeping the name and goal the learner set in the app. A file
// without a start date clears it: the file is the source of truth.
func importLevel(ctx context.Context, st *store.Store, path string, stdout io.Writer) error {
	f, err := os.Open(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	defer f.Close()

	level, started, err := importer.ParseLevelFile(f)
	if err != nil {
		return fmt.Errorf("%s: %w", path, err)
	}
	p, err := st.Profile(ctx)
	if err != nil {
		return err
	}
	p.Level, p.Started = level, started // level.md is the source of both
	if err := st.SaveProfile(ctx, p); err != nil {
		return err
	}
	fmt.Fprintf(stdout, "Level: %s (%s)", level, level.Name())
	if p.Started != (domain.Date{}) {
		fmt.Fprintf(stdout, ", started %s", p.Started)
	}
	fmt.Fprintln(stdout)
	return nil
}

// importHistory merges the study days of streak.md and daily-log.md.
func importHistory(ctx context.Context, st *store.Store, folder string, stdout io.Writer) error {
	parsers := []struct {
		name  string
		parse func(io.Reader) ([]domain.Day, error)
	}{
		{"streak.md", importer.ParseStreakHistory},
		{"daily-log.md", importer.ParseDailyLog},
	}
	var history []domain.Day
	found := false
	for _, p := range parsers {
		path := filepath.Join(folder, p.name)
		f, err := os.Open(path)
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return err
		}
		days, err := p.parse(f)
		f.Close()
		if err != nil {
			return fmt.Errorf("%s: %w", path, err)
		}
		history = append(history, days...)
		found = true
	}
	if !found {
		return nil
	}
	before, err := st.Days(ctx)
	if err != nil {
		return err
	}
	if _, err := st.MergeDays(ctx, history); err != nil {
		return err
	}
	after, err := st.Days(ctx)
	if err != nil {
		return err
	}
	n := len(after) - len(before)
	unit := "days"
	if n == 1 {
		unit = "day"
	}
	fmt.Fprintf(stdout, "History: %d new study %s\n", n, unit)
	return nil
}

// maxGoalMinutes caps the daily goal at four hours.
const maxGoalMinutes = 240

// runSet changes one profile setting: "name <name...>" or "goal <minutes>".
// Input is validated before the database is opened, so a bad value
// changes nothing.
func runSet(dbPath string, args []string, stdout io.Writer) error {
	if len(args) < 2 {
		return errors.New(usage)
	}
	key, value := args[0], strings.TrimSpace(strings.Join(args[1:], " "))

	var (
		apply func(*domain.Profile)
		done  string
	)
	switch key {
	case "name":
		apply = func(p *domain.Profile) { p.Name = value }
		done = fmt.Sprintf("Name set to %q", value)
	case "goal":
		n, err := strconv.Atoi(value)
		if err != nil || n < 1 || n > maxGoalMinutes {
			return fmt.Errorf("goal must be between 1 and %d minutes, got %q", maxGoalMinutes, value)
		}
		apply = func(p *domain.Profile) { p.DailyGoalMinutes = n }
		done = fmt.Sprintf("Daily goal set to %d min", n)
	default:
		return fmt.Errorf("unknown setting %q (want name or goal)\n%s", key, usage)
	}

	st, err := openStore(dbPath)
	if err != nil {
		return err
	}
	defer st.Close()

	ctx := context.Background()
	p, err := st.Profile(ctx)
	if err != nil {
		return err
	}
	apply(&p)
	if err := st.SaveProfile(ctx, p); err != nil {
		return err
	}
	fmt.Fprintln(stdout, done)
	return nil
}

// appOptions is how the app was asked to open.
type appOptions struct {
	dbPath     string
	configPath string
	start      tui.Screen
	maxNew     int
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

// appConfig loads what the app needs from the store and wires saving,
// study recording and the writing error log back to it.
func appConfig(ctx context.Context, st *store.Store, opts appOptions) (tui.Config, error) {
	cards, err := loadCards(ctx, st)
	if err != nil {
		return tui.Config{}, err
	}
	profile, err := st.Profile(ctx)
	if err != nil {
		return tui.Config{}, err
	}
	days, err := st.Days(ctx)
	if err != nil {
		return tui.Config{}, err
	}
	mistakes, err := st.MistakeCounts(ctx)
	if err != nil {
		return tui.Config{}, err
	}
	conf, err := loadConfig(opts.configPath)
	if err != nil {
		return tui.Config{}, err
	}
	return tui.Config{
		Cards:   cards,
		Save:    func(c domain.Card) error { return st.SaveCard(ctx, c) },
		Now:     time.Now,
		Start:   opts.start,
		MaxNew:  opts.maxNew,
		Profile: profile,
		Days:    days,
		Record:  func(d domain.Day) error { return st.RecordStudy(ctx, d) },
		Teacher: teacher.New(conf.models()),
		Speaker: conf.speaker(),
		Add:     func(c []domain.Card) (int, error) { return st.AddCards(ctx, c) },

		MistakeCounts:  mistakes,
		RecordMistakes: func(m []domain.Mistake) error { return st.RecordMistakes(ctx, m) },
	}, nil
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

	cfg, err := appConfig(context.Background(), st, opts)
	if err != nil {
		return err
	}
	app := tui.NewApp(cfg)
	final, err := tea.NewProgram(app).Run()
	if err != nil {
		return err
	}
	return final.(tui.App).Err()
}
