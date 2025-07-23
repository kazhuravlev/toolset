package workdir

import (
	"fmt"
	"os"
	"path/filepath"
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

func getCacheDir() (string, error) {
	return getDirFromEnv("TOOLSET_CACHE_DIR", defaultCacheDir)
}

func getSpecDir() (string, error) {
	return getDirFromEnv("TOOLSET_SPEC_DIR", defaultSpecDir)
}

func getDirFromEnv(envName, defaultDir string) (string, error) {
	dir := defaultDir
	if specDirEnv := os.Getenv(envName); specDirEnv != "" {
		dir = specDirEnv
	}

	dir, err := expandTilde(dir)
	if err != nil {
		return "", fmt.Errorf("expand tilde: %w", err)
	}

	absResDir, err := filepath.Abs(dir)
	if err != nil {
		return "", fmt.Errorf("resolve dir: %w", err)
	}

	return absResDir, nil
}
