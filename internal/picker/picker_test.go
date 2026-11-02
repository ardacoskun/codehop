package picker

import (
	"bytes"
	"errors"
	"strings"
	"testing"

	"github.com/ardacoskun/codehop/internal/discover"
)

func TestPickNumbered(t *testing.T) {
	projects := []discover.Project{{Key: "a", Path: "/a"}, {Key: "b", Path: "/b"}}
	tests := []struct {
		input   string
		want    string
		wantErr error
	}{
		{"2\n", "b", nil},
		{" 1 \n", "a", nil},
		{"\n", "", ErrCanceled},
		{"", "", ErrCanceled},
	}
	for _, tt := range tests {
		var out bytes.Buffer
		p, err := pickNumbered(projects, strings.NewReader(tt.input), &out)
		if !errors.Is(err, tt.wantErr) || p.Key != tt.want {
			t.Errorf("input %q: got %q, %v; want %q, %v", tt.input, p.Key, err, tt.want, tt.wantErr)
		}
	}
	for _, bad := range []string{"0\n", "3\n", "x\n"} {
		if _, err := pickNumbered(projects, strings.NewReader(bad), &bytes.Buffer{}); err == nil || errors.Is(err, ErrCanceled) {
			t.Errorf("input %q should be rejected, got %v", bad, err)
		}
	}
}
