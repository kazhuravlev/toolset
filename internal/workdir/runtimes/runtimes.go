package runtimes

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"

	runtimego "github.com/kazhuravlev/toolset/internal/workdir/runtimes/runtime-go"

	"github.com/kazhuravlev/toolset/internal/fsh"

	"github.com/kazhuravlev/toolset/internal/workdir/structs"
)

const runtimeGo = "go"

var ErrNotFound = errors.New("not found")

type IRuntime interface {
	// Parse will parse string with module name. It is used only on `toolset add` step.
	// Parse should:
	//	1) ensure that this program is valid, exists, and can be installed.
	//	2) normalize program name and return a canonical name.
	Parse(ctx context.Context, str string) (string, error)
	// GetModule returns an information about module (parsed module).
	GetModule(ctx context.Context, program string) (*structs.ModuleInfo, error)
	// Install will install the program.
	Install(ctx context.Context, program string) error
	Run(ctx context.Context, program string, args ...string) error
	GetLatest(ctx context.Context, module string) (string, bool, error)
	Remove(ctx context.Context, tool structs.Tool) error
}

type Runtimes struct {
	fs         fsh.FS
	binToolDir string
	impls      map[string]IRuntime
}

func New(fs fsh.FS, binToolDir string) (*Runtimes, error) {
	return &Runtimes{
		fs:         fs,
		binToolDir: binToolDir,
		impls:      make(map[string]IRuntime),
	}, nil
}

func (r *Runtimes) Get(runtime string) (IRuntime, error) {
	rt, ok := r.impls[runtime]
	if !ok {
		return nil, ErrNotFound
	}

	return rt, nil
}

// GetInstall will get installed runtime or try to install it in other case.
func (r *Runtimes) GetInstall(ctx context.Context, runtime string) (IRuntime, error) {
	if err := r.EnsureInstalled(ctx, runtime); err != nil {
		return nil, err
	}

	return r.Get(runtime)
}

func (r *Runtimes) EnsureInstalled(ctx context.Context, runtime string) error {
	if _, err := r.Get(runtime); err == nil {
		// Already installed
		return nil
	}

	if !strings.HasPrefix(runtime, runtimeGo+"@") {
		return fmt.Errorf("unsupported runtime: %s", runtime)
	}

	ver := strings.TrimPrefix(runtime, runtimeGo+"@")
	if err := runtimego.Install(ctx, r.fs, r.binToolDir, ver); err != nil {
		return fmt.Errorf("install tool runtime (%s): %w", runtime, err)
	}

	if err := r.Discover(ctx); err != nil {
		return fmt.Errorf("discover tools: %w", err)
	}

	return nil
}

func (r *Runtimes) List() []string {
	keys := make([]string, 0, len(r.impls))
	for k := range r.impls {
		keys = append(keys, k)
	}

	sort.Strings(keys)

	return keys
}

func (r *Runtimes) Discover(ctx context.Context) error {
	goRuntimes, err := runtimego.Discover(ctx, r.fs, r.binToolDir)
	if err != nil {
		return fmt.Errorf("discovering go runtimes: %w", err)
	}

	r.impls = make(map[string]IRuntime, len(goRuntimes))
	for _, rt := range goRuntimes {
		r.impls[rt.Version()] = rt
	}

	return nil
}
