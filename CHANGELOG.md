# Changelog

## 0.2.0 — 2026-11-10

Voice. Everything runs on your machine and is optional: without Piper or
whisper the app works as in 0.1.0 and tells you how to set audio up.

### Listening
- `p` reads aloud in Review (the word, then the example), Learn (the
  selected word; `P` the whole text), Write feedback and Quiz answers.
  Pressing it twice quickly repeats it slower. Speech uses Piper.
- Listen: dictation of up to 10 sentences from your own cards' examples,
  checked like a typed answer; recorded as "Listen: N/10".
- Pairs: minimal pairs for Spanish speakers (ship/sheep, full/fool,
  cat/cut, b/v, think/sink, they/day, work/worked, state/estate). Per-contrast
  accuracy is stored and the weakest contrasts come first; "Pairs: N/20".

### Speaking
- Speak: hear a sentence, record yourself, and whisper.cpp marks which words
  it understood. A changed vowel or a dropped ending is never forgiven;
  sentences with missed words come back once at the end of the round.
- Talk by voice: `ctrl+r` records your line into the input (you check it,
  then send it), the tutor's replies are read aloud (`ctrl+t` voice mode,
  `ctrl+p` replay).

### Setup
- `teacher setup voice` and `teacher setup whisper` download the models;
  `teacher doctor` shows pi, piper, the voice, a player, whisper, its model
  and a recorder.
- Database migration 5 adds the `pair_stats` table; existing data is kept.

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
