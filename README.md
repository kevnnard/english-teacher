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

# The AI model of each capability, and how to change one.
./teacher models
./teacher set model writing claude-bridge/claude-opus-5-5
```

The database lives at `$XDG_DATA_HOME/english-teacher/teacher.db`
(`~/.local/share/english-teacher/teacher.db` if unset). Use `--db path` before
the command to pick another file, e.g. `./teacher --db /tmp/test.db review`.

## The app

Screens: **Home** (one preselected next action), **Review** (cards due today),
**Learn** (a short reading written for you), **Write** (a writing task you
correct with the AI), **Talk** (a guided role-play with the AI), **Fluency** (the same story
three times, faster each time), **Quiz** (vocabulary from your cards or
grammar from the AI), **Words** (every word with its status) and **Stats**. Every screen is a notebook page: the top bar shows the date,
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

The `?` overlay shows the keys that work everywhere plus those of the
current screen.

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
| Learn | `enter` | get a short text written for you |
| | `↑` `↓` / `j` `k` | move through the glossary |
| | `space` | mark or unmark a word to study |
| | `a` | done: add the marked words as cards |
| | `n` | a new text |
| Write | `enter` | get a task; while writing, a new line |
| | `ctrl+d` | send your text to the teacher |
| | `enter` | fixing or rewriting: submit (empty: I don't know) |
| | `d` | show / hide the minor details in the feedback |
| | `s` | save the error cards again if saving failed |
| | `n` | a new task from the feedback or the result |
| | `esc` | back to Home; your work stays |
| Talk | `↑` `↓` / `j` `k` | choose a situation |
| | `enter` | start it; while talking, send your line (after a failed answer, the same line again) |
| | `ctrl+d` | finish the role-play early and get feedback |
| | `s` | save the error cards again if saving failed |
| | `n` | a new role-play from the feedback |
| | `esc` | back to Home; the conversation stays |
| Fluency | `↑` `↓` / `j` `k` | choose a topic (the last one: type your own) |
| | `enter` | pick it; then start each round's clock |
| | `ctrl+d` | end the round early; the time stops |
| | `n` | a new topic between rounds or from the result |
| | `esc` | back to Home; the clock keeps running |
| Quiz | `↑` `↓` / `j` `k` | choose a vocabulary or a grammar quiz |
| | `enter` | start it; after the feedback, the next question |
| | `1`–`4` | choose an option |
| | `enter` | check a typed answer (empty: I don't know) |
| | `enter` | after a failed grammar quiz: try again |
| | `n` | a new quiz from the result |
| | `esc` | back to Home; the quiz stays |
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

### The AI teacher

Learn, Write, Talk and the grammar quiz use the AI through `pi`, the engine of Gentle Shell, so they
need Gentle Shell installed and signed in; the app never reads its
credentials. Each answer takes about 10 seconds. Without `pi` these screens
say how to turn it on and the rest of the app works as before.

- **Learn** writes a 60–100 word text at your level that reuses the words you
  know and adds at most 3 new words or chunks (`new` in the glossary), so you
  know about 98% of the text and learn the rest from context. New words come
  marked; `space` marks any other, `a` adds them as cards and records the
  reading as study (at most 15 minutes).
- **Write** runs six steps, built on research on written corrective
  feedback (focused, direct feedback on a few errors, then revision):
  1. **Plan** — a short task with a hint in Spanish and 2–4 useful phrases,
     taken from your own cards when they fit. When an error kind keeps coming
     back (seen twice or more), the task makes you practice the two most
     repeated ones.
  2. **Write** — `enter` is a new line, `ctrl+d` sends, `esc` keeps the draft.
  3. **Fix it yourself** — your text comes back with at most 3 major errors
     marked (`«I has 30 years»`) and the kind of each, but not the answer.
     You get one try per error; `enter` with nothing typed means "I don't
     know". A try equal to the teacher's fix counts right away; the others
     go to the teacher in one call, which also accepts valid alternatives.
     If that call fails, those tries count as not fixed and the page says so.
  4. **Feedback** — each major error shows `✓ fixed by you` or
     `✗ I has 30 years → I am 30 years old` with a one-line why in Spanish.
     Minor details are collapsed (`+2 minor details`); `d` shows them. Only
     the major errors you could not fix become cards (your version is kept as
     the last wrong answer), and every major error is logged by kind.
  5. **Rewrite** — for each error you could not fix you see its Spanish
     meaning and type the English from memory, checked like a typed review
     answer (✓, ✗ almost with the letter diff, or ✗ with the right answer).
     No AI call.
  6. **Result** — Grammar, Vocabulary, Coherence and Task, each out of 10,
     the total out of 40 with a grade (A ≥ 90%, B ≥ 80%, C ≥ 70%, D ≥ 60%,
     F below), the cards added and a pattern line when an error kind has been
     seen before (`Pattern: 3rd time with to be (am/is/are)`).

  The writing counts as study (`Writing: 32/40`, at most 30 minutes) as soon
  as the correction arrives. `n` starts a new task from the feedback or the
  result; `esc` goes Home and keeps your place.
- **Talk** is a guided role-play: *Introduce yourself*, *Order a coffee*,
  *Ask for directions* or *Small talk at work*. The tutor plays the other
  person (a classmate, a barista, a local, a coworker) and opens the
  conversation; you see the goal and sentence frames that fade with your
  level: A1 gets whole frames (`Can I have a ___, please?`), A2 only their
  first words (`Can I…`), B1 and above only the goal. Every key is a letter
  while you type; `enter` sends your line. The tutor answers in character,
  short and at your level, and when your line has an error it says it
  correctly inside its reply without stopping you; the correct version shows
  quietly under your line (`↳ Can I have a big coffee, please?`). If an
  answer fails, your line comes back and `enter` sends it again. The
  role-play ends when you meet the goal, after 8 of your lines or with
  `ctrl+d`. Then the teacher gives 2–3 short points in Spanish on what went
  well and what to work on, and lists your errors: the major ones become
  cards (your line is the example), and are logged by kind like Write's.
  The role-play counts as study (`Talk: Order a coffee`, at most 20 minutes)
  when the feedback arrives; leaving before that records nothing. `n` picks
  a new role-play; `esc` goes Home and keeps the conversation.

### Fluency

Fluency is the written 4/3/2 exercise: tell the same story three times, in
4, 3 and 2 minutes, so the words you already know come out faster. It uses
no AI, so it works without `pi`. Pick a topic (*Your morning routine*,
*Your best friend*, *Your last weekend*, *Your home*, *Your favorite food*)
or the last entry to type your own. Each round starts on `enter`; the
clock counts down on the page and every key is a letter while it runs
(`enter` is a new line). When the time is up the round ends and the text
locks; `ctrl+d` ends it early. Words per minute is the words of the text
(split on spaces) divided by the minutes used, at most the round's limit.
The result shows one line per round (`Round 1 · 4:00 · 38 words · 9.5 wpm`)
and the change from round 1 to round 3. The session counts as study
(`Fluency: Your home`, the real time since round 1 began, at most 9
minutes) when round 3 ends; leaving earlier records nothing. `esc` goes
Home and the clock keeps running, so a round can end while you are
elsewhere; `n` picks a new topic between rounds or from the result.

### Quiz

Quiz checks what you remember. It is retrieval practice, not grading: a
quiz never changes your review dates. Pick one of two:

- **Vocabulary** uses no AI, so it works without `pi`. It asks 10 words of
  your own deck (each word once, in a new order every time), alternating
  multiple choice (the Spanish meaning → choose the English word among 4,
  with other words of your deck as the wrong options; keys `1`–`4`) and
  typed (the meaning, and the example with the word blanked → type the
  word, checked like a typed review answer: ✓, ✗ almost with the letter
  diff, or ✗ with the right answer). With fewer than 4 words every
  question is typed; with fewer than 10 there is one question per word; an
  empty deck says so.
- **Grammar** asks the teacher for 8 multiple-choice items at your level on
  your current lesson: the two error kinds you repeat most (seen twice or
  more, as in Write), or else the core grammar of your level (A1: to be,
  present simple, articles, plurals, possessives). Items with a bad answer
  or the wrong number of options are dropped; fewer than 3 good ones is an
  error. If the call fails, the page says why and `enter` tries again.

After each answer you see ✓, or ✗ with the right answer and, for grammar,
why in Spanish; `enter` moves on, and every key is a letter while you type
an answer. The result shows your score (`8/10`) and the questions to look at
again (a grammar item with its why). The quiz counts as study
(`Quiz: vocabulary 8/10` or `Quiz: grammar 6/8`, at most 10 minutes) when
you answer the last question; leaving earlier records nothing. `n` starts a
new quiz; `esc` goes Home and keeps the quiz.

Models live in `$XDG_CONFIG_HOME/english-teacher/config.json`
(`~/.config/english-teacher/config.json`); `--config path` picks another.
The defaults are Sonnet for reading, writing and conversation and Haiku for
quizzes; any model in `pi --list-models` works. A `"pi"` key sets the path of
the `pi` binary.

## Roadmap

- **M1** — Vocabulary review TUI with spaced repetition (SM-2), importing the existing vocabulary bank. No AI. Done.
- **M2** — Streak, CEFR level, daily goal and daily log in SQLite. Done.
- **M3** — AI teacher: graded reading and writing correction, errors become cards, one model per capability. Done.
- **M4** — Guided role-plays (Talk), written 4/3/2 fluency (Fluency) and quizzes (Quiz). Done.
- **v0.1.0** — M1–M4: every text feature. `teacher --version` prints it; see [CHANGELOG.md](CHANGELOG.md).
- **v0.2.0** — Voice: shadowing, pronunciation and minimal pairs.

## Layout

```
cmd/teacher/        CLI entry point
internal/domain/    pure types: Card, Grade, Answer, Direction
internal/srs/       SM-2 scheduler and session planning (pure, no I/O)
internal/check/     typed-answer check and letter diff (pure)
internal/progress/  streak, week and daily totals (pure, no I/O)
internal/importer/  imports english-master markdown data
internal/store/     SQLite persistence
internal/llm/pi/    runs the AI through the pi CLI
internal/teacher/   AI teacher use cases: prompts and answers
internal/tui/       Bubbletea screens
```
