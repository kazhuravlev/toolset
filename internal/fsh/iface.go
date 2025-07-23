package fsh

import (
	"path/filepath"

	"github.com/spf13/afero"
)

type FS interface {
	afero.Fs
	afero.Linker
	GetCurrentDir() string
	GetHomeDir() (string, error)
	Walk(root string, fn filepath.WalkFunc) error
}
