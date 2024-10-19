package workdir

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/kazhuravlev/optional"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
)

func isExists(path string) bool {
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return false
	}

	return true
}

func readSpec(path string) (*Spec, error) {
	bb, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read spec file (%s): %w", path, err)
	}

	var spec Spec
	if err := json.Unmarshal(bb, &spec); err != nil {
		return nil, fmt.Errorf("unmarshal spec (%s): %w", path, err)
	}

	return &spec, nil
}

func writeSpec(path string, spec Spec) error {
	file, err := os.OpenFile(path, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o644)
	if err != nil {
		return fmt.Errorf("open spec: %w", err)
	}

	enc := json.NewEncoder(file)
	enc.SetIndent("", "\t")

	if err := enc.Encode(spec); err != nil {
		return fmt.Errorf("marshal spec: %w", err)
	}

	return nil
}

func writeLock(path string, lock Lock) error {
	file, err := os.OpenFile(path, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o644)
	if err != nil {
		return fmt.Errorf("open lock: %w", err)
	}

	enc := json.NewEncoder(file)
	enc.SetIndent("", "\t")

	if err := enc.Encode(lock); err != nil {
		return fmt.Errorf("marshal lock: %w", err)
	}

	return nil
}

func getGoModuleName(link string) (string, error) {
	link = strings.Split(link, "@")[0]

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

func getGoModule(ctx context.Context, link string) (string, *GoModule, error) {
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

func getGoInstalledBinary(baseDir, goBinDir, mod string) string {
	modDir := filepath.Join(baseDir, goBinDir, getGoModDir(mod))
	return filepath.Join(modDir, getGoBinFromMod(mod))
}

func goInstall(baseDir, mod, goBinDir string, alias optional.Val[string]) error {
	const golang = "go"

	installedPath := getGoInstalledBinary(baseDir, goBinDir, mod)

	modDir := filepath.Join(baseDir, goBinDir, getGoModDir(mod))
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

const at = "@"

// getGoBinFromMod returns a binary name that installed by `go install`
// github.com/golangci/golangci-lint/cmd/golangci-lint@v1.55.2 ==> golangci-lint
func getGoBinFromMod(mod string) string {
	// github.com/user/repo@v1.0.0 => github.com/user/repo
	if strings.Contains(mod, at) {
		mod = strings.Split(mod, at)[0]
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

// getGoModDir returns a dir that will keep all mod-related stuff for specific version.
func getGoModDir(mod string) string {
	binName := getGoBinFromMod(mod)
	parts := strings.Split(mod, at)
	version := parts[1]

	return fmt.Sprintf(".%s___%s", binName, version)
}

func parseSourceURI(uri string) (SourceUri, error) {
	sourceURL, err := url.Parse(uri)
	if err != nil {
		return nil, fmt.Errorf("parse source uri: %w", err)
	}

	switch sourceURL.Scheme {
	default:
		return nil, fmt.Errorf("unsupported source uri scheme (%s)", sourceURL.Scheme)
	case "":
		uri, err := filepath.Abs(uri)
		if err != nil {
			return nil, fmt.Errorf("resolve absolute path: %w", err)
		}

		return SourceUriFile{Path: uri}, nil
	case "http", "https":
		return SourceUriUrl{URL: uri}, nil
	case "git+ssh":
		parts := strings.Split(uri, ":")
		pathToFile := parts[len(parts)-1]

		return SourceUriGit{
			Addr: strings.TrimSuffix(strings.TrimPrefix(uri, "git+ssh://"), ":"+pathToFile),
			Path: pathToFile,
		}, nil
	case "git+https":
		parts := strings.Split(uri, ":")
		pathToFile := parts[len(parts)-1]

		return SourceUriGit{
			Addr: strings.TrimSuffix(strings.TrimPrefix(uri, "git+"), ":"+pathToFile),
			Path: pathToFile,
		}, nil
	}
}

func fetchRemoteSpec(ctx context.Context, source string, tags []string) ([]RemoteSpec, error) {
	srcURI, err := parseSourceURI(source)
	if err != nil {
		return nil, fmt.Errorf("parse source uri: %w", err)
	}

	var buf []byte
	switch srcURI := srcURI.(type) {
	default:
		return nil, errors.New("unsupported source uri")
	case SourceUriUrl:
		fmt.Println("Include from url:", srcURI.URL)

		req, err := http.NewRequestWithContext(ctx, http.MethodGet, srcURI.URL, nil)
		if err != nil {
			return nil, fmt.Errorf("create request: %w", err)
		}

		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			return nil, fmt.Errorf("fetch source: %w", err)
		}
		defer resp.Body.Close()

		bb, err := io.ReadAll(resp.Body)
		if err != nil {
			return nil, fmt.Errorf("read response body: %w", err)
		}

		buf = bb
	case SourceUriFile:
		fmt.Println("Include from file:", srcURI.Path)

		bb, err := os.ReadFile(srcURI.Path)
		if err != nil {
			return nil, fmt.Errorf("read file: %w", err)
		}

		buf = bb
	case SourceUriGit:
		fmt.Println("Include from git:", srcURI.Addr, "file:", srcURI.Path)

		targetDir, err := os.MkdirTemp(os.TempDir(), "toolset")
		if err != nil {
			return nil, fmt.Errorf("create temp dir: %w", err)
		}

		args := []string{
			"clone",
			"--depth", "1",
			srcURI.Addr,
			targetDir,
		}

		cmd := exec.CommandContext(ctx, "git", args...)
		cmd.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0")
		cmd.Stdin = nil
		cmd.Stdout = io.Discard
		cmdErr := bytes.NewBufferString("")
		cmd.Stderr = cmdErr
		if err := cmd.Run(); err != nil {
			return nil, fmt.Errorf("clone git repo (%s): %w", strings.TrimSpace(cmdErr.String()), err)
		}

		targetFile := filepath.Join(targetDir, srcURI.Path)
		bb, err := os.ReadFile(targetFile)
		if err != nil {
			return nil, fmt.Errorf("read file: %w", err)
		}

		if err := os.RemoveAll(targetDir); err != nil {
			return nil, fmt.Errorf("remove temp dir: %w", err)
		}

		buf = bb
	}

	var spec Spec
	if err := json.Unmarshal(buf, &spec); err != nil {
		return nil, fmt.Errorf("parse source: %w", err)
	}

	var res []RemoteSpec
	for _, inc := range spec.Includes {
		// FIXME(zhuravlev): add cycle detection
		incSpecs, err := fetchRemoteSpec(ctx, inc.Src, append(slices.Clone(tags), inc.Tags...))
		if err != nil {
			return nil, fmt.Errorf("fetch one of remotes (%s): %w", inc, err)
		}

		res = append(res, incSpecs...)
	}

	return append(res, RemoteSpec{
		Spec:   spec,
		Source: source,
		Tags:   tags,
	}), nil
}
