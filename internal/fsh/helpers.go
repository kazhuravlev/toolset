package fsh

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/afero"
)

func IsExists(fSys FS, path string) bool {
	exists, err := afero.Exists(fSys, path)
	if err != nil {
		return false
	}

	return exists
}

func Abs(fSys FS, path string) (string, error) {
	if filepath.IsAbs(path) {
		return filepath.Clean(path), nil
	}

	return filepath.Join(fSys.GetCurrentDir(), path), nil
}

func ReadJson[T any](ctx context.Context, fs FS, path string) (*T, error) {
	{
		unlock, err := fs.RLock(ctx, path)
		if err != nil {
			return nil, fmt.Errorf("rlock file: %w", err)
		}

		defer unlock()
	}

	bb, err := afero.ReadFile(fs, path)
	if err != nil {
		return nil, fmt.Errorf("read file (%s): %w", path, err)
	}

	var res T
	if err := json.Unmarshal(bb, &res); err != nil {
		return nil, fmt.Errorf("parse file (%s): %w", path, err)
	}

	return &res, nil
}

func WriteJson(ctx context.Context, fs FS, in any, path string) error {
	{
		unlock, err := fs.Lock(ctx, path)
		if err != nil {
			return fmt.Errorf("lock file: %w", err)
		}

		defer unlock()
	}

	file, err := fs.OpenFile(path, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o644)
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

func ReadOrCreateJson[T any](ctx context.Context, fs FS, path string, defaultVal T) (*T, error) {
	if !IsExists(fs, path) {
		if err := fs.MkdirAll(filepath.Dir(path), DefaultDirPerm); err != nil {
			return nil, fmt.Errorf("mkdir: %w", err)
		}

		if err := WriteJson(ctx, fs, defaultVal, path); err != nil {
			return nil, fmt.Errorf("write json to file: %w", err)
		}
	}

	res, err := ReadJson[T](ctx, fs, path)
	if err != nil {
		return nil, fmt.Errorf("read json: %w", err)
	}

	return res, nil
}

func ExpandTilde(fs FS, path string) (string, error) {
	if strings.HasPrefix(path, "~") {
		home, err := fs.GetHomeDir()
		if err != nil {
			return "", fmt.Errorf("get user hoeme dir: %w", err)
		}

		if path == "~" {
			return home, nil
		}

		if strings.HasPrefix(path, "~/") {
			return home + path[1:], nil
		}
	}

	return path, nil
}

func DirSize(fSys FS, path string) (int64, error) {
	var size int64

	err := fSys.Walk(path, func(path string, d fs.FileInfo, err error) error {
		if err != nil {
			return err
		}

		if !d.IsDir() {
			size += d.Size()
		}

		return nil
	})

	return size, err
}

// FirstDir returns first directory name in given directory.
func FirstDir(fSys FS, dir string) (string, error) {
	entries, err := afero.ReadDir(fSys, dir)
	if err != nil {
		return "", err
	}

	for _, e := range entries {
		if e.IsDir() {
			return e.Name(), nil
		}
	}

	return "", errors.New("no directory found")
}

// Ext works like filepath.Ext but supports .tar.gz extensions.
func Ext(filename string) string {
	base := filepath.Base(filename)
	base = strings.ToLower(base)
	last6chars := base[max(len(base)-len(".tar.lzma"), 0):]

	_, ext, ok := strings.Cut(last6chars, ".")
	if !ok {
		return ""
	}

	return "." + ext
}

// SetExecutable mark file as executable.
func SetExecutable(fSys FS, path string) error {
	info, err := fSys.Stat(path)
	if err != nil {
		return err
	}

	mode := info.Mode().Perm()

	// 0o111 == execute for user/group/other

	return fSys.Chmod(path, mode|0o111)
}
