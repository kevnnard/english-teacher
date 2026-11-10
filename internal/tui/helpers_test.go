package tui

import (
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/kevnnard/english-teacher/internal/domain"
)

var fixedNow = time.Date(2026, 11, 9, 21, 30, 0, 0, time.UTC)

// testClock is an injectable clock tests can move forward.
type testClock struct{ t time.Time }

func (c *testClock) now() time.Time { return c.t }

// specialKeys maps the names used in tests to the key codes a terminal sends.
var specialKeys = map[string]tea.KeyPressMsg{
	"space":     {Code: tea.KeySpace, Text: " "},
	"enter":     {Code: tea.KeyEnter},
	"esc":       {Code: tea.KeyEscape},
	"tab":       {Code: tea.KeyTab},
	"shift+tab": {Code: tea.KeyTab, Mod: tea.ModShift},
	"backspace": {Code: tea.KeyBackspace},
	"up":        {Code: tea.KeyUp},
	"down":      {Code: tea.KeyDown},
	"ctrl+c":    {Code: 'c', Mod: tea.ModCtrl},
	"ctrl+z":    {Code: 'z', Mod: tea.ModCtrl},
	"ctrl+d":    {Code: 'd', Mod: tea.ModCtrl},
}

// key builds a synthetic key press, like the terminal would send.
func key(s string) tea.KeyPressMsg {
	if k, ok := specialKeys[s]; ok {
		return k
	}
	r := []rune(s)[0]
	return tea.KeyPressMsg{Code: r, Text: s}
}

// recorder is a fake save function that remembers every saved card.
type recorder struct {
	saved []domain.Card
	err   error
}

func (r *recorder) save(c domain.Card) error {
	r.saved = append(r.saved, c)
	return r.err
}

func day(offset int) time.Time {
	return time.Date(2026, 11, 9+offset, 0, 0, 0, 0, time.UTC)
}

func word(w, pron, meaning, example string) domain.Card {
	c := domain.NewCard(w)
	c.Pronunciation, c.Meaning, c.Example = pron, meaning, example
	return c
}

// reviewed marks c as studied before, with its next review on due.
func reviewed(c domain.Card, due time.Time, interval int) domain.Card {
	last := time.Date(2026, 8, 14, 0, 0, 0, 0, time.UTC)
	c.LastReviewed = &last
	c.TimesCorrect = 1
	c.Repetitions = 1
	c.IntervalDays = interval
	c.Due = due
	return c
}

// deck has three due words, two new, one learning and one known at fixedNow.
func deck() []domain.Card {
	return []domain.Card{
		reviewed(word("hello", "/həˈloʊ/", "Hola", "Hello, how are you?"), time.Time{}, 0),
		reviewed(word("hi", "/haɪ/", "Hola (informal)", "Hi, nice to meet you!"), time.Time{}, 0),
		reviewed(word("nice to meet you", "/naɪs tuː miːt juː/", "Mucho gusto", "Nice to meet you, I'm Kevin."), day(-1), 1),
		word("Argentina", "/ˌɑːrdʒənˈtiːnə/", "País: Argentina", "I am from Argentina."),
		word("Brazil", "/brəˈzɪl/", "País: Brasil", "She is from Brazil."),
		reviewed(word("goodbye", "/ɡʊdˈbaɪ/", "Adiós", "Goodbye, see you tomorrow!"), day(1), 1),
		reviewed(word("thank you", "/θæŋk juː/", "Gracias", "Thank you so much!"), day(20), 30),
	}
}

// newApp builds an app over cards at fixedNow, already sized to w×h.
func newApp(t *testing.T, cards []domain.Card, rec *recorder, start Screen, w, h int) App {
	t.Helper()
	a, _ := newAppClock(t, cards, rec, start, w, h)
	return a
}

// newAppClock is newApp that also returns the clock, to move time forward.
func newAppClock(t *testing.T, cards []domain.Card, rec *recorder, start Screen, w, h int) (App, *testClock) {
	t.Helper()
	clock := &testClock{t: fixedNow}
	a := NewApp(Config{Cards: cards, Save: rec.save, Now: clock.now, Start: start, MaxNew: DefaultMaxNew})
	return send(t, a, tea.WindowSizeMsg{Width: w, Height: h}), clock
}

// prodDeck has one word whose production direction has just been unlocked:
// its recognition card has two repetitions and is not due.
func prodDeck() []domain.Card {
	rec := reviewed(word("nice to meet you", "/naɪs tuː miːt juː/", "Mucho gusto", "Nice to meet you, I'm Kevin."), day(6), 6)
	rec.Repetitions = 2
	prod := word(rec.Word, rec.Pronunciation, rec.Meaning, rec.Example)
	prod.Direction = domain.Production
	return []domain.Card{rec, prod}
}

// send feeds msgs to a in order and returns the final model.
func send(t *testing.T, a App, msgs ...tea.Msg) App {
	t.Helper()
	a, _ = sendCmd(t, a, msgs...)
	return a
}

// sendCmd is send that also returns the last command.
func sendCmd(t *testing.T, a App, msgs ...tea.Msg) (App, tea.Cmd) {
	t.Helper()
	var cmd tea.Cmd
	for _, msg := range msgs {
		var next tea.Model
		next, cmd = a.Update(msg)
		a = next.(App)
	}
	return a, cmd
}

// press feeds named keys to a and returns the final model and last command.
func press(t *testing.T, a App, keys ...string) (App, tea.Cmd) {
	t.Helper()
	msgs := make([]tea.Msg, len(keys))
	for i, k := range keys {
		msgs[i] = key(k)
	}
	return sendCmd(t, a, msgs...)
}

// typeText presses one key per rune of s.
func typeText(t *testing.T, a App, s string) App {
	t.Helper()
	for _, r := range s {
		if r == ' ' {
			a, _ = press(t, a, "space")
			continue
		}
		a, _ = press(t, a, string(r))
	}
	return a
}

// screen returns the view without ANSI escape codes.
func screen(a App) string { return ansi.Strip(a.View().Content) }

func screenLines(a App) []string { return strings.Split(screen(a), "\n") }

// pageText is the view without the top bar and the status bar.
func pageText(a App) string {
	lines := screenLines(a)
	return strings.Join(lines[1:len(lines)-1], "\n")
}

func statusBar(a App) string {
	l := screenLines(a)
	return l[len(l)-1]
}

func isQuit(cmd tea.Cmd) bool {
	if cmd == nil {
		return false
	}
	_, ok := cmd().(tea.QuitMsg)
	return ok
}

func mustContain(t *testing.T, got string, wants ...string) {
	t.Helper()
	for _, w := range wants {
		if !strings.Contains(got, w) {
			t.Errorf("missing %q in:\n%s", w, got)
		}
	}
}

func mustNotContain(t *testing.T, got string, unwanted ...string) {
	t.Helper()
	for _, w := range unwanted {
		if strings.Contains(got, w) {
			t.Errorf("unexpected %q in:\n%s", w, got)
		}
	}
}

func TestKeyStrings(t *testing.T) {
	// Guards our synthetic keys against the strings Update matches on.
	for name := range specialKeys {
		if got := key(name).String(); got != name {
			t.Errorf("key(%q).String() = %q", name, got)
		}
	}
	for _, s := range []string{"q", ":", "?", "/", "1"} {
		if got := key(s).String(); got != s {
			t.Errorf("key(%q).String() = %q", s, got)
		}
	}
}
