package humanize_test

import (
	"testing"

	"github.com/kazhuravlev/toolset/internal/humanize"
	"github.com/stretchr/testify/require"
)

func TestHumanizeBytes(t *testing.T) {
	tests := []struct {
		bytes int64
		want  string
	}{
		{
			bytes: 0,
			want:  "0 B",
		},
		{
			bytes: 512,
			want:  "512 B",
		},
		{
			bytes: 1024,
			want:  "1.0 KB",
		},
		{
			bytes: 1536,
			want:  "1.5 KB",
		},
		{
			bytes: 1024 * 1024,
			want:  "1.0 MB",
		},
		{
			bytes: 1024*1024*2 + 1024*512,
			want:  "2.5 MB",
		},
		{
			bytes: 1024 * 1024 * 1024,
			want:  "1.0 GB",
		},
		{
			bytes: 1024*1024*1024*3 + 1024*1024*700,
			want:  "3.7 GB",
		},
		{
			bytes: 1024 * 1024 * 1024 * 1024,
			want:  "1.0 TB",
		},
		{
			bytes: 5*1024*1024*1024 + 256*1024*1024,
			want:  "5.2 GB",
		},
	}

	for _, tt := range tests {
		t.Run("", func(t *testing.T) {
			got := humanize.Bytes(tt.bytes)
			require.Equal(t, tt.want, got)
		})
	}
}
