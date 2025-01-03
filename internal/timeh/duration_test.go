package timeh

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestDuration(t *testing.T) {
	tests := []struct {
		name string
		in   time.Duration
		exp  string
	}{
		{
			name: "negative_duration",
			in:   -1 * time.Second,
			exp:  "-1s",
		},
		{
			name: "negative_duration_2",
			in:   -1 * time.Hour,
			exp:  "-1h",
		},
		{
			name: "zero_duration",
			in:   0,
			exp:  "0s",
		},
		{
			name: "less_than_second",
			in:   time.Second - 1,
			exp:  "1s", // FIXME: it should be 0s actually
		},
		{
			name: "less_than_minute",
			in:   time.Minute - 1,
			exp:  "59s",
		},
		{
			name: "less_than_hour",
			in:   time.Hour - 1,
			exp:  "59m 59s",
		},
		{
			name: "less_than_day",
			in:   24*time.Hour - 1,
			exp:  "23h 59m 59s",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			res := Duration(tt.in)
			require.Equal(t, tt.exp, res)
		})
	}
}
