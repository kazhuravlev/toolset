package workdir

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/kazhuravlev/toolset/internal/fsh"
)

const (
	EnvCacheDir = "TOOLSET_CACHE_DIR"
	EnvSpecDir  = "TOOLSET_SPEC_DIR"
)

func getCacheDir(fs fsh.FS) (string, error) {
	return getDirFromEnv(fs, EnvCacheDir, defaultCacheDir)
}

func getSpecDir() string {
	if specDirEnv := os.Getenv(EnvSpecDir); specDirEnv != "" {
		return specDirEnv
	}

	return defaultSpecDir
}

func getDirFromEnv(fs fsh.FS, envName, defaultDir string) (string, error) {
	dir := defaultDir
	if specDirEnv := os.Getenv(envName); specDirEnv != "" {
		dir = specDirEnv
	}

	dir, err := fsh.ExpandTilde(fs, dir)
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
	currentDir, err := fsh.Abs(fs, currentDir)
	if err != nil {
		return nil, fmt.Errorf("absolute current dir: %w", err)
	}

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
