package gsqlutils

import "testing"

func TestStripComments(t *testing.T) {
	for _, test := range []struct {
		desc  string
		input string
		want  string
	}{
		{desc: "no comment", input: "SELECT 1", want: "SELECT 1"},
		{desc: "line comment before EOF", input: "SELECT 1 // comment", want: "SELECT 1 "},
		{desc: "line comment", input: "SELECT 1// comment \n+ 2", want: "SELECT 1\n+ 2"},
		{desc: "inline multiline comment", input: `SELECT 1/**/+ 2`, want: "SELECT 1 + 2"},
		{desc: "multiline comment", input: "SELECT 1/*\n*/+ 2", want: "SELECT 1\n+ 2"},
	} {
		t.Run(test.desc, func(t *testing.T) {
			got, err := StripComments("", test.input)
			if err != nil {
				t.Errorf("StripComments() error = %v", err)
				return
			}
			if got != test.want {
				t.Errorf("StripComments() = %q, want %q", got, test.want)
			}
		})
	}
}
