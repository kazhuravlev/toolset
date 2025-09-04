package envh

import (
	"os"
)

// GetAllOverride will set some envs and return all envs together.
func GetAllOverride(envs [][2]string) []string {
	for _, pair := range envs {
		key := pair[0]
		val := pair[1]
		os.Setenv(key, val)
	}

	return os.Environ()
}
