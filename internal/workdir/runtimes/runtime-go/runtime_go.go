package runtimego

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"github.com/kazhuravlev/toolset/internal/envh"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/kazhuravlev/toolset/internal/fsh"
	"github.com/kazhuravlev/toolset/internal/version"
	"github.com/kazhuravlev/toolset/internal/workdir/structs"
	"github.com/spf13/afero"
)

const (
	runtimePrefix = "rtgo__"
	at            = "@"
)

type Runtime struct {
	fs         fsh.FS
	goBin      string // absolute path to golang binary
	isGlobal   bool
	goVersion  string // ex: 1.23
	binToolDir string
}

func New(fs fsh.FS, binToolDir, goBin, goVer string) *Runtime {
	return &Runtime{
		fs:         fs,
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

	goModule, err := fetchModule(ctx, r.fs, r.goBin, str)
	if err != nil {
		return "", fmt.Errorf("get go module version: %w", err)
	}

	return goModule.Mod.S(), nil
}

func (r *Runtime) GetModule(ctx context.Context, module string) (*structs.ModuleInfo, error) {
	mod, err := parse(ctx, r.goBin, module)
	if err != nil {
		return nil, fmt.Errorf("parse module (%s): %w", module, err)
	}

	programDir := filepath.Join(r.binToolDir, fmt.Sprintf("%s___%s", mod.Program, mod.Mod.Version()))
	programBinary := filepath.Join(programDir, mod.Program)

	return &structs.ModuleInfo{
		Name:        mod.Program,
		Mod:         mod.Mod,
		BinDir:      programDir,
		BinPath:     programBinary,
		IsInstalled: fsh.IsExists(r.fs, programBinary),
		IsPrivate:   mod.IsPrivate,
	}, nil
}

func (r *Runtime) Install(ctx context.Context, program string) error {
	mod, err := r.GetModule(ctx, program)
	if err != nil {
		return fmt.Errorf("get go module (%s): %w", program, err)
	}

	if err := r.fs.MkdirAll(mod.BinDir, 0o755); err != nil {
		return fmt.Errorf("create mod dir (%s): %w", mod.BinDir, err)
	}

	cmd := exec.CommandContext(ctx, r.goBin, "install", program)
	cmd.Env = envh.Unique([][2]string{{"GOTOOLCHAIN", "local"}, {"GOBIN", mod.BinDir}})

	var stdout bytes.Buffer
	cmd.Stderr = &stdout

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("run go install (%s): %w", strings.TrimSpace(stdout.String()), err)
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
			return fmt.Errorf("exit not ok (%s): %w", program, errors.Join(structs.RunError{ExitCode: exitErr.ExitCode()}, err))
		}

		return fmt.Errorf("run (%s): %w", program, err)
	}

	return nil
}

func (r *Runtime) GetLatest(ctx context.Context, moduleReq string) (string, bool, error) {
	mod, err := parse(ctx, r.goBin, moduleReq)
	if err != nil {
		return "", false, fmt.Errorf("parse module (%s): %w", moduleReq, err)
	}

	latestStr := mod.Mod.AsLatest().S()
	latestMod, err := fetchModule(ctx, r.fs, r.goBin, latestStr)
	if err != nil {
		return "", false, fmt.Errorf("get go module: %w", err)
	}

	if moduleReq == latestMod.Mod.S() {
		return moduleReq, false, nil
	}

	return latestMod.Mod.S(), true, nil
}

func (r *Runtime) Remove(ctx context.Context, tool structs.Tool) error {
	mod, err := r.GetModule(ctx, tool.Module)
	if err != nil {
		return fmt.Errorf("get go module (%s): %w", tool.Module, err)
	}

	if !mod.IsInstalled {
		return errors.New("module is not installed")
	}

	if err := r.fs.RemoveAll(mod.BinDir); err != nil {
		return fmt.Errorf("remove (%s): %w", mod.BinDir, err)
	}

	return nil
}

func (r *Runtime) Version() string {
	if r.isGlobal {
		return "go"
	}

	return "go@" + r.goVersion
}

// Discover will find all supported golang runtimes. It can be:
// - global installation
// - local ./bin/tools installation
func Discover(ctx context.Context, fSys fsh.FS, binToolDir string) ([]*Runtime, error) {
	const golang = "go"

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

		rt := New(fSys, binToolDir, lp, ver)
		rt.isGlobal = true
		res = append(res, rt)
	}

	// Discover local installations
	if fsh.IsExists(fSys, binToolDir) {
		entries, err := afero.ReadDir(fSys, binToolDir)
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
			if !fsh.IsExists(fSys, goBin) {
				_ = fSys.RemoveAll(filepath.Join(binToolDir, e.Name()))
				continue
			}

			goVer, err := getGoVersion(ctx, goBin)
			if err != nil {
				return res, fmt.Errorf("get go version for (%s): %w", goBin, err)
			}

			res = append(res, New(fSys, binToolDir, goBin, goVer))
		}
	}

	return res, nil
}

func Install(ctx context.Context, fSys fsh.FS, binToolDir, ver string) error {
	dstDir := filepath.Join(binToolDir, runtimePrefix+ver)
	if err := version.Install(ctx, dstDir, "go"+ver); err != nil {
		_ = fSys.RemoveAll(dstDir)
		return fmt.Errorf("install go (%s): %w", ver, err)
	}

	return nil
}
