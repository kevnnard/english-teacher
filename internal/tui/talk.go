package tui

import (
	"context"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/kevnnard/english-teacher/internal/domain"
	"github.com/kevnnard/english-teacher/internal/speech"
	"github.com/kevnnard/english-teacher/internal/teacher"
)

// maxTalkTurns is the most lines the learner says in one role-play; after
// the last one the feedback summary starts on its own.
const maxTalkTurns = 8

// maxTalkMinutes caps the study time one role-play can record. Eight short
// turns take a few minutes; 20 leaves room for a slow beginner without
// letting a conversation left open fill the daily goal.
const maxTalkMinutes = 20

// talkPhase is where the learner is in a role-play (spec S9).
type talkPhase int

const (
	talkPick        talkPhase = iota // choosing a scenario
	talkChatting                     // the learner types a line
	talkRecording                    // the learner says the line; ctrl+r stops
	talkListening                    // whisper is writing the line down
	talkWaiting                      // waiting for the tutor's reply
	talkSummarizing                  // waiting for the feedback summary
	talkSummaryFail                  // the summary failed; enter asks again
	talkSummary                      // the summary, its cards and the error log
)

// talkState is the Talk screen: one guided role-play and its feedback.
type talkState struct {
	phase     talkPhase
	req       int // the id of the latest request; older answers are ignored
	cursor    int // the scenario under the pointer in the picker
	scenario  teacher.Scenario
	lines     []teacher.Line // the transcript, starting with the tutor's opening
	input     string         // the line the learner is typing
	pending   string         // the line sent and waiting for its reply
	startedAt time.Time      // when the role-play started, for the study minutes

	recording speech.Recording // the line being recorded, while recording
	try       int              // recordings started, so an old transcript is ignored

	feedback teacher.Feedback
	cards    []domain.Card // the error cards, kept until they are saved
	added    int
	cardErr  error // the cards could not be saved; `s` tries again
	logErr   error // the major errors could not be logged
	err      error // the last failed request, shown on the page
}

// typing reports whether every key belongs to the conversation: while
// the learner types, records or waits for whisper.
func (t talkState) typing() bool {
	return t.phase == talkChatting || t.phase == talkRecording || t.phase == talkListening
}

// learnerTurns is how many lines the learner has said.
func (t talkState) learnerTurns() int {
	n := 0
	for _, l := range t.lines {
		if !l.Tutor {
			n++
		}
	}
	return n
}

// talkMsg and talkFeedbackMsg are the teacher's answers to request req.
type talkMsg struct {
	req  int
	turn teacher.Turn
	err  error
}

type talkFeedbackMsg struct {
	req      int
	feedback teacher.Feedback
	err      error
}

// startTalk opens the scenario under the cursor with the tutor's opening
// line. It needs no AI call. The command says the opening in voice mode.
func (a *App) startTalk() tea.Cmd {
	t := &a.talk
	sc := teacher.Scenarios[t.cursor]
	a.talk = talkState{
		phase: talkChatting, req: t.req + 1, try: t.try, cursor: t.cursor, scenario: sc,
		lines: []teacher.Line{{Tutor: true, Text: sc.Opening}}, startedAt: a.now(),
	}
	return a.sayTutor(sc.Opening)
}

// sendLine sends the learner's line and asks the tutor for the next turn.
// The line joins the transcript only when the reply arrives, so a failed
// turn gives it back to the input.
func (a App) sendLine() (App, tea.Cmd) {
	t := &a.talk
	line := strings.TrimSpace(t.input)
	if line == "" {
		return a, nil
	}
	t.req++
	t.phase, t.pending, t.input, t.err = talkWaiting, line, "", nil
	lines := append(append([]teacher.Line(nil), t.lines...), teacher.Line{Text: line})
	req, tc, level, sc := t.req, a.teacher, a.profile.Level, t.scenario
	return a, func() tea.Msg {
		turn, err := tc.RolePlay(context.Background(), level, sc, lines)
		return talkMsg{req: req, turn: turn, err: err}
	}
}

// gotTurn adds the learner's line, with its recast, and the tutor's reply.
// When the goal is met or the turns are used up, it asks for the summary.
func (a App) gotTurn(msg talkMsg) (App, tea.Cmd) {
	t := &a.talk
	if msg.req != t.req || t.phase != talkWaiting {
		return a, nil
	}
	if msg.err != nil {
		t.phase, t.input, t.pending, t.err = talkChatting, t.pending, "", msg.err
		return a, nil
	}
	t.lines = append(t.lines,
		teacher.Line{Text: t.pending, Recast: msg.turn.Recast},
		teacher.Line{Tutor: true, Text: msg.turn.Reply})
	t.pending = ""
	speak := a.sayTutor(msg.turn.Reply) // the reply only: the recast is to read
	if msg.turn.Done || t.learnerTurns() >= maxTalkTurns {
		a, ask := a.askTalkFeedback()
		return a, tea.Batch(speak, ask)
	}
	t.phase = talkChatting
	return a, speak
}

// askTalkFeedback asks for the summary of the role-play.
func (a App) askTalkFeedback() (App, tea.Cmd) {
	t := &a.talk
	t.req++
	t.phase, t.err = talkSummarizing, nil
	req, tc, level, sc, lines := t.req, a.teacher, a.profile.Level, t.scenario, t.lines
	return a, func() tea.Msg {
		fb, err := tc.RolePlayFeedback(context.Background(), level, sc, lines)
		return talkFeedbackMsg{req: req, feedback: fb, err: err}
	}
}

// gotTalkFeedback shows the summary and, once, records the role-play as
// study, turns the major errors into cards and logs them.
func (a App) gotTalkFeedback(msg talkFeedbackMsg) App {
	t := &a.talk
	if msg.req != t.req || t.phase != talkSummarizing {
		return a
	}
	if msg.err != nil {
		t.phase, t.err = talkSummaryFail, msg.err
		return a
	}
	t.phase, t.feedback = talkSummary, msg.feedback
	minutes := min(int(a.now().Sub(t.startedAt)/time.Minute), maxTalkMinutes)
	a.addToToday(domain.Day{Minutes: max(minutes, 0), Activities: []string{"Talk: " + t.scenario.Title}})

	majors := msg.feedback.Majors()
	t.cards = nil
	mistakes := make([]domain.Mistake, len(majors))
	for i, f := range majors {
		t.cards = append(t.cards, f.Card(t.learnerLineWith(f.Wrong)))
		mistakes[i] = domain.Mistake{Date: a.today(), Kind: teacher.NormalizeKind(f.Kind), Wrong: f.Wrong, Right: f.Right}
	}
	a.saveTalkCards()
	if len(mistakes) > 0 {
		t.logErr = a.storeMistakes(mistakes)
	}
	return a
}

// learnerLineWith is the first line of the learner that contains chunk,
// ignoring case, or "" when none does. It becomes the card's example.
func (t talkState) learnerLineWith(chunk string) string {
	chunk = strings.ToLower(strings.TrimSpace(chunk))
	for _, l := range t.lines {
		if !l.Tutor && chunk != "" && strings.Contains(strings.ToLower(l.Text), chunk) {
			return l.Text
		}
	}
	return ""
}

// saveTalkCards stores the error cards. On failure they stay in the
// state, so `s` can try again.
func (a *App) saveTalkCards() {
	t := &a.talk
	if len(t.cards) == 0 || a.add == nil {
		t.cards = nil
		return
	}
	n, err := a.add(t.cards)
	if err != nil {
		t.cardErr = err
		return
	}
	a.addCards(t.cards)
	t.added, t.cards, t.cardErr = n, nil, nil
}

func (a App) updateTalk(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	if a.teacher == nil {
		return a, nil
	}
	t := &a.talk
	k := msg.String()
	switch t.phase {
	case talkPick:
		switch k {
		case "up", "k":
			t.cursor = max(t.cursor-1, 0)
		case "down", "j":
			t.cursor = min(t.cursor+1, len(teacher.Scenarios)-1)
		case "enter":
			return a, a.startTalk()
		}
	case talkRecording, talkListening:
		return a.updateTalkVoice(k)
	case talkChatting:
		switch k {
		case "ctrl+r", "ctrl+t", "ctrl+p":
			return a.updateTalkVoice(k)
		case "enter":
			return a.sendLine()
		case "ctrl+d":
			if t.learnerTurns() > 0 {
				return a.askTalkFeedback()
			}
		case "esc":
			a.screen = Home // the conversation stays for later
		default:
			t.input = editInput(t.input, msg)
		}
	case talkSummaryFail:
		if k == "enter" {
			return a.askTalkFeedback()
		}
	case talkSummary:
		switch {
		case k == "s" && len(t.cards) > 0:
			a.saveTalkCards()
		case k == "n":
			a.talk = talkState{phase: talkPick, req: t.req, try: t.try, cursor: t.cursor}
		}
	}
	return a, nil
}
