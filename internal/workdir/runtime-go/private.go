package runtimego

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

func getGoModuleName(ctx context.Context, link string) (*goModule, error) {
	link = strings.Split(link, at)[0]

	for {
		// TODO: use a local proxy if configured.
		// Get the latest version
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, fmt.Sprintf("https://proxy.golang.org/%s/@latest", link), nil)
		if err != nil {
			return nil, fmt.Errorf("create request: %w", err)
		}

		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			return nil, fmt.Errorf("get go module: %w", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			if resp.StatusCode == http.StatusNotFound {
				parts := strings.Split(link, "/")
				if len(parts) == 1 {
					break
				}

				link = strings.Join(parts[:len(parts)-1], "/")
				continue
			}

			return nil, fmt.Errorf("unable to get module: %s", resp.Status)
		}

		var mod goModule
		if err := json.NewDecoder(resp.Body).Decode(&mod); err != nil {
			return nil, fmt.Errorf("unable to decode module: %w", err)
		}

		return &mod, nil
	}

	return nil, errors.New("unknown module")
}

// getProgramName returns a binary name that installed by `go install`
// github.com/golangci/golangci-lint/cmd/golangci-lint@v1.55.2 ==> golangci-lint
func getProgramName(mod string) string {
	// github.com/user/repo@v1.0.0 => github.com/user/repo
	if strings.Contains(mod, at) {
		mod = strings.Split(mod, at)[0]
	}

	// github.com/user/repo/cmd/program => program
	if strings.Contains(mod, "/cmd/") {
		mod = strings.Split(mod, "/cmd/")[1]
		return filepath.Base(mod)
	}

	parts := strings.Split(mod, "/")
	// github.com/user/repo/v3 => repo
	if strings.HasPrefix(parts[len(parts)-1], "v") {
		prevPart := parts[len(parts)-2]
		return prevPart
	}

	return filepath.Base(mod)
}

// getGoModDir returns a dir that will keep all mod-related stuff for specific version.
func getGoModDir(mod string) string {
	binName := getProgramName(mod)
	parts := strings.Split(mod, at)
	version := parts[1]

	return fmt.Sprintf(".%s___%s", binName, version)
}

type goModule struct {
	Version string    `json:"Version"`
	Time    time.Time `json:"Time"`
	Origin  struct {
		VCS  string `json:"VCS"`
		URL  string `json:"URL"`
		Hash string `json:"Hash"`
		Ref  string `json:"Ref"`
	} `json:"Origin"`
}

func isExists(path string) bool {
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return false
	}

	return true
}
