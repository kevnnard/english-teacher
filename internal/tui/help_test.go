package tui

import (
	"strings"
	"testing"
)

func TestHelpShowsTheKeysOfTheCurrentScreen(t *testing.T) {
	tests := []struct {
		screen     Screen
		want, lack string
	}{
		{Learn, "mark or unmark the word", "reveal the answer"},
		{Write, "send your text to the teacher", "mark or unmark the word"},
		{Review, "reveal the answer", "send your text"},
		{Talk, "finish the role-play early", "send your text"},
		{Fluency, "end the round early", "finish the role-play early"},
		{Quiz, "choose an option", "end the round early"},
		{Listen, "hear it slower", "choose an option"},
	}
	for _, tt := range tests {
		x := newAIApp(t, tt.screen)
		x.App = send(t, x.App, key("?"))
		if tt.screen == Write { // ? is a letter while writing, so open help from the idle page
			mustContain(t, pageText(x.App), "Keys")
		}
		page := pageText(x.App)
		mustContain(t, page, "Everywhere")
		mustContain(t, page, tt.want)
		if strings.Contains(page, tt.lack) {
			t.Errorf("%v help shows %q", tt.screen, tt.lack)
		}
	}
}

func TestHelpFitsTheSmallestTerminal(t *testing.T) {
	for _, s := range screens {
		a := newApp(t, deck(), &recorder{}, s, 80, 24)
		a, _ = press(t, a, "?")
		if !strings.Contains(pageText(a), "ctrl+c") {
			t.Errorf("%v: help is cut at 80x24", s)
		}
	}
}
