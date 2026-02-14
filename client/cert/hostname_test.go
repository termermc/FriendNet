package cert

import "testing"

func TestNormalizeHostname(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		in   string
		want string
	}{
		{
			name: "empty string returns original",
			in:   "",
			want: "",
		},
		{
			name: "whitespace-only returns original",
			in:   "   \t\n",
			want: "   \t\n",
		},
		{
			name: "trims outer whitespace but returns normalized value",
			in:   "  ExAmPlE.COM  ",
			want: "example.com",
		},

		{
			name: "ipv4 canonical stays canonical",
			in:   "8.8.8.8",
			want: "8.8.8.8",
		},
		{
			name: "ipv4 with surrounding whitespace parses and normalizes",
			in:   "  8.8.8.8  ",
			want: "8.8.8.8",
		},
		{
			name: "ipv4 with zero-padded octets is not parsed by net.ParseIP; returns original",
			in:   "192.168.001.010",
			want: "192.168.001.010",
		},

		{
			name: "ipv6 bare compresses",
			in:   "2001:0DB8:0000:0000:0000:0000:0000:0001",
			want: "2001:db8::1",
		},
		{
			name: "ipv6 with brackets strips brackets",
			in:   "[2001:db8::1]",
			want: "2001:db8::1",
		},

		{
			name: "ipv6 zone lowercased and ip normalized",
			in:   "FE80::0001%EN0",
			want: "fe80::1%en0",
		},

		{
			name: "hostname lowercased",
			in:   "WWW.Example.COM",
			want: "www.example.com",
		},
		{
			name: "trailing dot removed",
			in:   "Example.COM.",
			want: "example.com",
		},
		{
			name: "unicode hostname to punycode (bücher.example)",
			in:   "Bücher.Example",
			want: "xn--bcher-kva.example",
		},

		{
			name: "host containing port is invalid for host-only; should return original",
			in:   "example.com:443",
			want: "example.com:443",
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := NormalizeHostname(tc.in)
			if got != tc.want {
				t.Fatalf("NormalizeHostname(%q) = %q; want %q", tc.in, got, tc.want)
			}
		})
	}
}
