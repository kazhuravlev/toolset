package runtimego

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"github.com/kazhuravlev/toolset/internal/version"
	"github.com/kazhuravlev/toolset/internal/workdir/structs"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
)

const golang = "go"
const at = "@"

type Runtime struct {
	goBin      string // absolute path to golang binary
	goVersion  string // ex: 1.23
	binToolDir string
}

func New(binToolDir, goBin, goVer string) *Runtime {
	return &Runtime{
		goBin:      goBin,
		goVersion:  goVer,
		binToolDir: binToolDir,
	}
}

// Parse will parse string to normal version.
// github.com/kazhuravlev/toolset/cmd/toolset@latest
// github.com/kazhuravlev/toolset/cmd/toolset
// github.com/kazhuravlev/toolset/cmd/toolset@v4.2
func (r *Runtime) Parse(ctx context.Context, str string) (string, error) {
	if str == "" {
		return "", errors.New("program name not provided")
	}

	goModule, err := fetchLatest(ctx, str)
	if err != nil {
		return "", fmt.Errorf("get go module version: %w", err)
	}

	return goModule.Canonical, nil
}

func (r *Runtime) GetModule(ctx context.Context, module string) (*structs.ModuleInfo, error) {
	mod, err := parse(ctx, module)
	if err != nil {
		return nil, fmt.Errorf("parse module (%s): %w", module, err)
	}

	programDir := filepath.Join(r.binToolDir, fmt.Sprintf(".%s___%s", mod.Program, mod.Version))
	programBinary := filepath.Join(programDir, mod.Program)

	return &structs.ModuleInfo{
		Name:        mod.Program,
		Version:     mod.Version,
		BinDir:      programDir,
		BinPath:     programBinary,
		IsInstalled: isExists(programBinary),
		IsPrivate:   mod.IsPrivate,
	}, nil
}

func (r *Runtime) Install(ctx context.Context, program string) error {
	mod, err := r.GetModule(ctx, program)
	if err != nil {
		return fmt.Errorf("get go module (%s): %w", program, err)
	}

	if err := os.MkdirAll(mod.BinDir, 0o755); err != nil {
		return fmt.Errorf("create mod dir (%s): %w", mod.BinDir, err)
	}

	cmd := exec.CommandContext(ctx, golang, "install", program)
	cmd.Env = append(os.Environ(), "GOBIN="+mod.BinDir)

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

	return nil
}

func (r *Runtime) Run(ctx context.Context, program string, args ...string) error {
	mod, err := r.GetModule(ctx, program)
	if err != nil {
		return fmt.Errorf("get go module (%s): %w", program, err)
	}

	if !mod.IsInstalled {
		return fmt.Errorf("program (%s) is not installed: %w", program, structs.ErrToolNotInstalled)
	}

	programBinary := mod.BinPath
	cmd := exec.CommandContext(ctx, programBinary, args...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			return fmt.Errorf("exit not ok (%s): %w", program, errors.Join(&structs.RunError{ExitCode: exitErr.ExitCode()}, err))
		}

		return fmt.Errorf("run (%s): %w", program, err)
	}

	return nil
}

func (r *Runtime) GetLatest(ctx context.Context, module string) (string, bool, error) {
	latestMod, err := fetchLatest(ctx, module)
	if err != nil {
		return "", false, fmt.Errorf("get go module: %w", err)
	}

	if module == latestMod.Canonical {
		return module, false, nil
	}

	return latestMod.Canonical, true, nil
}

func (r *Runtime) Remove(ctx context.Context, tool structs.Tool) error {
	mod, err := r.GetModule(ctx, tool.Module)
	if err != nil {
		return fmt.Errorf("get go module (%s): %w", tool.Module, err)
	}

	if !mod.IsInstalled {
		return errors.New("module is not installed")
	}

	if err := os.RemoveAll(mod.BinDir); err != nil {
		return fmt.Errorf("remove (%s): %w", mod.BinDir, err)
	}

	return nil
}

func (r *Runtime) Version() string {
	return r.goVersion
}

const runtimePrefix = ".rtgo__"

// Discover will find all supported golang runtimes. It can be:
// - global installation
// - local ./bin/tools installation
func Discover(ctx context.Context, binToolDir string) ([]*Runtime, error) {
	var res []*Runtime

	// Discover global version
	{
		lp, err := exec.LookPath(golang)
		if err != nil {
			return res, fmt.Errorf("find golang: %w", err)
		}

		ver, err := getGoVersion(ctx, lp)
		if err != nil {
			return res, fmt.Errorf("get go version: %w", err)
		}

		res = append(res, New(binToolDir, lp, ver))
	}

	// Discover local installations
	if isExists(binToolDir) {
		entries, err := os.ReadDir(binToolDir)
		if err != nil {
			return nil, fmt.Errorf("list dir: %w", err)
		}

		for _, e := range entries {
			if !e.IsDir() {
				continue
			}

			if !strings.HasPrefix(e.Name(), runtimePrefix) {
				continue
			}

			ver := strings.TrimPrefix(e.Name(), runtimePrefix)

			goBin := filepath.Join(binToolDir, e.Name(), "go"+ver, "bin", "go")
			if !isExists(goBin) {
				_ = os.RemoveAll(filepath.Join(binToolDir, e.Name()))
				continue
			}

			goVer, err := getGoVersion(ctx, goBin)
			if err != nil {
				return res, fmt.Errorf("get go version for (%s): %w", goBin, err)
			}

			res = append(res, New(binToolDir, goBin, goVer))
		}
	}

	return res, nil
}

var reVersion = regexp.MustCompile(`^go version go(\d+\.\d+(?:\.\d+)?)(?: .*|$)`)

func getGoVersion(ctx context.Context, bin string) (string, error) {
	cmd := exec.CommandContext(ctx, bin, "version")

	var stdout bytes.Buffer
	cmd.Env = append(os.Environ(), "GOTOOLCHAIN=local")
	cmd.Stdout = &stdout

	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("go version (%s): %w", cmd.String(), err)
	}

	// FindStringSubmatch returns a slice of matched strings:
	// the first element is the entire match, and subsequent elements (if any)
	// are the capturing groups—in this case, our version number.
	matches := reVersion.FindStringSubmatch(stdout.String())

	if len(matches) > 1 {
		// matches[1] is the captured version part: "1.23.4"
		return matches[1], nil
	}

	return "", errors.New("could not determine go version")
}

func Install(ctx context.Context, binToolDir, ver string) error {
	dstDir := filepath.Join(binToolDir, runtimePrefix+ver)
	if err := version.Install(ctx, dstDir, "go"+ver); err != nil {
		_ = os.RemoveAll(dstDir)
		return fmt.Errorf("install go (%s): %w", ver, err)
	}

	return nil
}
