package tui

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kevnnard/english-teacher/internal/domain"
)

var update = flag.Bool("update", false, "rewrite the golden files in testdata/")

// TestGolden keeps a plain-text snapshot of each key screen at the three
// reference widths, so layout changes show up in review as a text diff.
// Run `go test ./internal/tui -run Golden -update` after an intended change.
func TestGolden(t *testing.T) {
	cases := []struct {
		name  string
		cards []domain.Card
		start Screen
		keys  []string
	}{
		{name: "home", cards: deck(), start: Home},
		{name: "review", cards: deck(), start: Review, keys: []string{"space"}},
		{name: "review-almost", cards: prodDeck(), start: Review,
			keys: strings.Split("n i c e space t o space m e a t space y o u enter", " ")},
		{name: "review-done", cards: deck(), start: Review,
			keys: []string{"space", "1", "space", "1", "space", "3", "space", "3", "space", "4"}},
		{name: "listen-no-audio", cards: deck(), start: Listen},
	}
	for _, c := range cases {
		for _, width := range []int{80, 120, 160} {
			name := fmt.Sprintf("%s-%d", c.name, width)
			t.Run(name, func(t *testing.T) {
				a := newApp(t, c.cards, &recorder{}, c.start, width, 24)
				a, _ = press(t, a, c.keys...)
				got := trimRight(screen(a))

				path := filepath.Join("testdata", name+".golden")
				if *update {
					if err := os.WriteFile(path, []byte(got), 0o644); err != nil {
						t.Fatal(err)
					}
				}
				want, err := os.ReadFile(path)
				if err != nil {
					t.Fatalf("%v (run with -update to create it)", err)
				}
				if got != string(want) {
					t.Errorf("view differs from %s:\n--- got ---\n%s\n--- want ---\n%s", path, got, want)
				}
			})
		}
	}
}

// trimRight drops trailing spaces so golden files stay readable.
func trimRight(s string) string {
	lines := strings.Split(s, "\n")
	for i, l := range lines {
		lines[i] = strings.TrimRight(l, " ")
	}
	return strings.Join(lines, "\n") + "\n"
}
