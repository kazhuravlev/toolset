package fsh_test

import (
	"testing"

	"github.com/kazhuravlev/toolset/internal/fsh"
	"github.com/stretchr/testify/require"
)

func TestExt(t *testing.T) {
	tests := []struct {
		name     string
		filename string
		exp      string
	}{
		{
			name:     "simple zip file",
			filename: "archive.zip",
			exp:      ".zip",
		},
		{
			name:     "tar.gz file",
			filename: "archive.tar.gz",
			exp:      ".tar.gz",
		},
		{
			name:     "tgz file",
			filename: "archive.tgz",
			exp:      ".tgz",
		},
		{
			name:     "tar.bz2 file",
			filename: "archive.tar.bz2",
			exp:      ".tar.bz2",
		},
		{
			name:     "tar.xz file",
			filename: "archive.tar.xz",
			exp:      ".tar.xz",
		},
		{
			name:     "with path",
			filename: "/path/to/archive.tar.gz",
			exp:      ".tar.gz",
		},
		{
			name:     "uppercase extension",
			filename: "archive.TAR.GZ",
			exp:      ".tar.gz",
		},
		{
			name:     "mixed case extension",
			filename: "archive.Tar.Gz",
			exp:      ".tar.gz",
		},
		{
			name:     "no extension",
			filename: "noext",
			exp:      "",
		},
		{
			name:     "hidden file with extension",
			filename: ".hidden.tar.gz",
			exp:      ".tar.gz",
		},
		{
			name:     "multiple dots",
			filename: "my.archive.file.tar.gz",
			exp:      ".tar.gz",
		},
		{
			name:     "exe file",
			filename: "tool.exe",
			exp:      ".exe",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := fsh.Ext(tt.filename)
			require.Equal(t, tt.exp, got)
		})
	}
}
