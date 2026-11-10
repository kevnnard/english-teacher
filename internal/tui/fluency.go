package tui

import (
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/kevnnard/english-teacher/internal/domain"
)

// fluencyTopics are the stories the learner can tell (spec S10). Nation's
// fluency strand works on familiar content, so they are everyday A1
// topics and need no AI call.
var fluencyTopics = []string{
	"Your morning routine",
	"Your best friend",
	"Your last weekend",
	"Your home",
	"Your favorite food",
}

// ownTopic is the last entry of the picker: the learner types a topic.
const ownTopic = "Your own topic…"

// roundLimits are the times of the three 4/3/2 rounds.
var roundLimits = [3]time.Duration{4 * time.Minute, 3 * time.Minute, 2 * time.Minute}

// maxFluencyMinutes caps the study time one 4/3/2 session can record:
// the three rounds together, so pauses between them do not count.
const maxFluencyMinutes = 9

// fluencyPhase is where the learner is in a 4/3/2 session.
type fluencyPhase int

const (
	fluencyPick     fluencyPhase = iota // choosing a topic
	fluencyOwnTopic                     // typing a topic of their own
	fluencyReady                        // the next round waits for enter
	fluencyRunning                      // a round runs; every key is a letter
	fluencyResult                       // the three rounds are over
)

// fluencyRound is one telling of the story.
type fluencyRound struct {
	text    string
	start   time.Time
	elapsed time.Duration // set when the round ends, at most its limit
}

// fluencyState is the Fluency screen: one topic told three times.
type fluencyState struct {
	phase  fluencyPhase
	cursor int    // the topic under the pointer in the picker
	topic  string // the topic being told
	input  string // the learner's own topic while they type it
	round  int    // the round ready, running or just finished: 0, 1 or 2
	run    int    // counts started rounds, so a late tick is ignored
	rounds [3]fluencyRound
}

// typing reports whether the keys belong to a text input.
func (f fluencyState) typing() bool {
	return f.phase == fluencyRunning || f.phase == fluencyOwnTopic
}

// fluencyTickMsg is the timer of the round started as run.
type fluencyTickMsg struct{ run int }

// fluencyTick asks Bubble Tea for one tick a second from now.
func fluencyTick(run int) tea.Cmd {
	return tea.Tick(time.Second, func(time.Time) tea.Msg { return fluencyTickMsg{run: run} })
}

// countWords counts the words of text, split on any white space.
func countWords(text string) int {
	return len(strings.Fields(text))
}

// wordsPerMinute is the words of text divided by the elapsed minutes,
// with elapsed capped at the round's limit. No time means 0.
func wordsPerMinute(text string, elapsed, limit time.Duration) float64 {
	elapsed = min(elapsed, limit)
	if elapsed <= 0 {
		return 0
	}
	return float64(countWords(text)) / elapsed.Minutes()
}

// remaining is the time left in the running round, never below zero.
func (a App) remaining() time.Duration {
	f := a.fluency
	used := a.now().Sub(f.rounds[f.round].start)
	return max(roundLimits[f.round]-used, 0)
}

// startTopic opens round 1 of topic.
func (a *App) startTopic(topic string) {
	a.fluency = fluencyState{phase: fluencyReady, cursor: a.fluency.cursor, topic: topic, run: a.fluency.run}
}

// startRound starts the timer of the round that is ready.
func (a App) startRound() (App, tea.Cmd) {
	f := &a.fluency
	f.run++
	f.phase = fluencyRunning
	f.rounds[f.round] = fluencyRound{start: a.now()}
	return a, fluencyTick(f.run)
}

// endRound locks the running round. After round 3 it records the session
// once, with the real time since round 1 began, capped.
func (a *App) endRound() {
	f := &a.fluency
	r := &f.rounds[f.round]
	r.elapsed = min(max(a.now().Sub(r.start), 0), roundLimits[f.round])
	if f.round < len(roundLimits)-1 {
		f.round++
		f.phase = fluencyReady
		return
	}
	f.phase = fluencyResult
	minutes := min(int(a.now().Sub(f.rounds[0].start)/time.Minute), maxFluencyMinutes)
	a.addToToday(domain.Day{Minutes: max(minutes, 0), Activities: []string{"Fluency: " + f.topic}})
}

// gotFluencyTick ends the round when its time is up and otherwise asks
// for the next tick. It touches only the fluency state, so it is harmless
// while another screen is on display.
func (a App) gotFluencyTick(msg fluencyTickMsg) (App, tea.Cmd) {
	if msg.run != a.fluency.run || a.fluency.phase != fluencyRunning {
		return a, nil
	}
	if a.remaining() == 0 {
		a.endRound()
		return a, nil
	}
	return a, fluencyTick(msg.run)
}

func (a App) updateFluency(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	f := &a.fluency
	k := msg.String()
	switch f.phase {
	case fluencyPick:
		switch k {
		case "up", "k":
			f.cursor = max(f.cursor-1, 0)
		case "down", "j":
			f.cursor = min(f.cursor+1, len(fluencyTopics))
		case "enter":
			if f.cursor == len(fluencyTopics) {
				f.phase, f.input = fluencyOwnTopic, ""
			} else {
				a.startTopic(fluencyTopics[f.cursor])
			}
		}
	case fluencyOwnTopic:
		switch k {
		case "enter":
			if topic := strings.TrimSpace(f.input); topic != "" {
				a.startTopic(topic)
			}
		case "esc":
			f.phase = fluencyPick
		default:
			f.input = editInput(f.input, msg)
		}
	case fluencyReady:
		switch k {
		case "enter":
			return a.startRound()
		case "n":
			a.fluency = fluencyState{cursor: f.cursor, run: f.run}
		}
	case fluencyRunning:
		if a.remaining() == 0 { // the tick is late; the time is still up
			a.endRound()
			return a, nil
		}
		r := &f.rounds[f.round]
		switch k {
		case "ctrl+d":
			a.endRound()
		case "esc":
			a.screen = Home // the round keeps running
		case "enter":
			r.text += "\n"
		default:
			r.text = editInput(r.text, msg)
		}
	case fluencyResult:
		if k == "n" {
			a.fluency = fluencyState{cursor: f.cursor, run: f.run}
		}
	}
	return a, nil
}
