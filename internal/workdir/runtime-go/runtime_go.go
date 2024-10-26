package runtimego

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"github.com/kazhuravlev/toolset/internal/workdir/structs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

const at = "@"

type Runtime struct {
	baseDir string
}

func New(baseDir string) *Runtime {
	return &Runtime{baseDir: baseDir}
}

// Parse will parse string to normal version.
// github.com/kazhuravlev/toolset/cmd/toolset@latest
// github.com/kazhuravlev/toolset/cmd/toolset
// github.com/kazhuravlev/toolset/cmd/toolset@v4.2
func (r *Runtime) Parse(ctx context.Context, str string) (string, error) {
	if str == "" {
		return "", errors.New("program name not provided")
	}

	mod, err := parse(str)
	if err != nil {
		return "", fmt.Errorf("parse program: %w", err)
	}

	goModule, err := fetch(ctx, mod.Canonical)
	if err != nil {
		return "", fmt.Errorf("get go module version: %w", err)
	}

	if mod.Version == "latest" {
		return fmt.Sprintf("%s%s%s", mod.Module, at, goModule.Version), nil
	}

	return str, nil
}

func (r *Runtime) GetModule(_ context.Context, module string) (*structs.ModuleInfo, error) {
	mod, err := parse(module)
	if err != nil {
		return nil, fmt.Errorf("parse module (%s): %w", module, err)
	}

	programDir := filepath.Join(r.baseDir, fmt.Sprintf(".%s___%s", mod.Program, mod.Version))
	programBinary := filepath.Join(programDir, mod.Program)

	return &structs.ModuleInfo{
		Name:        mod.Program,
		Version:     mod.Version,
		BinDir:      programDir,
		BinPath:     programBinary,
		IsInstalled: isExists(programBinary),
	}, nil
}

func (r *Runtime) Install(ctx context.Context, program string) error {
	const golang = "go"

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
	goModule, err := fetch(ctx, module)
	if err != nil {
		return "", false, fmt.Errorf("get go module: %w", err)
	}

	goBinaryWoVersion := strings.Split(module, at)[0]
	latestModule := fmt.Sprintf("%s%s%s", goBinaryWoVersion, at, goModule.Version)

	if module == latestModule {
		return module, false, nil
	}

	return latestModule, true, nil
}
