package workdir

import (
	"context"
	"fmt"
	runtimego "github.com/kazhuravlev/toolset/internal/workdir/runtime-go"
	"github.com/kazhuravlev/toolset/internal/workdir/structs"
	"path/filepath"
)

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
	impls map[string]IRuntime
}

func (r *Runtimes) Get(runtime string) (IRuntime, error) {
	rt, ok := r.impls[runtime]
	if !ok {
		return nil, fmt.Errorf("unsupported runtime: %s", runtime)
	}

	return rt, nil
}

func NewRuntimes(ctx context.Context, baseDir, specDir string) (*Runtimes, error) {
	binToolDir := filepath.Join(baseDir, specDir)

	goRuntimes, err := runtimego.Discover(ctx, binToolDir)
	if err != nil {
		return nil, fmt.Errorf("discovering go runtimes: %w", err)
	}

	impls := make(map[string]IRuntime, len(goRuntimes))
	for _, rt := range goRuntimes {
		impls["go@"+rt.Version()] = rt
	}

	return &Runtimes{
		impls: impls,
	}, nil
}
