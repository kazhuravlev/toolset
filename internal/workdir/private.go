package workdir

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/kazhuravlev/toolset/internal/fsh"
)

func expandTilde(path string) (string, error) {
	if strings.HasPrefix(path, "~") {
		home, err := os.UserHomeDir()
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

func getCacheDir(fs fsh.FS) (string, error) {
	return getDirFromEnv(fs, "TOOLSET_CACHE_DIR", defaultCacheDir)
}

func getSpecDir() string {
	if specDirEnv := os.Getenv("TOOLSET_SPEC_DIR"); specDirEnv != "" {
		return specDirEnv
	}

	return defaultSpecDir
}

func getDirFromEnv(fs fsh.FS, envName, defaultDir string) (string, error) {
	dir := defaultDir
	if specDirEnv := os.Getenv(envName); specDirEnv != "" {
		dir = specDirEnv
	}

	dir, err := expandTilde(dir)
	if err != nil {
		return "", fmt.Errorf("expand tilde: %w", err)
	}

	absResDir, err := fsh.Abs(fs, dir)
	if err != nil {
		return "", fmt.Errorf("resolve dir: %w", err)
	}

	return absResDir, nil
}

// Locations store location of any project-related files and dirs.
type Locations struct {
	ToolsetFile     string
	ToolsetLockFile string
	CacheDir        string
	ProjectRootDir  string
	CurrentDir      string
	StatsFile       string
}

func getLocations(fs fsh.FS, currentDir string, discovery bool) (*Locations, error) {
	cacheDir, err := getCacheDir(fs)
	if err != nil {
		return nil, fmt.Errorf("resolve cache dir: %w", err)
	}

	if err := fs.MkdirAll(cacheDir, fsh.DefaultDirPerm); err != nil {
		return nil, fmt.Errorf("create cache dir: %w", err)
	}

	specDir := getSpecDir()

	pathToSpec := filepath.Join(specDir, specFilename)
	dir := currentDir
	toolsetFilename := filepath.Join(dir, pathToSpec)
	if discovery {
		// Check that file is exists in current or parent directories.
		for {
			if !fsh.IsExists(fs, toolsetFilename) {
				dir = filepath.Dir(dir)
				if filepath.Dir(dir) == dir {
					return nil, errors.New("unable to find spec in fs tree")
				}

				toolsetFilename = filepath.Join(dir, pathToSpec)

				continue
			}

			break
		}
	}

	return &Locations{
		ToolsetFile:     toolsetFilename,
		ToolsetLockFile: filepath.Join(filepath.Dir(toolsetFilename), lockFilename),
		CacheDir:        cacheDir,
		StatsFile:       filepath.Join(cacheDir, statsFilename),
		ProjectRootDir:  dir,
		CurrentDir:      currentDir,
	}, nil
}
