package check

import (
	"reflect"
	"testing"
)

// marks writes the result of Understood as "word" for understood words
// and "!word" for the others, so a whole sentence fits in one string.
func marks(ms []WordMark) []string {
	out := make([]string, len(ms))
	for i, m := range ms {
		out[i] = m.Word
		if !m.Understood {
			out[i] = "!" + m.Word
		}
	}
	return out
}

func TestUnderstood(t *testing.T) {
	tests := []struct {
		name                 string
		expected, transcript string
		want                 []string
	}{
		{"full match ignores case and punctuation",
			"My name is Anna.", " my name is anna", []string{"My", "name", "is", "Anna."}},
		{"a missing word",
			"I am from Argentina.", "I from Argentina.", []string{"I", "!am", "from", "Argentina."}},
		{"a substituted word: sheep is not ship",
			"The ship is big.", "The sheep is big.", []string{"The", "!ship", "is", "big."}},
		{"digits equal number words",
			"I am thirty years old.", "I am 30 years old.", []string{"I", "am", "thirty", "years", "old."}},
		{"number words equal digits",
			"She is 25.", "She is twenty-five.", []string{"She", "is", "25."}},
		{"extra words in the transcript are ignored",
			"Nice to meet you.", "Oh, nice to meet you too, really.", []string{"Nice", "to", "meet", "you."}},
		{"an empty transcript understands nothing",
			"Hello, how are you?", "", []string{"!Hello,", "!how", "!are", "!you?"}},
		{"British and American spellings are the same word",
			"My favourite colour is grey.", "My favorite color is gray.", []string{"My", "favourite", "colour", "is", "grey."}},
		{"a doubled letter is the same word",
			"My name is Ana.", "My name is Anna.", []string{"My", "name", "is", "Ana."}},
		{"a changed vowel is another word, even in a long word",
			"The ship is big.", "The shop is big.", []string{"The", "!ship", "is", "big."}},
		{"a changed vowel in a short word is another word",
			"I like my cat.", "I like my cut.", []string{"I", "like", "my", "!cat."}},
		{"a dropped final s is not understood",
			"She lives here.", "She live here.", []string{"She", "!lives", "here."}},
		{"a dropped -ed is not understood",
			"I worked today.", "I work today.", []string{"I", "!worked", "today."}},
		{"apostrophes count; curly ones are straight",
			"I’m Kevin.", "Im Kevin.", []string{"!I’m", "Kevin."}},
		{"an e before s and a consonant is the Spanish e, not a typo",
			"The state is big.", "The estate is big.", []string{"The", "!state", "is", "big."}},
		{"word order matters",
			"I am here.", "Here I am.", []string{"I", "am", "!here."}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := marks(Understood(tt.expected, tt.transcript))
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("Understood(%q, %q) = %q, want %q", tt.expected, tt.transcript, got, tt.want)
			}
		})
	}
}

func TestUnderstoodKeepsPunctuationOnlyTokens(t *testing.T) {
	got := marks(Understood("Hello — Anna!", "hello anna"))
	if want := []string{"Hello", "—", "Anna!"}; !reflect.DeepEqual(got, want) {
		t.Errorf("got %q, want %q: a dash has nothing to say", got, want)
	}
}

func TestWordKeyNumbers(t *testing.T) {
	for in, want := range map[string]string{
		"zero": "0", "Thirty-Five": "35", "100": "100", "hundred": "100", "30.": "30",
		"ninety-nine": "99", "twelve,": "12", "101": "101", "it's": "it's", "well-known": "wellknown",
	} {
		if got := wordKey(in); got != want {
			t.Errorf("wordKey(%q) = %q, want %q", in, got, want)
		}
	}
}
