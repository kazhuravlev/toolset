package workdir

import (
	"fmt"
	"github.com/kazhuravlev/toolset/internal/fsh"
	"os"
	"strings"
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
