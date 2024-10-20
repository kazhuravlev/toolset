package workdir

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/kazhuravlev/optional"
	runtimego "github.com/kazhuravlev/toolset/internal/workdir/runtime-go"
	"golang.org/x/sync/semaphore"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

const (
	specFilename    = ".toolset.json"
	lockFilename    = ".toolset.lock.json"
	defaultToolsDir = "./bin/tools"
)

type IRuntime interface {
	Parse(ctx context.Context, program string) (string, error)
	GetProgramDir(program string) string
	GetProgramName(program string) string
	GetBinaryPath(program string) string
	IsInstalled(program string) bool
	Install(ctx context.Context, program string) error
}

type Workdir struct {
	dir      string
	spec     *Spec
	lock     *Lock
	runtimes map[string]IRuntime
}

func New() (*Workdir, error) {
	// Make abs path to spec.
	toolsetFilename, err := filepath.Abs(specFilename)
	if err != nil {
		return nil, fmt.Errorf("get abs spec path: %w", err)
	}

	// Check that file is exists in current or parent directories.
	for {
		if !isExists(toolsetFilename) {
			parentDir := filepath.Dir(filepath.Dir(toolsetFilename))
			if filepath.Dir(parentDir) == parentDir {
				return nil, errors.New("unable to find spec in fs tree")
			}

			toolsetFilename = filepath.Join(parentDir, specFilename)
			continue
		}

		break
	}

	baseDir := filepath.Dir(toolsetFilename)

	spec, err := readJson[Spec](toolsetFilename)
	if err != nil {
		return nil, fmt.Errorf("spec file not found: %w", err)
	}

	if filepath.IsAbs(spec.Dir) {
		if !strings.HasPrefix(spec.Dir, baseDir) {
			return nil, fmt.Errorf("'Dir' should contains a relative path, not (%s)", spec.Dir)
		}

		spec.Dir = strings.TrimPrefix(spec.Dir, baseDir)
	}

	var lockFile Lock
	{
		bb, err := os.ReadFile(filepath.Join(baseDir, lockFilename))
		if err != nil {
			// NOTE(zhuravlev): Migration: add lockfile.
			{
				if os.IsNotExist(err) {
					fmt.Println("Migrate to `lock-based` version...")

					toolsetFilenameBak := toolsetFilename + "_bak"
					if err := os.Rename(toolsetFilename, toolsetFilenameBak); err != nil {
						return nil, fmt.Errorf("migrate toolset to lockfile: %w", err)
					}

					if _, err := Init(baseDir); err != nil {
						return nil, fmt.Errorf("re-init toolset: %w", err)
					}

					wCtx, err := New()
					if err != nil {
						return nil, fmt.Errorf("new context in re-created workdir: %w", err)
					}

					for _, tool := range spec.Tools {
						wCtx.spec.Tools.Add(tool)
						wCtx.lock.Tools.Add(tool)
					}

					if err := wCtx.Save(); err != nil {
						return nil, fmt.Errorf("save lock-based workdir: %w", err)
					}

					os.Remove(toolsetFilenameBak)

					return wCtx, nil
				}
			}

			return nil, fmt.Errorf("read lock file: %w", err)
		}

		if err := json.Unmarshal(bb, &lockFile); err != nil {
			return nil, fmt.Errorf("unmarshal lock: %w", err)
		}
	}

	return &Workdir{
		dir:  baseDir,
		spec: spec,
		lock: &lockFile,
		runtimes: map[string]IRuntime{
			"go": runtimego.New(filepath.Join(baseDir, spec.Dir)),
		},
	}, nil
}

func (c *Workdir) getToolsDir() string {
	return filepath.Join(c.dir, c.spec.Dir)
}

func (c *Workdir) getSpecFilename() string {
	return filepath.Join(c.dir, specFilename)
}

func (c *Workdir) getLockFilename() string {
	return filepath.Join(c.dir, lockFilename)
}

func (c *Workdir) Save() error {
	if err := writeJson(*c.spec, c.getSpecFilename()); err != nil {
		return fmt.Errorf("write spec: %w", err)
	}

	if err := writeJson(*c.lock, c.getLockFilename()); err != nil {
		return fmt.Errorf("write lock: %w", err)
	}

	return nil
}

func (c *Workdir) AddInclude(ctx context.Context, source string, tags []string) (int, error) {
	// Check that source is exists and valid.
	remotes, err := fetchRemoteSpec(ctx, source, tags, nil)
	if err != nil {
		return 0, fmt.Errorf("fetch spec: %w", err)
	}

	wasAdded := c.spec.AddInclude(Include{Src: source, Tags: tags})
	if !wasAdded {
		return 0, nil
	}

	c.lock.Remotes = append(c.lock.Remotes, remotes...)

	var count int
	for _, remote := range remotes {
		for _, tool := range remote.Spec.Tools {
			tool.Tags = append(tool.Tags, remote.Tags...)
			c.lock.Tools.Add(tool)
			count++
		}
	}

	return count, nil
}

func (c *Workdir) Add(ctx context.Context, runtime, program string, alias optional.Val[string], tags []string) (bool, string, error) {
	rt, ok := c.runtimes[runtime]
	if !ok {
		return false, "", fmt.Errorf("unsupported runtime: %s", runtime)
	}

	program, err := rt.Parse(ctx, program)
	if err != nil {
		return false, "", fmt.Errorf("parse program: %w", err)
	}

	tool := Tool{
		Runtime: runtime,
		Module:  program,
		Alias:   alias,
		Tags:    tags,
	}
	wasAdded := c.spec.Tools.Add(tool)
	if wasAdded {
		c.lock.Tools.Add(tool)
	}

	return wasAdded, program, nil
}

func (c *Workdir) FindTool(str string) (*Tool, error) {
	for _, tool := range c.lock.Tools {
		rt, ok := c.runtimes[tool.Runtime]
		if !ok {
			return nil, fmt.Errorf("unsupported runtime: %s", tool.Runtime)
		}

		// ...by alias
		if tool.Alias.HasVal() && tool.Alias.Val() == str {
			return &tool, nil
		}

		// ...by canonical binary from module
		if rt.GetProgramName(tool.Module) != str {
			continue
		}

		return &tool, nil
	}

	return nil, fmt.Errorf("tool (%s) not found", str)
}

// RunTool will run a tool by its name and args.
func (c *Workdir) RunTool(ctx context.Context, str string, args ...string) error {
	tool, err := c.FindTool(str)
	if err != nil {
		return err
	}

	rt, ok := c.runtimes[tool.Runtime]
	if !ok {
		return fmt.Errorf("unsupported runtime: %s", tool.Runtime)
	}

	programBinary := rt.GetBinaryPath(tool.Module)
	cmd := exec.CommandContext(ctx, programBinary, args...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("run tool: %w", err)
	}

	return nil
}

// Sync will read the locked tools and try to install the desired version. It will skip the installation in
// case when we have a desired version.
func (c *Workdir) Sync(ctx context.Context, maxWorkers int, tags []string) error {
	toolsDir := c.getToolsDir()
	if !isExists(toolsDir) {
		fmt.Println("Target dir not exists. Creating...", toolsDir)
		if err := os.MkdirAll(toolsDir, 0o755); err != nil {
			return fmt.Errorf("create target dir (%s): %w", toolsDir, err)
		}
	}

	fmt.Println("Target dir:", toolsDir)

	{
		c.lock.Tools = make(Tools, 0)
		for _, tool := range c.spec.Tools {
			c.lock.Tools.Add(tool)
		}

		for _, remote := range c.lock.Remotes {
			for _, tool := range remote.Spec.Tools {
				tool.Tags = append(tool.Tags, remote.Tags...)
				c.lock.Tools.Add(tool)
			}
		}
	}

	errs := make(chan error, len(c.spec.Tools))

	sem := semaphore.NewWeighted(int64(maxWorkers))
	for _, tool := range c.lock.Tools.Filter(tags) {
		fmt.Println("Sync:", tool.Runtime, tool.Module, tool.Alias.ValDefault(""))

		rt, ok := c.runtimes[tool.Runtime]
		if !ok {
			return fmt.Errorf("unsupported runtime: %s", tool.Runtime)
		}

		// NOTE(zhuravlev): do not install tool in case it's directory is exists.
		if rt.IsInstalled(tool.Module) {
			continue
		}

		if err := sem.Acquire(ctx, 1); err != nil {
			return fmt.Errorf("acquire semaphore: %w", err)
		}

		go func() {
			defer sem.Release(1)

			if err := rt.Install(ctx, tool.Module); err != nil {
				errs <- fmt.Errorf("install tool (%s): %w", tool.Module, err)
				return
			}

			if alias, ok := tool.Alias.Get(); ok {
				targetPath := filepath.Join(toolsDir, alias)
				if isExists(targetPath) {
					if err := os.Remove(targetPath); err != nil {
						errs <- fmt.Errorf("remove alias (%s): %w", targetPath, err)
						return
					}
				}

				installedPath := rt.GetBinaryPath(tool.Module)
				if err := os.Symlink(installedPath, targetPath); err != nil {
					errs <- fmt.Errorf("symlink %s to %s: %w", installedPath, targetPath, err)
					return
				}
			}
		}()
	}

	if err := sem.Acquire(ctx, int64(maxWorkers)); err != nil {
		return fmt.Errorf("wait processes to end: %w", err)
	}

	close(errs)

	var allErrors []error
	for err := range errs {
		if err != nil {
			allErrors = append(allErrors, err)
		}
	}

	if len(allErrors) > 0 {
		return fmt.Errorf("errors encountered during sync: %w", errors.Join(allErrors...))
	}

	return nil
}

// Upgrade will upgrade only spec tools. and re-fetch latest versions of includes.
func (c *Workdir) Upgrade(ctx context.Context, tags []string) error {
	for _, tool := range c.spec.Tools.Filter(tags) {
		_, goModule, err := runtimego.GetGoModule(ctx, tool.Module)
		if err != nil {
			return fmt.Errorf("get go module version: %w", err)
		}

		goBinaryWoVersion := strings.Split(tool.Module, runtimego.At)[0]
		latestModule := fmt.Sprintf("%s@%s", goBinaryWoVersion, goModule.Version)

		if tool.Module == latestModule {
			continue
		}

		fmt.Println("Upgrade:", tool.Module, "=>", latestModule)

		tool.Module = latestModule

		c.spec.Tools.AddOrUpdateTool(tool)
		c.lock.Tools.AddOrUpdateTool(tool)
	}

	var resRemotes []RemoteSpec
	for _, inc := range c.spec.Includes {
		remotes, err := fetchRemoteSpec(ctx, inc.Src, inc.Tags, nil)
		if err != nil {
			return fmt.Errorf("fetch remotes: %w", err)
		}

		resRemotes = append(resRemotes, remotes...)
		for _, remote := range remotes {
			for _, tool := range remote.Spec.Tools {
				tool.Tags = append(tool.Tags, remote.Tags...)
				c.lock.Tools.Add(tool)
			}
		}
	}

	c.lock.Remotes = resRemotes

	return nil
}

// CopySource will add all tools from source.
// Source can be a path to file or a http url or git repo.
func (c *Workdir) CopySource(ctx context.Context, source string, tags []string) (int, error) {
	specs, err := fetchRemoteSpec(ctx, source, tags, nil)
	if err != nil {
		return 0, fmt.Errorf("fetch spec: %w", err)
	}

	var count int
	for _, spec := range specs {
		for _, tool := range spec.Spec.Tools {
			tool.Tags = append(tool.Tags, tags...)
			if c.spec.Tools.Add(tool) {
				c.lock.Tools.Add(tool)
				count++
			}
		}
	}

	return count, nil
}

// Init will initialize context in specified directory.
func Init(dir string) (string, error) {
	dir, err := filepath.Abs(dir)
	if err != nil {
		return "", fmt.Errorf("get abs path: %w", err)
	}

	targetSpecFile := filepath.Join(dir, specFilename)
	targetLockFile := filepath.Join(dir, lockFilename)

	switch _, err := os.Stat(targetSpecFile); {
	default:
		return "", fmt.Errorf("check target spec file exists: %w", err)
	case err == nil:
		return "", errors.New("spec already exists")
	case os.IsNotExist(err):
		spec := Spec{
			Dir:      defaultToolsDir,
			Tools:    make([]Tool, 0),
			Includes: make([]Include, 0),
		}
		if err := writeJson(spec, targetSpecFile); err != nil {
			return "", fmt.Errorf("write init spec: %w", err)
		}

		lock := Lock{
			Tools:   make([]Tool, 0),
			Remotes: make([]RemoteSpec, 0),
		}
		if err := writeJson(lock, targetLockFile); err != nil {
			return "", fmt.Errorf("write init lock: %w", err)
		}

		return targetSpecFile, nil
	}
}
