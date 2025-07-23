package fsh

import (
	"github.com/spf13/afero"
	"path/filepath"
)

type FS interface {
	afero.Fs
	afero.Linker
	GetCurrentDir() string
	GetHomeDir() (string, error)
	Walk(root string, fn filepath.WalkFunc) error
}
