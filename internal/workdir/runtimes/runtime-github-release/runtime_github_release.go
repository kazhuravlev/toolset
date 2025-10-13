package runtimegh

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
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
}

func New(fs fsh.FS, binToolDir string, ghClient *github.Client) *Runtime {
	return &Runtime{
		fs:         fs,
		binToolDir: binToolDir,
		github:     ghClient,
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

	// after unarchive archive - we get a directory inside. Not always, but often.
	dirName, err := fsh.FirstDir(r.fs, tmpDirUnarchived)
	if err != nil {
		return fmt.Errorf("unable to get release dir: %w", err)
	}

	binFile := filepath.Join(tmpDirUnarchived, dirName, repo)

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
	mod, err := r.GetModule(ctx, moduleReq)
	if err != nil {
		return "", false, fmt.Errorf("get module (%s): %w", moduleReq, err)
	}

	// TODO: implement upgrade through github client. Get release with `latest` mark.

	return mod.Mod.S(), false, nil
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

	rt := New(fSys, binToolDir, ghClient)

	return []*Runtime{rt}, nil
}
