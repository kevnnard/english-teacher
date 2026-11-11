package tui

import (
	"fmt"
	"image/color"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/kevnnard/english-teacher/internal/domain"
)

// marginColumn is where the notebook margin line sits for a terminal width.
func marginColumn(width int) int {
	if width >= 100 {
		return 6
	}
	return 2
}

func TestHeaderShowsDateContextAndTabs(t *testing.T) {
	for _, width := range []int{80, 120, 160} {
		a := newApp(t, deck(), &recorder{}, Home, width, 24)

		top := screenLines(a)[0]

		date := "Monday 9 Nov"
		if width == 80 {
			date = "Mon 9 Nov" // eleven tabs leave room only for the short date
		}
		mustContain(t, top, date, "Home", "Review", "Words", "Stats")
	}
}

func TestHeaderKeepsTheActiveTabWhenNarrow(t *testing.T) {
	a := newApp(t, deck(), &recorder{}, Review, 50, 24)

	top := screenLines(a)[0]

	mustContain(t, top, "Page 1 of 5", "Review")
	mustNotContain(t, top, "Stats")
}

func TestMarginLineRunsDownThePage(t *testing.T) {
	for _, width := range []int{60, 80, 99, 100, 120, 160} {
		t.Run(fmt.Sprint(width), func(t *testing.T) {
			a := newApp(t, deck(), &recorder{}, Review, width, 24)
			lines := screenLines(a)
			col := marginColumn(width)

			for i, l := range lines[1 : len(lines)-1] {
				r := []rune(l)
				if len(r) <= col || r[col] != '│' {
					t.Fatalf("line %d: want the margin line in column %d:\n%q", i+1, col, l)
				}
			}
		})
	}
}

func TestLayoutIsRecomputedOnResize(t *testing.T) {
	a := newApp(t, deck(), &recorder{}, Home, 160, 24)

	a = send(t, a, tea.WindowSizeMsg{Width: 80, Height: 30})

	lines := screenLines(a)
	if n := len(lines); n != 30 {
		t.Errorf("view has %d lines, want the new height 30", n)
	}
	if r := []rune(lines[1]); r[marginColumn(80)] != '│' {
		t.Errorf("after shrinking to 80 the margin should move to column 2: %q", lines[1])
	}
}

// longDeck stresses wrapping and truncation with oversized text.
func longDeck() []domain.Card {
	long := word("a really long expression that keeps going and going past the card",
		"/ə ˈrɪəli lɒŋ ɪkˈsprɛʃən ðæt kiːps ˈɡəʊɪŋ ænd ˈɡəʊɪŋ/",
		strings.Repeat("Una expresión muy larga ", 8),
		strings.Repeat("This example sentence is much longer than any terminal line. ", 4))
	return append([]domain.Card{long}, deck()...)
}

func TestNoLineExceedsTheTerminalWidth(t *testing.T) {
	for _, width := range []int{60, 80, 120, 160} {
		for _, s := range screens {
			for _, keys := range [][]string{nil, {"space"}, {"?"}, {":"}} {
				name := fmt.Sprintf("%d/%s/%v", width, s, keys)
				t.Run(name, func(t *testing.T) {
					a := newApp(t, longDeck(), &recorder{}, s, width, 24)
					a, _ = press(t, a, keys...)

					lines := strings.Split(a.View().Content, "\n")
					if len(lines) != 24 {
						t.Errorf("view has %d lines, want 24", len(lines))
					}
					for i, l := range lines {
						if w := ansi.StringWidth(l); w > width {
							t.Errorf("line %d is %d columns wide (terminal %d):\n%s", i, w, width, ansi.Strip(l))
						}
					}
				})
			}
		}
	}
}

func TestReviewStatesFitTheTerminalWidth(t *testing.T) {
	long := prodDeck()
	long[1].LastWrong = strings.Repeat("a very long wrong answer ", 6)
	states := map[string]struct {
		cards []domain.Card
		keys  []string
	}{
		"production":    {long, nil},
		"near miss":     {long, strings.Split("n i c e space t o space m e a t space y o u enter", " ")},
		"wrong":         {long, []string{"x", "enter"}},
		"session end":   {deck(), []string{"space", "1", "space", "1", "space", "3", "space", "3", "space", "3"}},
		"mistakes done": {deck(), []string{"space", "1", "space", "3", "space", "3", "space", "3", "space", "3", "r", "space", "1"}},
	}
	for _, width := range []int{60, 80, 120, 160} {
		for name, st := range states {
			t.Run(fmt.Sprintf("%d/%s", width, name), func(t *testing.T) {
				a := newApp(t, st.cards, &recorder{}, Review, width, 24)
				a, _ = press(t, a, st.keys...)

				lines := strings.Split(a.View().Content, "\n")
				if len(lines) != 24 {
					t.Errorf("view has %d lines, want 24", len(lines))
				}
				for i, l := range lines {
					if w := ansi.StringWidth(l); w > width {
						t.Errorf("line %d is %d columns wide (terminal %d):\n%s", i, w, width, ansi.Strip(l))
					}
				}
			})
		}
	}
}

func TestStatusBarIsTheLastRow(t *testing.T) {
	for _, width := range []int{60, 80, 120, 160} {
		a := newApp(t, deck(), &recorder{}, Home, width, 24)

		bar := statusBar(a)

		mustContain(t, bar, "A1", "5 due")
		if !strings.Contains(bar, "help") && width >= 80 {
			t.Errorf("width %d: status bar should end with key hints: %q", width, bar)
		}
	}
}

func TestStatusBarNeverFakesAStreak(t *testing.T) {
	a := newApp(t, deck(), &recorder{}, Home, 160, 24)

	mustNotContain(t, statusBar(a), "▲", "streak", "days")
}

func TestStatusBarShowsSessionResultsAfterGrading(t *testing.T) {
	a := newApp(t, deck(), &recorder{}, Review, 120, 24)
	mustNotContain(t, statusBar(a), "✓", "✗")

	a, _ = press(t, a, "space", "4", "space", "1")

	mustContain(t, statusBar(a), "✓ 1", "✗ 1", "3 due")
}

func TestStatusBarHintsFollowTheScreen(t *testing.T) {
	a := newApp(t, deck(), &recorder{}, Home, 160, 24)
	mustContain(t, statusBar(a), "enter start")

	a, _ = press(t, a, "enter")
	mustContain(t, statusBar(a), "space reveal")

	a, _ = press(t, a, "space")
	mustContain(t, statusBar(a), "grade")
}

func TestTabCyclesScreens(t *testing.T) {
	a := newApp(t, deck(), &recorder{}, Home, 120, 24)

	var visited []Screen
	for range len(screens) {
		a, _ = press(t, a, "tab")
		visited = append(visited, a.Screen())
	}
	want := []Screen{Review, Learn, Write, Talk, Fluency, Quiz, Listen, Pairs, Words, Stats, Home}
	if fmt.Sprint(visited) != fmt.Sprint(want) {
		t.Errorf("tab visited %v, want %v", visited, want)
	}

	a, _ = press(t, a, "shift+tab")
	if a.Screen() != Stats {
		t.Errorf("shift+tab from Home = %v, want Stats", a.Screen())
	}
}

func TestEscGoesHome(t *testing.T) {
	for _, s := range screens {
		a := newApp(t, deck(), &recorder{}, s, 80, 24)

		a, cmd := press(t, a, "esc")

		if a.Screen() != Home || isQuit(cmd) {
			t.Errorf("esc on %v: screen %v quit %v, want Home", s, a.Screen(), isQuit(cmd))
		}
	}
}

func TestQuitKeys(t *testing.T) {
	a := newApp(t, deck(), &recorder{}, Home, 80, 24)
	if _, cmd := press(t, a, "q"); !isQuit(cmd) {
		t.Error("q on Home should quit")
	}

	a = newApp(t, deck(), &recorder{}, Review, 80, 24)
	a, cmd := press(t, a, "q")
	if isQuit(cmd) || a.Screen() != Home {
		t.Errorf("q in Review should return Home, got %v quit=%v", a.Screen(), isQuit(cmd))
	}
}

func TestCtrlCAlwaysQuits(t *testing.T) {
	for _, keys := range [][]string{nil, {":"}, {"?"}, {"tab"}, {"tab", "space"}, {"tab", "tab", "tab", "tab", "/"}} {
		a := newApp(t, deck(), &recorder{}, Home, 80, 24)
		a, _ = press(t, a, keys...)

		if _, cmd := press(t, a, "ctrl+c"); !isQuit(cmd) {
			t.Errorf("after %v: ctrl+c should quit", keys)
		}
	}
}

func TestPaletteFiltersAndJumps(t *testing.T) {
	a := newApp(t, deck(), &recorder{}, Home, 80, 24)

	a, _ = press(t, a, ":")
	mustContain(t, screen(a), "Review", "Words", "Stats")

	a = typeText(t, a, "wo")
	v := pageText(a) // the header always lists every tab
	mustContain(t, v, "▸ Words")
	mustNotContain(t, v, "Stats")

	a, _ = press(t, a, "enter")
	if a.Screen() != Words {
		t.Errorf("palette enter went to %v, want Words", a.Screen())
	}
	mustNotContain(t, screen(a), ": wo")
}

func TestPaletteTypingDoesNotTriggerGlobalKeys(t *testing.T) {
	a := newApp(t, deck(), &recorder{}, Home, 80, 24)

	a, cmd := press(t, a, ":", "q")

	if isQuit(cmd) {
		t.Error("typing q in the palette should not quit")
	}
	a, _ = press(t, a, "esc")
	if a.Screen() != Home || strings.Contains(screen(a), ": q") {
		t.Errorf("esc should close the palette and stay Home:\n%s", screen(a))
	}
}

func TestHelpOverlayToggles(t *testing.T) {
	a := newApp(t, deck(), &recorder{}, Home, 80, 24)

	a, _ = press(t, a, "?")
	mustContain(t, screen(a), "tab", "command palette", "ctrl+c")

	a, _ = press(t, a, "?")
	mustNotContain(t, screen(a), "command palette")
}

func TestStartScreen(t *testing.T) {
	a := newApp(t, deck(), &recorder{}, Review, 80, 24)

	if a.Screen() != Review {
		t.Errorf("Screen() = %v, want Review", a.Screen())
	}
	mustContain(t, screen(a), "hello")
}

func TestRendersBeforeTheFirstWindowSize(t *testing.T) {
	a := NewApp(Config{Cards: deck(), Save: (&recorder{}).save, Now: (&testClock{fixedNow}).now})

	if !strings.Contains(screen(a), "Good evening") {
		t.Errorf("want Home at a default size:\n%s", screen(a))
	}
}

func TestColorsFollowTheTerminalBackground(t *testing.T) {
	a := newApp(t, deck(), &recorder{}, Home, 120, 24)
	if a.Init() == nil {
		t.Fatal("Init should ask the terminal for its background color")
	}
	dark := a.View().Content

	light := send(t, a, tea.BackgroundColorMsg{Color: color.White}).View().Content

	if dark == light {
		t.Error("a light background should change the colors")
	}
	if ansi.Strip(dark) != ansi.Strip(light) {
		t.Error("the background should change colors only, not text")
	}
}

func TestEnglishTargetIsBoldAndHighlighted(t *testing.T) {
	a := newApp(t, deck(), &recorder{}, Review, 80, 24)

	word := a.st.main.word
	if !word.GetBold() {
		t.Error("the highlighted English word should be bold")
	}
	if _, none := word.GetBackground().(lipgloss.NoColor); none {
		t.Error("the English word should sit on a highlight background")
	}
	if !strings.Contains(a.View().Content, word.Render(" hello ")) {
		t.Errorf("the English word should be rendered highlighted:\n%q", a.View().Content)
	}
}
