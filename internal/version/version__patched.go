package version

import (
	"fmt"
	"log"
	"path/filepath"
)

func Install(dst, version string) error {
	log.SetFlags(0)

	root := Path(dst, version)

	if err := install(root, version); err != nil {
		return fmt.Errorf("%s: download failed: %w", version, err)
	}

	return nil
}

func Path(dst, version string) string {
	return filepath.Join(dst, version)
}
