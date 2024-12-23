package workdir

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/kazhuravlev/optional"
	runtimego "github.com/kazhuravlev/toolset/internal/workdir/runtime-go"
	"github.com/kazhuravlev/toolset/internal/workdir/structs"
	"golang.org/x/sync/semaphore"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const (
	// This files is placed in project root
	specFilename    = ".toolset.json"
	lockFilename    = ".toolset.lock.json"
	defaultToolsDir = "./bin/tools"
	// This file is places in tools directory
	statsFilename  = ".stats.json"
	defaultDirPerm = 0o755
)

const (
	StatsVer1 = "v1"
)

var ErrToolNotFoundInSpec = errors.New("tool not found in spec")
var ErrToolNotInstalled = errors.New("tool not installed")

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
}

type Workdir struct {
	dir      string
	spec     *Spec
	lock     *Lock
	stats    *Stats
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

					wd, err := New()
					if err != nil {
						return nil, fmt.Errorf("new context in re-created workdir: %w", err)
					}

					for _, tool := range spec.Tools {
						wd.spec.Tools.Add(tool)
					}

					wd.lock.FromSpec(spec)

					if err := wd.Save(); err != nil {
						return nil, fmt.Errorf("save lock-based workdir: %w", err)
					}

					os.Remove(toolsetFilenameBak)

					return wd, nil
				}
			}

			return nil, fmt.Errorf("read lock file: %w", err)
		}

		if err := json.Unmarshal(bb, &lockFile); err != nil {
			return nil, fmt.Errorf("unmarshal lock: %w", err)
		}
	}

	absToolsDir := filepath.Join(baseDir, spec.Dir)

	statsFName := filepath.Join(absToolsDir, statsFilename)
	statsFile, err := forceReadJson(statsFName, Stats{
		Version: StatsVer1,
		Tools:   make(map[string]time.Time),
	})
	if err != nil {
		return nil, fmt.Errorf("read stats: %w", err)
	}

	return &Workdir{
		dir:   baseDir,
		spec:  spec,
		lock:  &lockFile,
		stats: statsFile,
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

func (c *Workdir) getStatsFilename() string {
	return filepath.Join(c.getToolsDir(), statsFilename)
}

func (c *Workdir) Save() error {
	if err := writeJson(*c.spec, c.getSpecFilename()); err != nil {
		return fmt.Errorf("write spec: %w", err)
	}

	if err := writeJson(*c.lock, c.getLockFilename()); err != nil {
		return fmt.Errorf("write lock: %w", err)
	}

	if err := c.saveStats(); err != nil {
		return fmt.Errorf("save stats: %w", err)
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

	c.lock.FromSpec(c.spec)

	var count int
	for _, remote := range remotes {
		count += len(remote.Spec.Tools)
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

	tool := structs.Tool{
		Runtime: runtime,
		Module:  program,
		Alias:   alias,
		Tags:    tags,
	}
	wasAdded := c.spec.Tools.Add(tool)
	if wasAdded {
		c.lock.FromSpec(c.spec)
	}

	return wasAdded, program, nil
}

func (c *Workdir) FindTool(name string) (*ToolState, error) {
	for _, tool := range c.lock.Tools {
		mod, err := c.getModuleInfo(context.TODO(), tool)
		if err != nil {
			return nil, fmt.Errorf("get module (%s) info: %w", tool.Module, err)
		}

		// ...by alias
		if tool.Alias.HasVal() && tool.Alias.Val() == name {
			lastUse := c.getToolLastUse(tool.ID())
			res := adaptToolState(tool, mod, lastUse)

			return &res, nil
		}

		// ...by canonical binary from module
		if mod.Name != name {
			continue
		}

		lastUse := c.getToolLastUse(tool.ID())
		res := adaptToolState(tool, mod, lastUse)

		return &res, nil
	}

	return nil, fmt.Errorf("tool (%s) not found: %w", name, ErrToolNotFoundInSpec)
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

	c.stats.Tools[tool.ID()] = time.Now()
	if err := c.saveStats(); err != nil {
		return fmt.Errorf("save stats: %w", err)
	}

	if err := rt.Run(ctx, tool.Module, args...); err != nil {
		if errors.Is(err, structs.ErrToolNotInstalled) {
			return fmt.Errorf("run tool: %w", errors.Join(err, ErrToolNotInstalled))
		}

		var errRun *structs.RunError
		if errors.As(err, &errRun) {
			return fmt.Errorf("exit not zero: %w", err)
		}

		return fmt.Errorf("run tool: %w", err)
	}

	return nil
}

// Sync will read the locked tools and try to install the desired version. It will skip the installation in
// case when we have a desired version.
func (c *Workdir) Sync(ctx context.Context, maxWorkers int, tags []string) error {
	if toolsDir := c.getToolsDir(); !isExists(toolsDir) {
		fmt.Println("Target dir not exists. Creating...", toolsDir)
		if err := os.MkdirAll(toolsDir, defaultDirPerm); err != nil {
			return fmt.Errorf("create target dir (%s): %w", toolsDir, err)
		}
	}

	c.lock.FromSpec(c.spec)

	errs := make(chan error, len(c.spec.Tools))

	sem := semaphore.NewWeighted(int64(maxWorkers))
	for _, tool := range c.lock.Tools.Filter(tags) {
		fmt.Println("Sync:", tool.Runtime, tool.Module, tool.Alias.ValDefault(""))

		rt, ok := c.runtimes[tool.Runtime]
		if !ok {
			return fmt.Errorf("unsupported runtime: %s", tool.Runtime)
		}

		mod, err := rt.GetModule(ctx, tool.Module)
		if err != nil {
			return fmt.Errorf("get module (%s) info: %w", tool.Module, err)
		}

		if mod.IsInstalled {
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
				targetPath := filepath.Join(c.getToolsDir(), alias)
				if isExists(targetPath) {
					if err := os.Remove(targetPath); err != nil {
						errs <- fmt.Errorf("remove alias (%s): %w", targetPath, err)
						return
					}
				}

				mod, err := rt.GetModule(ctx, tool.Module)
				if err != nil {
					errs <- fmt.Errorf("get module (%s) info: %w", tool.Module, err)
					return
				}

				installedPath := mod.BinPath
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
		fmt.Println("Checking:", tool.Module, "...")

		// FIXME(zhuravlev): remove all "is runtime supported" checks by checking it once at spec load.
		rt, ok := c.runtimes[tool.Runtime]
		if !ok {
			return fmt.Errorf("unsupported runtime: %s", tool.Runtime)
		}

		module, haveUpdate, err := rt.GetLatest(ctx, tool.Module)
		if err != nil {
			return fmt.Errorf("get latest module: %w", err)
		}

		if !haveUpdate {
			fmt.Println(">>> Have no updates")
			continue
		}

		fmt.Println(">>> Upgrade to:", module)

		tool.Module = module

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
			// TODO(zhuravlev): should we use an official way to add a tool (like `c.Add()`)?
			if c.spec.Tools.Add(tool) {
				count++
			}
		}
	}

	c.lock.FromSpec(c.spec)

	return count, nil
}

// ToolState describe a state of this tool.
type ToolState struct {
	Module  structs.ModuleInfo
	Tool    structs.Tool
	LastUse optional.Val[time.Time]
}

func (c *Workdir) GetTools(ctx context.Context) ([]ToolState, error) {
	res := make([]ToolState, 0, len(c.lock.Tools))
	for _, tool := range c.lock.Tools {
		mod, err := c.getModuleInfo(ctx, tool)
		if err != nil {
			return nil, fmt.Errorf("get module info: %w", err)
		}

		lastUse := c.getToolLastUse(tool.ID())

		res = append(res, adaptToolState(tool, mod, lastUse))
	}

	return res, nil
}

func (c *Workdir) getModuleInfo(ctx context.Context, tool structs.Tool) (*structs.ModuleInfo, error) {
	rt, ok := c.runtimes[tool.Runtime]
	if !ok {
		return nil, fmt.Errorf("unsupported runtime: %s", tool.Runtime)
	}

	mod, err := rt.GetModule(ctx, tool.Module)
	if err != nil {
		return nil, fmt.Errorf("get module (%s): %w", tool.Module, err)
	}

	return mod, nil
}

func (c *Workdir) saveStats() error {
	return writeJson(*c.stats, c.getStatsFilename())
}

func (c *Workdir) getToolLastUse(id string) optional.Val[time.Time] {
	if val, ok := c.stats.Tools[id]; ok {
		return optional.New(val)
	}

	return optional.Empty[time.Time]()
}

// Init will initialize context in specified directory.
func Init(dir string) (string, error) {
	dir, err := filepath.Abs(dir)
	if err != nil {
		return "", fmt.Errorf("get abs path: %w", err)
	}

	absToolsDir := filepath.Join(dir, defaultToolsDir)
	if err := os.MkdirAll(absToolsDir, defaultDirPerm); err != nil {
		return "", fmt.Errorf("create tools dir: %w", err)
	}

	targetSpecFile := filepath.Join(dir, specFilename)
	targetLockFile := filepath.Join(dir, lockFilename)
	targetStatsFile := filepath.Join(absToolsDir, statsFilename)

	switch _, err := os.Stat(targetSpecFile); {
	default:
		return "", fmt.Errorf("check target spec file exists: %w", err)
	case err == nil:
		return "", errors.New("spec already exists")
	case os.IsNotExist(err):
		spec := Spec{
			Dir:      defaultToolsDir,
			Tools:    make(structs.Tools, 0),
			Includes: make([]Include, 0),
		}
		if err := writeJson(spec, targetSpecFile); err != nil {
			return "", fmt.Errorf("write init spec: %w", err)
		}

		lock := Lock{
			Tools:   make(structs.Tools, 0),
			Remotes: make([]RemoteSpec, 0),
		}
		if err := writeJson(lock, targetLockFile); err != nil {
			return "", fmt.Errorf("write init lock: %w", err)
		}

		_, errStats := forceReadJson(targetStatsFile, Stats{
			Version: StatsVer1,
			Tools:   make(map[string]time.Time),
		})
		if err := errStats; err != nil {
			return "", fmt.Errorf("write init stats: %w", err)
		}

		return targetSpecFile, nil
	}
}

func adaptToolState(tool structs.Tool, mod *structs.ModuleInfo, lastUse optional.Val[time.Time]) ToolState {
	return ToolState{
		Tool:    tool,
		LastUse: lastUse,
		Module:  *mod,
	}
}
