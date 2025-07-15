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
	cacheDir := defaultCacheDir
	if cacheDirEnv := os.Getenv("TOOLSET_CACHE_DIR"); cacheDirEnv != "" {
		cacheDir = cacheDirEnv
	}

	cacheDir, err := expandTilde(cacheDir)
	if err != nil {
		return "", fmt.Errorf("expand tilde: %w", err)
	}

	cacheDirAbs, err := filepath.Abs(cacheDir)
	if err != nil {
		return "", fmt.Errorf("resolve cache dir: %w", err)
	}

	return cacheDirAbs, nil
}
