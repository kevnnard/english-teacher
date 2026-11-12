# Changelog

## 0.5.0 — 2026-11-12

Each learner uses their own AI subscription. No database migration.

### AI models
- New `agy` provider: a model `agy/<model-id>` runs through the `agy` command
  of Antigravity, with the learner's own login. The app runs it with no tools
  in an empty temporary folder and never reads or stores credentials. An
  `"agy"` config key sets its path (default `agy` on the PATH).
- `teacher setup ai` lists the models of the learner's account (`agy models`)
  and asks which one to use for reading, writing, conversation and quiz,
  suggesting a strong one for the first three and a fast one for quiz
  (`enter` keeps it). It saves them as `agy/<model-id>` and prints them. When
  `agy` is missing or not logged in it says how to set it up and saves
  nothing.
- No model is set by default any more. A lesson whose capability has no model
  says `no AI model set for <capability>: run "teacher setup ai"`, and
  `teacher models` shows `(not set)`. Models of any other provider keep going
  through `pi`, so existing configs keep working.
- `.gitignore` also ignores `.env` files, `auth*.json`, `oauth*.json` and a
  repo-local `config.json`.

## 0.4.0 — 2026-11-11

Sync the database between two machines over ssh. No database migration.

### Sync
- `teacher sync push [user@]host` and `teacher sync pull [user@]host` copy
  the whole database to or from another machine over the system `ssh`;
  `--remote-db path` sets the database there (default
  `~/.local/share/english-teacher/teacher.db`). Close teacher on both
  machines before syncing.
- It sends a consistent snapshot, checks both databases on a temporary copy
  (integrity, the expected tables and columns, an upgrade and a read) before
  replacing anything, and replaces with a final atomic rename. Before that
  rename a failure changes nothing; a connection lost during it can leave the
  new database in place, and the error says so and names the backup.
- The old database is kept as `teacher.db.bak-<timestamp>`; a backup is never
  overwritten (a second sync in the same second adds `-1`, `-2`, ...).
- It refuses to replace a database with newer study activity (showing both
  dates) unless `--force`. It always refuses a malformed database and a
  database, on either side, with a schema newer than this teacher reads.

## 0.3.0 — 2026-11-11

More input, a level that adapts, grammar in context, progress you can see,
and prosody. Opening this version upgrades the database to migration 7;
existing data is kept.

### Input
- Stories in Learn: stories at your level in chapters of about 250–400
  words, mostly known words. The next chapter continues the story; each
  chapter has one comprehension question; recorded as "Reading: <story> ch N".
- Words read are counted per week.
- Long listening in Listen: a 2–5 minute dialogue or story read by Piper,
  heard first without text, then 3–5 questions, then the transcript
  sentence by sentence; recorded as "Listening: N/M".

### Level
- The Level screen shows each readiness criterion for the next CEFR level
  and its current value: known words, Write average, grammar quiz,
  Listen/Pairs accuracy and days at the level.
- When every criterion is met, Home offers a short level check (reading,
  writing and grammar at the next level); passing it raises the level.
- `teacher set level <A1..C2>` sets the level by hand. The level never
  rises any other way.

### Grammar in context
- Quiz has a cloze mode: gaps in sentences from your own texts and cards,
  mixing at least 3 grammar points per round.
- Chunks such as "I'm looking for…" become cards in Review (`a` on the
  feedback; missed chunks are added at the end).

### Progress
- Progress page (`g` on Home): this week against last week (study time,
  new words, words learned, words read, Write average, top errors and
  whether they are falling) and the CEFR "can do" goals of your level,
  ticked by the activities that prove them.
- Words added or learned before 0.3.0 have no date, so no week counts them.

### Prosody
- Pairs has two more modes (`m`): word stress (hear a word, pick the
  stressed syllable: PHO·to·graph / pho·TOG·ra·phy) and intonation (hear a
  sentence, choose question ↗ or statement ↘). Results are stored like
  pair stats, and the weakest drills come first.

### Data
- Migration 6 adds stories and words read per day; migration 7 adds the
  dates a word was added and became known, and the can-do goal ticks.

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
