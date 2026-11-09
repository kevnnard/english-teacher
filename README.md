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

# At most 5 never-reviewed words in this session (default 20).
./teacher --new 5 review
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
| Review | `space` | reveal the answer (English → Spanish) |
| | `1`–`4` | grade your recall: 1 again · 2 hard · 3 good · 4 easy |
| | `enter` | check a typed answer (Spanish → English) |
| | `u` | undo the last grade |
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
ends with a summary: how many you reviewed and how long it took, up to three
tricky words, and when the next review is. `r` replays the session's mistakes
for practice without moving any review date. On wide terminals the coach
panel shows your history with the card on screen: times seen, ✓/✗ and the
last wrong answer you typed.

## Roadmap

- **M1** — Vocabulary review TUI with spaced repetition (SM-2), importing the existing vocabulary bank. No AI.
- **M2** — Streak, CEFR level and daily log in SQLite.
- **M3** — LLM teacher: lesson of the day and homework review, with one model per capability (text, speech, resource search).
- **M4** — Free conversation practice and quizzes.
- **M5** — Voice: speech-to-text and pronunciation practice.

## Layout

```
cmd/teacher/        CLI entry point
internal/domain/    pure types: Card, Grade, Answer, Direction
internal/srs/       SM-2 scheduler and session planning (pure, no I/O)
internal/check/     typed-answer check and letter diff (pure)
internal/importer/  imports english-master markdown data
internal/store/     SQLite persistence
internal/llm/       model clients (M3)
internal/teacher/   use cases (M3)
internal/tui/       Bubbletea screens
```
