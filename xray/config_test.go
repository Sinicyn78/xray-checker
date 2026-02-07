package xray

import "testing"

func TestNormalizeStreamSecurity(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{in: "", want: "none"},
		{in: "none", want: "none"},
		{in: "false", want: "none"},
		{in: "0", want: "none"},
		{in: "tls", want: "tls"},
		{in: "reality", want: "reality"},
		{in: "unknown", want: "none"},
	}

	for _, tc := range cases {
		got := normalizeStreamSecurity(tc.in, "test")
		if got != tc.want {
			t.Fatalf("normalizeStreamSecurity(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}
