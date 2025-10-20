package humanize_test

import (
	"testing"

	"github.com/kazhuravlev/toolset/internal/humanize"
	"github.com/stretchr/testify/require"
)

func TestHumanizeBytes(t *testing.T) {
	tests := []struct {
		name  string
		bytes int64
		want  string
	}{
		{
			name:  "zero bytes",
			bytes: 0,
			want:  "0 B",
		},
		{
			name:  "small bytes",
			bytes: 512,
			want:  "512 B",
		},
		{
			name:  "exactly 1 KB",
			bytes: 1024,
			want:  "1.0 KB",
		},
		{
			name:  "1.5 KB",
			bytes: 1536,
			want:  "1.5 KB",
		},
		{
			name:  "exactly 1 MB",
			bytes: 1024 * 1024,
			want:  "1.0 MB",
		},
		{
			name:  "2.5 MB",
			bytes: 1024*1024*2 + 1024*512,
			want:  "2.5 MB",
		},
		{
			name:  "exactly 1 GB",
			bytes: 1024 * 1024 * 1024,
			want:  "1.0 GB",
		},
		{
			name:  "3.7 GB",
			bytes: 1024*1024*1024*3 + 1024*1024*700,
			want:  "3.7 GB",
		},
		{
			name:  "exactly 1 TB",
			bytes: 1024 * 1024 * 1024 * 1024,
			want:  "1.0 TB",
		},
		{
			name:  "large cache size",
			bytes: 5*1024*1024*1024 + 256*1024*1024,
			want:  "5.2 GB",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := humanize.Bytes(tt.bytes)
			require.Equal(t, tt.want, got)
		})
	}
}
