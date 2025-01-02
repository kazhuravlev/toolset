package fsh

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/afero"
)

const DefaultDirPerm = 0o755

type FS interface {
	afero.Fs
	afero.Symlinker
}

func NewOSFS() FS {
	return afero.NewOsFs().(*afero.OsFs)
}

func IsExists(fSys FS, path string) bool {
	if _, err := fSys.Stat(path); os.IsNotExist(err) {
		return false
	}

	return true
}

func ReadJson[T any](fSys FS, path string) (*T, error) {
	bb, err := afero.ReadFile(fSys, path)
	if err != nil {
		return nil, fmt.Errorf("read file (%s): %w", path, err)
	}

	var res T
	if err := json.Unmarshal(bb, &res); err != nil {
		return nil, fmt.Errorf("parse file (%s): %w", path, err)
	}

	return &res, nil
}

func WriteJson(fSys FS, in any, path string) error {
	file, err := fSys.OpenFile(path, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o644)
	if err != nil {
		return fmt.Errorf("open file: %w", err)
	}

	enc := json.NewEncoder(file)
	enc.SetIndent("", "\t")

	if err := enc.Encode(in); err != nil {
		return fmt.Errorf("marshal file: %w", err)
	}

	if err := file.Close(); err != nil {
		return fmt.Errorf("close file: %w", err)
	}

	return nil
}

func ForceReadJson[T any](fs FS, path string, defVal T) (*T, error) {
	if !IsExists(fs, path) {
		if err := fs.MkdirAll(filepath.Dir(path), DefaultDirPerm); err != nil {
			return nil, fmt.Errorf("mkdir: %w", err)
		}

		if err := WriteJson(fs, defVal, path); err != nil {
			return nil, fmt.Errorf("write json to file: %w", err)
		}
	}

	res, err := ReadJson[T](fs, path)
	if err != nil {
		return nil, fmt.Errorf("read json: %w", err)
	}

	return res, nil
}
