package fsh_test

import (
	"runtime"
	"testing"

	"github.com/kazhuravlev/toolset/internal/fsh"
	"github.com/stretchr/testify/require"
)

func TestMemFS_GetTree(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("skip for Windows")
	}

	fs := fsh.NewMemFS(map[string]string{
		"/dir/file1.txt":      `Hello 1!`,
		"/dir/file2.txt":      `Hello 2!`,
		"/dir/dir2/file3.txt": `Hello 3!`,
	})

	tree, err := fs.GetTree("/dir")
	require.NoError(t, err)
	require.Equal(t, []string{
		"/dir",
		"/dir/dir2",
		"/dir/dir2/file3.txt",
		"/dir/file1.txt",
		"/dir/file2.txt",
	}, tree)
}

func TestMemFS_SymlinkIfPossible(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("skip for Windows")
	}

	fs := fsh.NewMemFS(nil)

	require.NoError(t, fs.SymlinkIfPossible("/tmp/a", "/tmp/a"))
}
