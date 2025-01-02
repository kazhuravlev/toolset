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

	"github.com/kazhuravlev/toolset/internal/fsh"
	"github.com/spf13/afero"

	"github.com/kazhuravlev/toolset/internal/prog"

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
func parse(ctx context.Context, goBin, str string) (*moduleInfo, error) {
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
		cmd := exec.CommandContext(ctx, goBin, "env", "GOPRIVATE")
		cmd.Env = append(os.Environ(), "GOTOOLCHAIN=local")
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
func fetchModule(ctx context.Context, fs fsh.FS, goBin, link string) (*moduleInfo, error) {
	mod, err := parse(ctx, goBin, link)
	if err != nil {
		return nil, fmt.Errorf("parse module (%s) string: %w", link, err)
	}

	if mod.IsPrivate {
		privateMod, err := fetchPrivate(ctx, fs, goBin, *mod)
		if err != nil {
			return nil, fmt.Errorf("fetch private module: %w", err)
		}

		return parse(ctx, goBin, mod.Mod.Name()+at+privateMod.Mod.Version())
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

		var fMod fetchedMod
		if err := json.NewDecoder(resp.Body).Decode(&fMod); err != nil {
			return nil, fmt.Errorf("unable to decode module: %w", err)
		}

		mod2, err := parse(ctx, goBin, mod.Mod.Name()+at+fMod.Version)
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
func fetchPrivate(ctx context.Context, fSys fsh.FS, goBin string, mod moduleInfo) (*moduleInfo, error) {
	tmpDir, err := afero.TempDir(fSys, "", "gomodtemp")
	if err != nil {
		return nil, fmt.Errorf("failed to create temp directory: %v", err)
	}
	defer fSys.RemoveAll(tmpDir) //nolint:errcheck

	{
		cmd := exec.CommandContext(ctx, goBin, "mod", "init", "sample")
		cmd.Env = append(os.Environ(), "GOTOOLCHAIN=local")
		cmd.Dir = tmpDir
		cmd.Stdout = io.Discard
		cmd.Stderr = io.Discard
		if err := cmd.Run(); err != nil {
			return nil, fmt.Errorf("go mod init: %w", err)
		}
	}

	{
		cmd := exec.CommandContext(ctx, goBin, "get", mod.Mod.S())
		cmd.Env = append(os.Environ(), "GOTOOLCHAIN=local")
		cmd.Dir = tmpDir
		cmd.Stdout = io.Discard
		cmd.Stderr = io.Discard
		if err := cmd.Run(); err != nil {
			return nil, fmt.Errorf("go get: %w", err)
		}
	}

	goModFilename := filepath.Join(tmpDir, "go.mod")
	bb, err := afero.ReadFile(fSys, goModFilename)
	if err != nil {
		return nil, fmt.Errorf("failed to read go.mod: %w", err)
	}

	modFile, err := modfile.Parse(goModFilename, bb, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to parse go.mod: %w", err)
	}

	for _, require := range modFile.Require {
		if strings.HasPrefix(mod.Mod.Name(), require.Mod.Path) {
			return parse(ctx, goBin, mod.Mod.Name()+at+require.Mod.Version)
		}
	}

	return nil, errors.New("sky was falling")
}

func getGoVersion(ctx context.Context, bin string) (string, error) {
	cmd := exec.CommandContext(ctx, bin, "version")
	cmd.Env = append(os.Environ(), "GOTOOLCHAIN=local")

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
