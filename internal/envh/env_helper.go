package envh

import (
	"fmt"
	"os"
	"strings"
)

func Unique(envs [][2]string) []string {
	sysEnvs := os.Environ()
	out := make([]string, 0, len(envs)+len(sysEnvs))
	saw := make(map[string]struct{}, len(envs))
	for _, pair := range envs {
		key := pair[0]
		val := pair[1]
		if _, ok := saw[key]; ok {
			continue
		}

		saw[key] = struct{}{}
		out = append(out, fmt.Sprintf("%s=%s", key, val))
	}

	for _, kv := range sysEnvs {
		key, val, _ := strings.Cut(kv, "=")
		if _, ok := saw[key]; ok {
			continue
		}

		if strings.HasPrefix(key, "GO") {
			continue
		}

		saw[key] = struct{}{}
		out = append(out, fmt.Sprintf("%s=%s", key, val))
	}

	return out
}
