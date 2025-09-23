package fsh

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"github.com/spf13/afero"
)

var _ FS = (*MemFS)(nil)

type MemFS struct {
	afero.Fs
	lockMu *sync.Mutex
}

func NewMemFS(files map[string]string) *MemFS {
	fs := afero.NewMemMapFs()
	for path, content := range files {
		_ = afero.WriteFile(fs, path, []byte(content), 0o644)
	}

	return &MemFS{
		Fs:     fs,
		lockMu: new(sync.Mutex),
	}
}

func (m *MemFS) GetCurrentDir() string {
	return "/"
}

func (m *MemFS) GetHomeDir() (string, error) {
	return "/home-dir", nil
}

func (m *MemFS) Walk(root string, fn filepath.WalkFunc) error {
	return afero.Walk(m.Fs, root, fn)
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

func (m *MemFS) RLock(_ context.Context, _ string) (func(), error) {
	m.lockMu.Lock()
	return m.lockMu.Unlock, nil
}

func (m *MemFS) Lock(_ context.Context, _ string) (func(), error) {
	m.lockMu.Lock()
	return m.lockMu.Unlock, nil
}
