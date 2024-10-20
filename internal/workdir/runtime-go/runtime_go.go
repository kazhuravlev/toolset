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

	goModule, err := getGoModule(ctx, str)
	if err != nil {
		return "", fmt.Errorf("get go module version: %w", err)
	}

	goBinaryWoVersion := strings.Split(str, at)[0]
	if strings.Contains(str, "@latest") || !strings.Contains(str, at) {
		str = fmt.Sprintf("%s%s%s", goBinaryWoVersion, at, goModule.Version)
	}

	return str, nil
}

func (r *Runtime) GetModule(ctx context.Context, module string) (*structs.ModuleInfo, error) {
	return &structs.ModuleInfo{
		Name:        r.getProgramName(module),
		BinaryPath:  r.getBinaryPath(module),
		IsInstalled: isExists(r.getBinaryPath(module)),
	}, nil
}

func (r *Runtime) Install(ctx context.Context, program string) error {
	const golang = "go"

	goBinDir := filepath.Join(r.baseDir, getGoModDir(program))
	if err := os.MkdirAll(goBinDir, 0o755); err != nil {
		return fmt.Errorf("create mod dir (%s): %w", goBinDir, err)
	}

	cmd := exec.CommandContext(ctx, golang, "install", program)
	cmd.Env = append(os.Environ(), "GOBIN="+goBinDir)

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
	programBinary := r.getBinaryPath(program)
	cmd := exec.CommandContext(ctx, programBinary, args...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("run (%s): %w", program, err)
	}

	return nil
}

func (r *Runtime) GetLatest(ctx context.Context, module string) (string, bool, error) {
	goModule, err := getGoModule(ctx, module)
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

func (r *Runtime) getProgramName(program string) string {
	return getProgramName(program)
}

func (r *Runtime) getBinaryPath(program string) string {
	programDir := filepath.Join(r.baseDir, getGoModDir(program))

	return filepath.Join(programDir, r.getProgramName(program))
}
