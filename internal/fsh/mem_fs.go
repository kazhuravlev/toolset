package fsh

import "github.com/spf13/afero"

var _ afero.Linker = (*MemFS)(nil)

type MemFS struct {
	afero.Fs
}

func (m *MemFS) SymlinkIfPossible(oldname, newname string) error {
	return nil
}

func NewMemFS(files map[string]string) *MemFS {
	fs := afero.NewMemMapFs()
	for path, content := range files {
		_ = afero.WriteFile(fs, path, []byte(content), 0o644)
	}

	return &MemFS{fs}
}
