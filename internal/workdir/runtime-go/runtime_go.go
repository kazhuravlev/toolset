package runtimego

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/kazhuravlev/optional"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

const At = "@"

func getGoModuleName(link string) (string, error) {
	link = strings.Split(link, At)[0]

	for {
		// TODO: use a local proxy if configured.
		resp, err := http.Get(fmt.Sprintf("https://proxy.golang.org/%s/@latest", link))
		if err != nil {
			return "", fmt.Errorf("do request to golang proxy: %w", err)
		}
		resp.Body.Close()

		if resp.StatusCode == http.StatusOK {
			return link, nil
		}

		if resp.StatusCode == http.StatusNotFound {
			parts := strings.Split(link, "/")
			if len(parts) == 1 {
				break
			}

			link = strings.Join(parts[:len(parts)-1], "/")
		}
	}

	return "", errors.New("unknown module")
}

func GetGoModule(ctx context.Context, link string) (string, *GoModule, error) {
	// FIXME: duplicated http request
	module, err := getGoModuleName(link)
	if err != nil {
		return "", nil, fmt.Errorf("get go module name: %w", err)
	}

	// TODO: use a proxy from env
	// Get the latest version
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, fmt.Sprintf("https://proxy.golang.org/%s/@latest", module), nil)
	if err != nil {
		return "", nil, fmt.Errorf("create request: %w", err)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", nil, fmt.Errorf("get go module: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", nil, fmt.Errorf("unable to get module: %s", resp.Status)
	}

	var mod GoModule
	if err := json.NewDecoder(resp.Body).Decode(&mod); err != nil {
		return "", nil, fmt.Errorf("unable to decode module: %w", err)
	}

	return module, &mod, nil
}

func GetGoInstalledBinary(baseDir, goBinDir, mod string) string {
	modDir := filepath.Join(baseDir, goBinDir, GetGoModDir(mod))
	return filepath.Join(modDir, GetGoBinFromMod(mod))
}

func GoInstall(baseDir, mod, goBinDir string, alias optional.Val[string]) error {
	const golang = "go"

	installedPath := GetGoInstalledBinary(baseDir, goBinDir, mod)

	modDir := filepath.Join(baseDir, goBinDir, GetGoModDir(mod))
	if err := os.MkdirAll(modDir, 0o755); err != nil {
		return fmt.Errorf("create mod dir (%s): %w", modDir, err)
	}

	cmd := &exec.Cmd{
		Path: golang,
		Args: []string{golang, "install", mod},
		Env: append(os.Environ(),
			"GOBIN="+modDir,
		),
	}

	lp, _ := exec.LookPath(golang)
	if lp != "" {
		// Update cmd.Path even if err is non-nil.
		// If err is ErrDot (especially on Windows), lp may include a resolved
		// extension (like .exe or .bat) that should be preserved.
		cmd.Path = lp
	}

	var stdout bytes.Buffer
	cmd.Stdout = &stdout

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("run go install (%s): %w", cmd.String(), err)
	}

	if alias, ok := alias.Get(); ok {
		targetPath := filepath.Join(baseDir, goBinDir, alias)
		if _, err := os.Stat(targetPath); err == nil {
			if err := os.Remove(targetPath); err != nil {
				return fmt.Errorf("remove alias (%s): %w", targetPath, err)
			}
		}

		if err := os.Symlink(installedPath, targetPath); err != nil {
			return fmt.Errorf("symlink %s to %s: %w", installedPath, targetPath, err)
		}
	}

	return nil
}

// GetGoBinFromMod returns a binary name that installed by `go install`
// github.com/golangci/golangci-lint/cmd/golangci-lint@v1.55.2 ==> golangci-lint
func GetGoBinFromMod(mod string) string {
	// github.com/user/repo@v1.0.0 => github.com/user/repo
	if strings.Contains(mod, At) {
		mod = strings.Split(mod, At)[0]
	}

	// github.com/user/repo/cmd/some/program => program
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

// GetGoModDir returns a dir that will keep all mod-related stuff for specific version.
func GetGoModDir(mod string) string {
	binName := GetGoBinFromMod(mod)
	parts := strings.Split(mod, At)
	version := parts[1]

	return fmt.Sprintf(".%s___%s", binName, version)
}

type GoModule struct {
	Version string    `json:"Version"`
	Time    time.Time `json:"Time"`
	Origin  struct {
		VCS  string `json:"VCS"`
		URL  string `json:"URL"`
		Hash string `json:"Hash"`
		Ref  string `json:"Ref"`
	} `json:"Origin"`
}
