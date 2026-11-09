# english-teacher

A personal English teacher in the terminal, written in Go.

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
