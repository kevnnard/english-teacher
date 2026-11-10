package teacher

import "strings"

// Kind is one kind of writing error, so the app can count the learner's
// recurring errors and aim the next task at them.
type Kind struct {
	Name  string // the identifier the model answers with, such as "be"
	Label string // a short English label for the learner
}

// OtherKind is the kind of every error that fits no other kind.
const OtherKind = "other"

// Kinds is the fixed list of error kinds, in the order the prompt lists them.
var Kinds = []Kind{
	{"be", "to be (am/is/are)"},
	{"verb-tense", "verb tenses"},
	{"verb-form", "verb forms (go/goes/going)"},
	{"agreement", "subject–verb agreement"},
	{"word-order", "word order"},
	{"article", "articles (a/an/the)"},
	{"preposition", "prepositions (in/on/at)"},
	{"plural", "plurals"},
	{"pronoun", "pronouns (he/him/his)"},
	{"word-choice", "word choice"},
	{"spelling", "spelling"},
	{"punctuation", "punctuation"},
	{OtherKind, "other details"},
}

// NormalizeKind returns the kind name s stands for: lowercase and trimmed,
// or OtherKind when s is not in Kinds.
func NormalizeKind(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	for _, k := range Kinds {
		if k.Name == s {
			return s
		}
	}
	return OtherKind
}

// KindLabel is the learner's label of kind name; unknown names get the
// label of OtherKind.
func KindLabel(name string) string {
	name = NormalizeKind(name)
	for _, k := range Kinds {
		if k.Name == name {
			return k.Label
		}
	}
	return name
}

// kindNames lists the kind names for a prompt.
func kindNames() string {
	names := make([]string, len(Kinds))
	for i, k := range Kinds {
		names[i] = k.Name
	}
	return strings.Join(names, ", ")
}
