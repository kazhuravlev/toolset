package fsh

import (
	"context"
	"path/filepath"

	"github.com/spf13/afero"
)

type FS interface {
	afero.Fs
	afero.Linker
	GetCurrentDir() string
	GetHomeDir() (string, error)
	Walk(root string, fn filepath.WalkFunc) error
	RLock(ctx context.Context, filename string) (func(), error)
	Lock(ctx context.Context, filename string) (func(), error)
}
