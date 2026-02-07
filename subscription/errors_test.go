package subscription

import (
	"errors"
	"os"
	"testing"
)

func TestShouldTreatAsEmptyResult(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want bool
	}{
		{
			name: "nil",
			err:  nil,
			want: false,
		},
		{
			name: "not exists",
			err:  os.ErrNotExist,
			want: true,
		},
		{
			name: "empty folder parse",
			err:  errors.New("no valid proxy configurations found in folder"),
			want: true,
		},
		{
			name: "missing folder",
			err:  errors.New("failed to read folder: open /tmp/foo: no such file or directory"),
			want: true,
		},
		{
			name: "missing file",
			err:  errors.New("error reading file: open /tmp/foo.txt: no such file or directory"),
			want: true,
		},
		{
			name: "network error",
			err:  errors.New("request failed: 500"),
			want: false,
		},
	}

	for _, tc := range cases {
		if got := ShouldTreatAsEmptyResult(tc.err); got != tc.want {
			t.Fatalf("%s: got %v, want %v", tc.name, got, tc.want)
		}
	}
}
