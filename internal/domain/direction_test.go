package domain

import "testing"

func TestDirection(t *testing.T) {
	var zero Direction
	if zero != Recognition {
		t.Error("the zero Direction should be Recognition, so existing cards stay recognition")
	}
	for _, d := range []Direction{Recognition, Production} {
		got, err := ParseDirection(d.String())
		if err != nil || got != d {
			t.Errorf("ParseDirection(%q) = %v, %v", d.String(), got, err)
		}
	}
	if Recognition.String() != "recognition" || Production.String() != "production" {
		t.Errorf("names = %q, %q", Recognition, Production)
	}
	if _, err := ParseDirection("sideways"); err == nil {
		t.Error("ParseDirection should reject unknown names")
	}
}
