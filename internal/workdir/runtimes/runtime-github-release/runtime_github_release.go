package runtimegh

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/google/go-github/v75/github"
	"github.com/kazhuravlev/toolset/internal/archive"
	"github.com/kazhuravlev/toolset/internal/fsh"
	"github.com/kazhuravlev/toolset/internal/workdir/structs"
	"golang.org/x/oauth2"
)

const (
	runtimeName = "gh"
	at          = "@"
)

type Runtime struct {
	fs         fsh.FS
	binToolDir string
	github     *github.Client
	os, arch   string
}

func New(fs fsh.FS, binToolDir string, ghClient *github.Client, goos, goarch string) *Runtime {
	return &Runtime{
		fs:         fs,
		binToolDir: binToolDir,
		github:     ghClient,
		os:         goos,
		arch:       goarch,
	}
}

// Parse will parse string to normal version.
// Supported strings:
//
//	golangci/golangci-lint@v2.5.0
func (r *Runtime) Parse(_ context.Context, str string) (string, error) {
	mod, err := parse(str)
	if err != nil {
		return "", fmt.Errorf("parse: %w", err)
	}

	return mod.Mod.S(), nil
}

func (r *Runtime) GetModule(ctx context.Context, module string) (*structs.ModuleInfo, error) {
	mod, err := parse(module)
	if err != nil {
		return nil, fmt.Errorf("parse module (%s): %w", module, err)
	}

	programDir := filepath.Join(r.binToolDir, fmt.Sprintf("gh/%s", mod.Mod.S()))
	programBinary := filepath.Join(programDir, mod.Program)

	return &structs.ModuleInfo{
		Name:        mod.Program,
		Mod:         mod.Mod,
		BinDir:      programDir,
		BinPath:     programBinary,
		IsInstalled: fsh.IsExists(r.fs, programBinary),
		// NOTE: this is not correct, because we can use GITHUB_TOKEN env to access to github api. Projects can be private.
		IsPrivate: false,
	}, nil
}

func (r *Runtime) Install(ctx context.Context, program string) error {
	mod, err := r.GetModule(ctx, program)
	if err != nil {
		return fmt.Errorf("get go module (%s): %w", program, err)
	}

	tmpDirBase := filepath.Join(r.binToolDir, runtimeName, "tmp")
	if err := r.fs.MkdirAll(tmpDirBase, 0o755); err != nil {
		return fmt.Errorf("create tmp dir base (%s): %w", tmpDirBase, err)
	}

	if err := r.fs.MkdirAll(mod.BinDir, 0o755); err != nil {
		return fmt.Errorf("create mod dir (%s): %w", mod.BinDir, err)
	}

	tmpDir, err := os.MkdirTemp(tmpDirBase, "toolset-gh-release")
	if err != nil {
		return fmt.Errorf("create tmp dir: %w", err)
	}

	tmpDirUnarchived, err := os.MkdirTemp(tmpDirBase, "toolset-gh-release-unarchived")
	if err != nil {
		return fmt.Errorf("create tmp dir: %w", err)
	}

	owner, repo, ok := strings.Cut(mod.Mod.Name(), "/")
	if !ok {
		return fmt.Errorf("unexpected module name (%s)", mod.Mod.Name())
	}

	asset, err := r.getAsset(ctx, owner, repo, mod.Mod.Version())
	if err != nil {
		return fmt.Errorf("get gh asset: %w", err)
	}

	tmpFile := filepath.Join(tmpDir, "download"+fsh.Ext(asset.GetName()))

	if err := r.downloadAsset(ctx, owner, repo, *asset.ID, tmpFile); err != nil {
		return fmt.Errorf("download asset: %w", err)
	}

	if err := archive.Extract(r.fs, tmpFile, tmpDirUnarchived); err != nil {
		return fmt.Errorf("extract release file: %w", err)
	}

	// Find the binary in the extracted archive
	binFile, err := r.findBinary(tmpDirUnarchived, repo)
	if err != nil {
		return fmt.Errorf("find binary in extracted archive: %w", err)
	}

	if err := r.fs.Rename(binFile, mod.BinPath); err != nil {
		return fmt.Errorf("move binary to target location (%s): %w", binFile, err)
	}

	if err := fsh.SetExecutable(r.fs, mod.BinPath); err != nil {
		return fmt.Errorf("set executable (%s): %w", binFile, err)
	}

	if err := r.fs.RemoveAll(tmpDir); err != nil {
		return fmt.Errorf("remove tmp dir: %w", err)
	}

	if err := r.fs.RemoveAll(tmpDirUnarchived); err != nil {
		return fmt.Errorf("remove tmp dir: %w", err)
	}

	return nil
}

// findBinary searches for the binary in the extracted archive.
// It handles multiple cases:
// 1. Binary is directly in the archive root
// 2. Binary is in a subdirectory (e.g., toolname-v1.0.0/toolname)
// 3. Binary is in common directories like bin/, cmd/
// 4. Binary might have platform-specific suffixes (e.g., toolname_darwin_amd64)
func (r *Runtime) findBinary(extractedDir, binaryName string) (string, error) {
	// List of paths to check, in order of preference
	pathsToCheck := []string{
		// 1. Direct in root
		filepath.Join(extractedDir, binaryName),
	}

	// 2. Check if there's a subdirectory (common for releases)
	dirName, err := fsh.FirstDir(r.fs, extractedDir)
	if err == nil {
		pathsToCheck = append(pathsToCheck,
			// In first subdirectory
			filepath.Join(extractedDir, dirName, binaryName),
			// In first subdirectory's bin/ folder
			filepath.Join(extractedDir, dirName, "bin", binaryName),
			// In first subdirectory's cmd/ folder
			filepath.Join(extractedDir, dirName, "cmd", binaryName),
		)
	}

	// 3. Also check common directories at root level
	pathsToCheck = append(pathsToCheck,
		filepath.Join(extractedDir, "bin", binaryName),
		filepath.Join(extractedDir, "cmd", binaryName),
	)

	// Check all paths
	for _, path := range pathsToCheck {
		if info, err := r.fs.Stat(path); err == nil && !info.IsDir() {
			return path, nil
		}
	}

	// 4. If not found by name, search recursively for any file matching the binary name
	var found string
	walkErr := r.fs.Walk(extractedDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil // Skip errors, continue walking
		}
		if info.IsDir() {
			return nil
		}
		if info.Name() == binaryName {
			found = path
			return filepath.SkipDir // Stop walking once found
		}
		return nil
	})

	if walkErr == nil && found != "" {
		return found, nil
	}

	return "", fmt.Errorf("could not find binary %q in extracted archive", binaryName)
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
	mod, err := parse(moduleReq)
	if err != nil {
		return "", false, fmt.Errorf("parse module (%s): %w", moduleReq, err)
	}

	owner, repo, ok := strings.Cut(mod.Mod.Name(), "/")
	if !ok {
		return "", false, fmt.Errorf("unexpected module name (%s)", mod.Mod.Name())
	}

	// Get the latest release from GitHub
	latestRelease, _, err := r.github.Repositories.GetLatestRelease(ctx, owner, repo)
	if err != nil {
		return "", false, fmt.Errorf("get latest release: %w", err)
	}

	latestTag := latestRelease.GetTagName()
	if latestTag == "" {
		return "", false, fmt.Errorf("latest release has no tag")
	}

	// Compare current version with latest version
	currentVersion := mod.Mod.Version()
	if currentVersion == latestTag {
		return moduleReq, false, nil
	}

	// Build the new module string with latest version
	latestModule := mod.Mod.Name() + "@" + latestTag

	return latestModule, true, nil
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
	return runtimeName
}

// Discover will return runtimes
func Discover(ctx context.Context, fSys fsh.FS, binToolDir string) ([]*Runtime, error) {
	httpClient := &http.Client{}

	// Auto-auth with github_token
	{
		if token := os.Getenv("TOOLSET_GITHUB_TOKEN"); token != "" {
			src := oauth2.StaticTokenSource(&oauth2.Token{AccessToken: token})
			httpClient = oauth2.NewClient(ctx, src)
		} else if token := os.Getenv("GITHUB_TOKEN"); token != "" {
			src := oauth2.StaticTokenSource(&oauth2.Token{AccessToken: token})
			httpClient = oauth2.NewClient(ctx, src)
		}
	}

	ghClient := github.NewClient(httpClient)

	rt := New(fSys, binToolDir, ghClient, runtime.GOOS, runtime.GOARCH)

	return []*Runtime{rt}, nil
}
