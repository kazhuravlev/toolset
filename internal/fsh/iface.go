package fsh

import "github.com/spf13/afero"

type FS interface {
	afero.Fs
	afero.Linker
	GetCurrentDir() string
}
