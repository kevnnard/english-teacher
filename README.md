# english-teacher

A personal English teacher in the terminal, written in Go.

## Usage

```sh
go build ./cmd/teacher

# Import (or re-import) the english-master vocabulary bank.
# Re-importing never duplicates cards or resets your review schedule.
./teacher import ~/Projects/english-master/progress/vocabulary-bank.md

# Open the app on Home, or straight on today's review.
./teacher
./teacher review
```

The database lives at `$XDG_DATA_HOME/english-teacher/teacher.db`
(`~/.local/share/english-teacher/teacher.db` if unset). Use `--db path` before
the command to pick another file, e.g. `./teacher --db /tmp/test.db review`.

## The app

Screens: **Home** (one preselected next action), **Review** (cards due today),
**Words** (every word with its status), **Stats**, and **Learn** / **Write**
(coming in M3). The layout follows the terminal width:

| Width | Layout |
|---|---|
| < 100 columns | one pane, progress line on top |
| 100–139 columns | navigation rail + content |
| ≥ 140 columns | rail + content + coach panel |

The bottom row is the status bar: your level, cards still due and this
session's ✓/✗ on the left, the keys for the current screen on the right.
Word status is always a glyph plus a word: `● due`, `◐ learning`, `✓ known`, `○ new`.

### Keys

The `?` overlay shows the same list.

| Where | Key | Action |
|---|---|---|
| everywhere | `tab` / `shift+tab` | next / previous screen |
| | `:` | command palette: type a screen name, `enter` to go |
| | `?` | show / hide help |
| | `c` | show / hide the coach (140+ columns) |
| | `esc` | back to Home |
| | `q` | quit on Home, back to Home elsewhere |
| | `ctrl+c` | quit |
| Home | `enter` | start the preselected action |
| Review | `space` | reveal the answer |
| | `0`–`5` | grade your recall: 0-2 forgot · 3 hard · 4 good · 5 easy |
| | `enter` | back to Home when the session is done |
| Words | `/` | filter by word or meaning (`enter` keep · `esc` clear) |
| | `↑` `↓` / `j` `k` | scroll |

Cards are scheduled with SM-2: a failed card comes back tomorrow. Answers are
saved as soon as you grade them.

## Roadmap

- **M1** — Vocabulary review TUI with spaced repetition (SM-2), importing the existing vocabulary bank. No AI.
- **M2** — Streak, CEFR level and daily log in SQLite.
- **M3** — LLM teacher: lesson of the day and homework review, with one model per capability (text, speech, resource search).
- **M4** — Free conversation practice and quizzes.
- **M5** — Voice: speech-to-text and pronunciation practice.

## Layout

```
cmd/teacher/        CLI entry point
internal/domain/    pure types: Card, Grade
internal/srs/       SM-2 scheduler (pure, no I/O)
internal/importer/  imports english-master markdown data
internal/store/     SQLite persistence
internal/llm/       model clients (M3)
internal/teacher/   use cases (M3)
internal/tui/       Bubbletea screens
```
