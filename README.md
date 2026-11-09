# english-teacher

A personal English teacher in the terminal, written in Go.

## Usage

```sh
go build ./cmd/teacher

# Import (or re-import) the english-master vocabulary bank.
# Re-importing never duplicates cards or resets your review schedule.
./teacher import ~/Projects/english-master/progress/vocabulary-bank.md

# Review the cards due today.
./teacher review
```

In `review`, each card shows the word and its pronunciation first:

| Key | Action |
|---|---|
| `space` | show the meaning and an example |
| `0`–`5` | grade your recall: 0-2 forgot · 3 hard · 4 good · 5 easy |
| `q` | quit (answers already given are saved) |

Cards are scheduled with SM-2: a grade below 3 brings the card back tomorrow.

The database lives at `$XDG_DATA_HOME/english-teacher/teacher.db`
(`~/.local/share/english-teacher/teacher.db` if unset). Use `--db path` before
the command to pick another file, e.g. `./teacher --db /tmp/test.db review`.

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
