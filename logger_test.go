package sqlxx

import (
	"bufio"
	"bytes"
	"context"
	"strings"
	"testing"
)

func TestLogger(t *testing.T) {
	tests := []struct {
		format string
		args   []interface{}
		want   string
	}{
		{"Hello", []interface{}{}, "Hello"},
		{"Hello %s", []interface{}{"Alice"}, "Hello Alice"},
		{"Hello %s %d", []interface{}{"Alice", 2}, "Hello Alice 2"},
	}

	var buf bytes.Buffer
	l := NewLogger(&buf)
	ctx := context.Background()

	for _, tt := range tests {
		l.Debugf(ctx, tt.format, tt.args...)
		l.Infof(ctx, tt.format, tt.args...)
		l.Warnf(ctx, tt.format, tt.args...)
		l.Errorf(ctx, tt.format, tt.args...)

		var got string
		scanner := bufio.NewScanner(&buf)

		// Debugf
		scanner.Scan()
		got = scanner.Text()
		if !strings.HasPrefix(got, "[DEBUG]") {
			t.Errorf("prefix does not match [DEBUG]")
		}
		if !strings.HasSuffix(got, tt.want) {
			t.Errorf("suffix does not match %s", tt.want)
		}

		// Infof
		scanner.Scan()
		got = scanner.Text()
		if !strings.HasPrefix(got, "[INFO]") {
			t.Errorf("prefix does not match [INFO]")
		}
		if !strings.HasSuffix(got, tt.want) {
			t.Errorf("suffix does not match %s", tt.want)
		}

		// Warnf
		scanner.Scan()
		got = scanner.Text()
		if !strings.HasPrefix(got, "[WARN]") {
			t.Errorf("prefix does not match [WARN]")
		}
		if !strings.HasSuffix(got, tt.want) {
			t.Errorf("suffix does not match %s", tt.want)
		}

		// Errorf
		scanner.Scan()
		got = scanner.Text()
		if !strings.HasPrefix(got, "[ERROR]") {
			t.Errorf("prefix does not match [ERROR]")
		}
		if !strings.HasSuffix(got, tt.want) {
			t.Errorf("suffix does not match %s", tt.want)
		}
	}
}
