package fsh

import (
	"fmt"
	"github.com/spf13/afero"
	"os"
)

var _ afero.Linker = (*MemFS)(nil)

type MemFS struct {
	afero.Fs
}

func (m *MemFS) SymlinkIfPossible(oldname, newname string) error {
	return nil
}

func (m *MemFS) GetTree(dir string) ([]string, error) {
	res := make([]string, 0)
	err := afero.Walk(m, dir, func(path string, info os.FileInfo, err error) error {
		res = append(res, path)
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("walk dir: %w", err)
	}

	return res, nil
}

func NewMemFS(files map[string]string) *MemFS {
	fs := afero.NewMemMapFs()
	for path, content := range files {
		_ = afero.WriteFile(fs, path, []byte(content), 0o644)
	}

	return &MemFS{fs}
}
