package common

import "testing"

func TestParseGoOctalLiteral_OK(t *testing.T) {
	t.Parallel()

	tests := []struct {
		in   string
		want uint32
	}{
		{"0", 0},
		{"00", 0},
		{"0000", 0},

		{"7", 7},
		{"07", 7},

		{"10", 010},
		{"010", 010},

		{"755", 0755},
		{"0755", 0755},

		{"777", 0777},
		{"0777", 0777},

		{"644", 0644},
		{"0644", 0644},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.in, func(t *testing.T) {
			t.Parallel()

			got, err := ParseGoOctalLiteral(tt.in)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tt.want {
				t.Fatalf("got %d, want %d", got, tt.want)
			}
		})
	}
}

func TestParseGoOctalLiteral_Errors(t *testing.T) {
	t.Parallel()

	tests := []string{
		"",
		"8",
		"09",
		"0789",
		"0x755",
		"755\n",
		" 0755",
		"0755 ",
		"-755",
		"+755",
		"7.55",
		"0755_",
		"_755",
	}

	for _, in := range tests {
		in := in
		t.Run(in, func(t *testing.T) {
			t.Parallel()

			_, err := ParseGoOctalLiteral(in)
			if err == nil {
				t.Fatalf("expected error for input %q, got nil", in)
			}
		})
	}
}
