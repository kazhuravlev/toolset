package runtimego

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/kazhuravlev/toolset/internal/prog"
	"github.com/spf13/afero"
	"golang.org/x/mod/modfile"
	"golang.org/x/mod/module"
)

var reVersion = regexp.MustCompile(`^go version go(\d+\.\d+(?:\.\d+)?)(?: .*|$)`)

type moduleInfo struct {
	Mod prog.Version

	Program   string // golangci-lint
	IsPrivate bool   // depends on `go env GOPRIVATE`
}

// parse will parse source string and try to extract all details about mentioned golang program.
func (r *Runtime) parse(ctx context.Context, str string) (*moduleInfo, error) {
	var mod, version, program string

	{
		parts := strings.Split(str, at)
		switch len(parts) {
		default:
			return nil, errors.New("invalid format")
		case 1: // have no version. means latest
			version = "latest"
		case 2: // have version. parse it
			version = parts[1]
		}

		mod = parts[0]

		// github.com/user/repo/cmd/program => program
		if strings.Contains(mod, "/cmd/") {
			program = filepath.Base(strings.Split(mod, "/cmd/")[1])
		} else {
			// github.com/user/repo/v3 => repo
			parts := strings.Split(mod, "/")
			lastPart := parts[len(parts)-1]
			if strings.HasPrefix(lastPart, "v") {
				program = parts[len(parts)-2]
			} else {
				program = lastPart
			}
		}
	}

	buf := bytes.NewBuffer(nil)
	{
		cmd := exec.CommandContext(ctx, r.goBin, "env", "GOPRIVATE")
		cmd.Env = r.goEnv()
		cmd.Stdout = buf
		cmd.Stderr = io.Discard
		if err := cmd.Run(); err != nil {
			return nil, fmt.Errorf("go env GOPRIVATE: %w", err)
		}
	}

	goPrivate := strings.TrimSpace(buf.String()) // trim new line ending

	var modVer prog.Version
	if version == "latest" {
		modVer = prog.NewLatest(mod)
	} else {
		modVer = prog.NewVer(mod, version)
	}

	return &moduleInfo{
		Mod:       modVer,
		Program:   program,
		IsPrivate: module.MatchPrefixPatterns(goPrivate, mod),
	}, nil
}

type fetchedMod struct {
	Version string `json:"Version"`
}

// fetchModule will fetch the module for required version. Always returns a specific version
// @ => @latest
// @latest => @vX.X.X
// @vX.X.X => @vX.X.X
func (r *Runtime) fetchModule(ctx context.Context, link string) (*moduleInfo, error) {
	mod, err := r.parse(ctx, link)
	if err != nil {
		return nil, fmt.Errorf("parse module (%s) string: %w", link, err)
	}

	if mod.IsPrivate {
		privateMod, err := r.fetchPrivate(ctx, *mod)
		if err != nil {
			return nil, fmt.Errorf("fetch private module: %w", err)
		}

		return r.parse(ctx, mod.Mod.Name()+at+privateMod.Mod.Version())
	}

	link = mod.Mod.Name()
	for {
		// TODO: use a local proxy if configured.
		// Get the latest version
		var modUrl string
		if mod.Mod.IsLatest() {
			modUrl = fmt.Sprintf("https://proxy.golang.org/%s/@latest", link)
		} else {
			modUrl = fmt.Sprintf("https://proxy.golang.org/%s/@v/%s.info", link, mod.Mod.Version())
		}

		req, err := http.NewRequestWithContext(ctx, http.MethodGet, modUrl, nil)
		if err != nil {
			return nil, fmt.Errorf("create request: %w", err)
		}

		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			return nil, fmt.Errorf("get go module: %w", err)
		}
		defer resp.Body.Close() //nolint:errcheck

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

		var fMod fetchedMod
		if err := json.NewDecoder(resp.Body).Decode(&fMod); err != nil {
			return nil, fmt.Errorf("unable to decode module: %w", err)
		}

		mod2, err := r.parse(ctx, mod.Mod.Name()+at+fMod.Version)
		if err != nil {
			return nil, fmt.Errorf("parse fetched module: %w", err)
		}

		return mod2, nil
	}

	return nil, errors.New("unknown module")
}

// fetchPrivate is a hack around golang tooling. This function do next steps:
// - Creates a temp dir
// - Init module in this dir
// - Add dependency
// - Get dep info
// - Remove temp dir
func (r *Runtime) fetchPrivate(ctx context.Context, mod moduleInfo) (*moduleInfo, error) {
	tmpDir, err := afero.TempDir(r.fs, "", "gomodtemp")
	if err != nil {
		return nil, fmt.Errorf("failed to create temp directory: %v", err)
	}
	defer r.fs.RemoveAll(tmpDir) //nolint:errcheck

	{
		cmd := exec.CommandContext(ctx, r.goBin, "mod", "init", "sample")
		cmd.Env = r.goEnv()
		cmd.Dir = tmpDir
		cmd.Stdout = io.Discard
		cmd.Stderr = io.Discard
		if err := cmd.Run(); err != nil {
			return nil, fmt.Errorf("go mod init: %w", err)
		}
	}

	{
		cmd := exec.CommandContext(ctx, r.goBin, "get", mod.Mod.S())
		cmd.Env = r.goEnv()
		cmd.Dir = tmpDir
		cmd.Stdout = io.Discard
		cmd.Stderr = io.Discard
		if err := cmd.Run(); err != nil {
			return nil, fmt.Errorf("go get: %w", err)
		}
	}

	goModFilename := filepath.Join(tmpDir, "go.mod")
	bb, err := afero.ReadFile(r.fs, goModFilename)
	if err != nil {
		return nil, fmt.Errorf("failed to read go.mod: %w", err)
	}

	modFile, err := modfile.Parse(goModFilename, bb, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to parse go.mod: %w", err)
	}

	for _, require := range modFile.Require {
		if strings.HasPrefix(mod.Mod.Name(), require.Mod.Path) {
			return r.parse(ctx, mod.Mod.Name()+at+require.Mod.Version)
		}
	}

	return nil, errors.New("sky was falling")
}

func getGoVersion(ctx context.Context, bin string) (string, error) {
	cmd := exec.CommandContext(ctx, bin, "version")
	cmd.Env = envAllOverride([][2]string{{"GOTOOLCHAIN", "local"}})

	var stdout bytes.Buffer
	cmd.Stdout = &stdout

	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("go version (%s): %w", cmd.String(), err)
	}

	matches := reVersion.FindStringSubmatch(stdout.String())

	if len(matches) > 1 {
		// matches[1] is the captured version part: "1.23.4"
		return matches[1], nil
	}

	return "", errors.New("could not determine go version")
}

// envAllOverride will set some envs and return all envs together. Go-specific.
// This is also will cleanup all envs from host installation except the goprivate and some other important vars.
//
// Example variables that not excluded
// GOEXPERIMENT='rangefunc'
// GONOPROXY='github.com/example/provate'
// GONOSUMDB='github.com/example/provate'
// GOPRIVATE='github.com/example/provate'
// GOPROXY='https://proxy.golang.org,direct'
// GOSUMDB='sum.golang.org'
// GOTELEMETRY='local'
func envAllOverride(envs [][2]string) []string {
	excluded := []string{
		"AR",
		"CC",
		"CGO_CFLAGS",
		"CGO_CPPFLAGS",
		"CGO_CXXFLAGS",
		"CGO_ENABLED",
		"CGO_FFLAGS",
		"CGO_LDFLAGS",
		"CXX",
		"GCCGO",
		"GO111MODULE",
		"GOARCH",
		"GOARM64",
		"GOAUTH",
		"GOBIN",
		"GODEBUG",
		"GOENV",
		"GOEXE",
		"GOFIPS140",
		"GOFLAGS",
		"GOGCCFLAGS",
		"GOHOSTARCH",
		"GOHOSTOS",
		"GOINSECURE",
		"GOMOD",
		"GOMODCACHE",
		"GOOS",
		"GOPATH",
		"GOROOT",
		"GOTELEMETRYDIR",
		"GOTMPDIR",
		"GOTOOLCHAIN",
		"GOTOOLDIR",
		"GOVCS",
		"GOVERSION",
		"GOWORK",
		"PKG_CONFIG",

		// "GOCACHE",
		// "GOCACHEPROG",
		// "GOEXPERIMENT",
		// "GONOPROXY",
		// "GONOSUMDB",
		// "GOPRIVATE",
		// "GOPROXY",
		// "GOSUMDB",
		// "GOTELEMETRY",
	}

	for _, env := range excluded {
		os.Unsetenv(env) //nolint:errcheck
	}

	for _, pair := range envs {
		key := pair[0]
		val := pair[1]
		os.Setenv(key, val) //nolint:errcheck
	}

	return os.Environ()
}
