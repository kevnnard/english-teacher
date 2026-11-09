# english-teacher

A personal English teacher in the terminal, written in Go.

## Usage

```sh
go build ./cmd/teacher

# Import (or re-import) the whole english-master progress folder:
# vocabulary-bank.md, plus level.md, streak.md and daily-log.md when present.
# Re-importing never duplicates cards or days, never resets a review schedule,
# and keeps the name and goal you set. A single bank file also works.
./teacher import ~/Projects/english-master/progress
./teacher import ~/Projects/english-master/progress/vocabulary-bank.md

# Your name (Home greets you with it) and your daily goal in minutes (default 20).
./teacher set name Kevin
./teacher set goal 25

# Open the app on Home, or straight on today's review.
./teacher
./teacher review

# At most 5 never-reviewed words in this session (default 20).
./teacher --new 5 review
```

The database lives at `$XDG_DATA_HOME/english-teacher/teacher.db`
(`~/.local/share/english-teacher/teacher.db` if unset). Use `--db path` before
the command to pick another file, e.g. `./teacher --db /tmp/test.db review`.

## The app

Screens: **Home** (one preselected next action), **Review** (cards due today),
**Words** (every word with its status), **Stats**, and **Learn** / **Write**
(coming in M3). Every screen is a notebook page: the top bar shows the date,
the page ("Page 3 of 26" during a review) and the screen tabs; a margin line
runs down the left (6 columns in from the edge on terminals 100+ columns
wide, 2 on narrower ones) and the text never gets wider than 76 columns.

The bottom row is the status bar: your level, cards still due, this
session's ✓/✗ and your streak on the left, the keys for the current screen on
the right.

Home shows today's study time against your daily goal and the last seven days
(`●` studied, `○` not). Stats adds your current and longest streak and a row per
day with minutes, cards and ✓/✗. A day counts for the streak when you grade at
least one card or finish an activity; time alone does not. The streak survives
until the end of the day after your last study day. Study time is the time
between grades, at most 2 minutes per card, so leaving the app open does not
count.
Word status is always a glyph plus a word: `● due`, `◐ learning`, `✓ known`, `○ new`.

### Keys

The `?` overlay shows the same list.

| Where | Key | Action |
|---|---|---|
| everywhere | `tab` / `shift+tab` | next / previous screen |
| | `:` | command palette: type a screen name, `enter` to go |
| | `?` | show / hide help |
| | `c` | show / hide the margin notes during review |
| | `esc` | back to Home |
| | `q` | quit on Home, back to Home elsewhere |
| | `ctrl+c` | quit |
| Home | `enter` | start the preselected action |
| Review | `space` | reveal the answer (English → Spanish) |
| | `1`–`4` | grade your recall: 1 again · 2 hard · 3 good · 4 easy |
| | `enter` | check a typed answer (Spanish → English) |
| | `u` / `ctrl+z` | undo the last grade (`ctrl+z` also works while typing an answer) |
| | `r` | review the session's mistakes again (dates don't move) |
| Words | `/` | filter by word or meaning (`enter` keep · `esc` clear) |
| | `↑` `↓` / `j` `k` | scroll |

### Review

Every word is studied in two directions, each with its own SM-2 schedule
and ✓/✗ counters:

- **Recognize** (English → Spanish): see the word, `space` to reveal the
  meaning, then grade yourself 1–4.
- **Produce** (Spanish → English): read the meaning and type the English.
  It starts only once you have recognized the word **twice in a row**
  (recognition repetitions ≥ 2): recognizing comes before producing.

A typed answer is compared ignoring case, surrounding spaces and trailing
punctuation:

| Answer | Shown | Grades allowed |
|---|---|---|
| exact | ✓, graded *good* automatically | `4` upgrades to *easy*, `enter` next |
| 1–2 letters off | ✗ almost, with a letter diff: `nice to me[a→e]t you` | `1` again · `2` hard |
| anything else | ✗ and the right answer | `1` again |

The grades map to SM-2 as again 1 · hard 3 · good 4 · easy 5, so "hard"
still passes and "again" brings the card back tomorrow. Answers are saved as
soon as you grade them; `u` takes the last grade back (in the database too)
until you grade the next card. In a typing prompt every key is a letter, so
`u` there types a "u".

A session is every card due today plus at most 20 new words (`--new`). It
ends with a summary page: how many you reviewed and how long it took, up to
three words to watch, and when the next review is. `r` replays the session's
mistakes for practice without moving any review date. The margin notes at the
foot of each card show your history with it: times seen, ✓/✗, when it comes
back if you grade it good, and the last wrong answer you typed.

## Roadmap

- **M1** — Vocabulary review TUI with spaced repetition (SM-2), importing the existing vocabulary bank. No AI.
- **M2** — Streak, CEFR level, daily goal and daily log in SQLite. Done.
- **M3** — LLM teacher: lesson of the day and homework review, with one model per capability (text, speech, resource search).
- **M4** — Free conversation practice and quizzes.
- **M5** — Voice: speech-to-text and pronunciation practice.

## Layout

```
cmd/teacher/        CLI entry point
internal/domain/    pure types: Card, Grade, Answer, Direction
internal/srs/       SM-2 scheduler and session planning (pure, no I/O)
internal/check/     typed-answer check and letter diff (pure)
internal/progress/  streak, week and daily totals (pure, no I/O)
internal/importer/  imports english-master markdown data
internal/store/     SQLite persistence
internal/llm/       model clients (M3)
internal/teacher/   use cases (M3)
internal/tui/       Bubbletea screens
```
