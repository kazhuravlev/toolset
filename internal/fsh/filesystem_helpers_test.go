package fsh_test

import (
	"github.com/kazhuravlev/toolset/internal/fsh"
	"github.com/spf13/afero"
	"github.com/stretchr/testify/require"
	"testing"
)

var _ afero.Linker = (*MemFS)(nil)

type MemFS struct {
	afero.Fs
}

func (m *MemFS) SymlinkIfPossible(oldname, newname string) error {
	return nil
}

func NewFS() fsh.FS {
	return &MemFS{afero.NewMemMapFs()}
}

func TestIsExists(t *testing.T) {
	fs := NewFS()
	require.False(t, fsh.IsExists(fs, "/not/exists/path"))

	require.NoError(t, afero.WriteFile(fs, "/foo/bar", []byte("foo"), 0644))
	require.True(t, fsh.IsExists(fs, "/foo/bar"))
}
