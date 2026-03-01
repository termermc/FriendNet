package common

import "testing"

var escapeQueryStringTests = []struct {
	name   string
	input  string
	output string
}{
	{
		name:   "removes special characters",
		input:  `foo+bar^baz*qux(quux){corge}[grault]-garply,waldo`,
		output: `foo bar baz qux quux  corge  grault  garply waldo`,
	},
	{
		name:   "removes unclosed quote",
		input:  `foo "bar baz`,
		output: `foo  bar baz`,
	},
	{
		name:   "unclosed quote only",
		input:  `"`,
		output: ` `,
	},
	{
		name:   "closed quote only",
		input:  `""`,
		output: `""`,
	},
	{
		name:   "keeps closed quotes",
		input:  `foo "bar" baz`,
		output: `foo "bar" baz`,
	},
	{
		name:   "removes last unclosed quote only",
		input:  `foo "bar" "baz`,
		output: `foo "bar"  baz`,
	},
	{
		name:   "lowercases FTS5 keywords",
		input:  `foo NEAR bar AND baz OR qux NOT quux`,
		output: "foo nEAR bar aND baz oR qux nOT quux",
	},
	{
		name:   "mixed case and special",
		input:  `foo+NEAR(bar) AND[qux]`,
		output: "foo nEAR bar  aND qux ",
	},
	{
		name:   "empty string",
		input:  ``,
		output: ``,
	},
	{
		name:   "only special characters",
		input:  `+^*()-{},:`,
		output: `          `,
	},
	{
		name:   "colon at end",
		input:  `foo:`,
		output: `foo `,
	},
	{
		name:   "NEAR at end",
		input:  `NEAR`,
		output: `nEAR`,
	},
	{
		name:   "colon only",
		input:  `:`,
		output: ` `,
	},
	{
		name:   "question mark",
		input:  `?`,
		output: ` `,
	},
	{
		name:   "test all keywords",
		input:  `there are NOT any stores NEAR, do you want this OR that AND those?`,
		output: `there are nOT any stores nEAR  do you want this oR that aND those `,
	},
}

func TestEscapeQueryString(t *testing.T) {
	for _, tt := range escapeQueryStringTests {
		t.Run(tt.name, func(t *testing.T) {
			got := EscapeQueryString(tt.input)
			if got != tt.output {
				t.Errorf("EscapeQueryString(%q) = %q; want %q", tt.input, got, tt.output)
			}
		})
	}
}

func BenchmarkEscapeQueryString(b *testing.B) {
	for i := 0; i < b.N; i++ {
		for _, s := range escapeQueryStringTests {
			_ = EscapeQueryString(s.input)
		}
	}
}
