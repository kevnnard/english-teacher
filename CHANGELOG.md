# Changelog

## 0.1.0 — 2026-11-10

The first release: every text feature of the English teacher, covering
Nation's four strands (meaning-focused input and output, language-focused
learning, fluency). Voice comes in 0.2.0.

### Review (M1)
- Vocabulary review with spaced repetition (SM-2), grades 1–4, both
  directions per word, typed answers with a letter diff, and undo.
- `teacher import` for the vocabulary bank and the progress folder.

### Progress (M2)
- Streak, CEFR level, daily goal (default 20 min) and a daily log in SQLite.
- Home shows the streak only from real data, with guidance when it is zero.
- `teacher set name` and `teacher set goal`.

### AI teacher (M3)
- AI calls go through the `pi` CLI, with one configurable model per
  capability: `teacher models` and `teacher set model`.
- Learn: graded reading with at most 3 new words; marked words become cards.
- Write: six steps — plan with useful phrases, write, correct your own
  marked errors, focused feedback (at most 3 major errors), rewrite from
  memory, and a /40 rubric with a grade letter. Major errors become cards and
  are logged by kind, so recurring errors shape the next task.

### Practice (M4)
- Talk: four guided A1 role-plays with sentence frames that fade with level,
  in-character replies with gentle recasts, and a feedback summary.
- Fluency: written 4/3/2 — the same story in 4, 3 and 2 minutes, with words
  per minute for each round.
- Quiz: vocabulary quizzes from your own cards (multiple choice and typed)
  and AI grammar quizzes on your recurring errors or the level's core grammar.

### Storage
- SQLite (pure Go, no cgo), migrated in place up to schema version 4.
