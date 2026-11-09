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

// hasRail reports whether the navigation rail is on screen: its items
// sit in the first 26 columns.
func hasRail(a App) bool {
	for _, l := range screenLines(a) {
		if strings.HasPrefix(strings.TrimSpace(l), "Learn") || strings.HasPrefix(strings.TrimSpace(l), "Stats") {
			return true
		}
	}
	return false
}

func hasCoach(a App) bool { return strings.Contains(screen(a), "Coach") }

func TestLayoutFollowsWidth(t *testing.T) {
	tests := []struct {
		width     int
		wantRail  bool
		wantCoach bool
	}{
		{width: 60},
		{width: 80},
		{width: 99},
		{width: 100, wantRail: true},
		{width: 120, wantRail: true},
		{width: 139, wantRail: true},
		{width: 140, wantRail: true, wantCoach: true},
		{width: 160, wantRail: true, wantCoach: true},
	}
	for _, tt := range tests {
		t.Run(fmt.Sprint(tt.width), func(t *testing.T) {
			a := newApp(t, deck(), &recorder{}, Home, tt.width, 24)

			if got := hasRail(a); got != tt.wantRail {
				t.Errorf("rail = %v, want %v:\n%s", got, tt.wantRail, screen(a))
			}
			if got := hasCoach(a); got != tt.wantCoach {
				t.Errorf("coach = %v, want %v:\n%s", got, tt.wantCoach, screen(a))
			}
		})
	}
}

func TestRailIs26ColumnsWide(t *testing.T) {
	a := newApp(t, deck(), &recorder{}, Home, 120, 24)

	for i, l := range screenLines(a)[:23] {
		r := []rune(l)
		if len(r) < 26 || r[25] != '│' {
			t.Fatalf("line %d: want the rail rule in column 26:\n%q", i, l)
		}
	}
}

func TestCoachIs44ColumnsWide(t *testing.T) {
	a := newApp(t, deck(), &recorder{}, Home, 160, 24)

	for i, l := range screenLines(a)[:23] {
		r := []rune(l)
		if len(r) != 160 || r[160-44] != '│' {
			t.Fatalf("line %d: want the coach rule 44 columns from the right edge:\n%q", i, l)
		}
	}
}

func TestFocusLayoutShowsProgressLineOnTop(t *testing.T) {
	a := newApp(t, deck(), &recorder{}, Review, 80, 24)

	top := screenLines(a)[0]
	if !strings.Contains(top, "━") && !strings.Contains(top, "─") {
		t.Errorf("first line should be the progress line, got %q", top)
	}
	if hasRail(a) {
		t.Error("focus layout should have no rail")
	}
}

func TestLayoutIsRecomputedOnResize(t *testing.T) {
	a := newApp(t, deck(), &recorder{}, Home, 160, 24)
	if !hasCoach(a) {
		t.Fatal("want coach at 160")
	}

	a = send(t, a, tea.WindowSizeMsg{Width: 80, Height: 24})
	if hasRail(a) || hasCoach(a) {
		t.Errorf("after shrinking to 80 want focus layout:\n%s", screen(a))
	}

	a = send(t, a, tea.WindowSizeMsg{Width: 120, Height: 30})
	if !hasRail(a) || hasCoach(a) {
		t.Errorf("after growing to 120 want rail only:\n%s", screen(a))
	}
	if n := len(screenLines(a)); n != 30 {
		t.Errorf("view has %d lines, want the new height 30", n)
	}
}

func TestCoachToggle(t *testing.T) {
	a := newApp(t, deck(), &recorder{}, Home, 160, 24)

	a, _ = press(t, a, "c")
	if hasCoach(a) {
		t.Errorf("c should hide the coach:\n%s", screen(a))
	}
	a, _ = press(t, a, "c")
	if !hasCoach(a) {
		t.Errorf("c again should show the coach:\n%s", screen(a))
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
	want := []Screen{Review, Learn, Write, Words, Stats, Home}
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
	v := screen(a)
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

func TestPlaceholderScreens(t *testing.T) {
	for _, s := range []Screen{Learn, Write} {
		a := newApp(t, deck(), &recorder{}, s, 80, 24)

		mustContain(t, screen(a), "Coming in M3")
	}
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

func TestEnglishTargetIsBold(t *testing.T) {
	a := newApp(t, deck(), &recorder{}, Review, 80, 24)

	bold := lipgloss.NewStyle().Bold(true).Render("hello")

	if !strings.Contains(a.View().Content, bold) {
		t.Errorf("the English word should be rendered bold:\n%q", a.View().Content)
	}
}
