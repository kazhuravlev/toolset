package runtimego

import (
	"bytes"
	"context"
	"errors"
	"fmt"
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

func (r *Runtime) GetLatest(ctx context.Context, module string) (string, bool, error) {
	_, goModule, err := getGoModule(ctx, module)
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

func (r *Runtime) GetProgramName(program string) string {
	return getProgramName(program)
}

// Parse will parse string to normal version.
// github.com/kazhuravlev/toolset/cmd/toolset@latest
// github.com/kazhuravlev/toolset/cmd/toolset
// github.com/kazhuravlev/toolset/cmd/toolset@v4.2
func (r *Runtime) Parse(ctx context.Context, program string) (string, error) {
	if program == "" {
		return "", errors.New("program name not provided")
	}

	_, goModule, err := getGoModule(ctx, program)
	if err != nil {
		return "", fmt.Errorf("get go module version: %w", err)
	}

	goBinaryWoVersion := strings.Split(program, at)[0]
	if strings.Contains(program, "@latest") || !strings.Contains(program, at) {
		program = fmt.Sprintf("%s%s%s", goBinaryWoVersion, at, goModule.Version)
	}

	return program, nil
}

func (r *Runtime) GetProgramDir(program string) string {
	return filepath.Join(r.baseDir, getGoModDir(program))
}

func (r *Runtime) IsInstalled(program string) bool {
	programDir := filepath.Join(r.baseDir, r.GetProgramDir(program))

	return isExists(programDir)
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

func (r *Runtime) GetBinaryPath(program string) string {
	return filepath.Join(r.GetProgramDir(program), r.GetProgramName(program))
}

func (r *Runtime) Run(ctx context.Context, program string, args ...string) error {
	programBinary := r.GetBinaryPath(program)
	cmd := exec.CommandContext(ctx, programBinary, args...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("run (%s): %w", program, err)
	}

	return nil
}
