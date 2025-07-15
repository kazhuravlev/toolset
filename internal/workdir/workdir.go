package workdir

import (
	"context"
	"errors"
	"fmt"
	"github.com/kazhuravlev/optional"
	"github.com/kazhuravlev/toolset/internal/fsh"
	remotes2 "github.com/kazhuravlev/toolset/internal/workdir/remotes"
	runtimes "github.com/kazhuravlev/toolset/internal/workdir/runtimes"
	"github.com/kazhuravlev/toolset/internal/workdir/structs"
	"golang.org/x/sync/semaphore"
	"os"
	"path/filepath"
	"time"
)

const (
	// This files is placed in project root
	specFilename = ".toolset.json"
	lockFilename = ".toolset.lock.json"
	// This file is places in tools directory
	statsFilename = ".stats.json"
)

const (
	StatsVer1 = "v1"
)

var (
	ErrToolNotFoundInSpec = errors.New("tool not found in spec")
	ErrToolNotInstalled   = errors.New("tool not installed")
)

type Workdir struct {
	cacheDir    string
	projectRoot string
	spec        *structs.Spec
	lock        *structs.Lock
	stats       *structs.Stats
	runtimes    *runtimes.Runtimes
	fs          fsh.FS
}

func New(ctx context.Context, fs fsh.FS, dir string) (*Workdir, error) {
	cacheDir := "~/.cache/toolset"
	if cacheDirEnv := os.Getenv("TOOLSET_CACHE_DIR"); cacheDirEnv != "" {
		cacheDir = cacheDirEnv
	}

	cacheDir, err := filepath.Abs(cacheDir)
	if err != nil {
		return nil, fmt.Errorf("resolve cache dir: %w", err)
	}

	// Make an abs path to spec.
	toolsetFilename, err := filepath.Abs(filepath.Join(dir, specFilename))
	if err != nil {
		return nil, fmt.Errorf("get abs spec path: %w", err)
	}

	// Check that file is exists in current or parent directories.
	for {
		if !fsh.IsExists(fs, toolsetFilename) {
			parentDir := filepath.Dir(filepath.Dir(toolsetFilename))
			if filepath.Dir(parentDir) == parentDir {
				return nil, errors.New("unable to find spec in fs tree")
			}

			toolsetFilename = filepath.Join(parentDir, specFilename)
			continue
		}

		break
	}

	dir = filepath.Dir(toolsetFilename)
	lockFname := filepath.Join(dir, lockFilename)

	spec, err := fsh.ReadJson[structs.Spec](fs, toolsetFilename)
	if err != nil {
		return nil, fmt.Errorf("spec file not found: %w", err)
	}

	if spec.Dir != "" {
		fmt.Println("'dir' parameter is deprecated. It will be removed automatically. toolset@v0.27.0 store all downloads into one global cache directory. Check TOOLSET_CACHE_DIR env.")
		spec.Dir = ""
	}

	lockFile, err := fsh.ReadJson[structs.Lock](fs, lockFname)
	if err != nil {
		return nil, fmt.Errorf("read lock file: %w", err)
	}

	statsFName := filepath.Join(cacheDir, statsFilename)
	statsFile, err := fsh.ReadOrCreateJson(fs, statsFName, structs.Stats{
		Version: StatsVer1,
		Tools:   make(map[string]time.Time),
	})
	if err != nil {
		return nil, fmt.Errorf("read stats: %w", err)
	}

	rnTimes, err := runtimes.New(fs, cacheDir)
	if err != nil {
		return nil, fmt.Errorf("new runtimes: %w", err)
	}

	if err := rnTimes.Discover(ctx); err != nil {
		return nil, fmt.Errorf("discover runtimes: %w", err)
	}

	return &Workdir{
		fs:          fs,
		cacheDir:    cacheDir,
		projectRoot: dir,
		spec:        spec,
		lock:        lockFile,
		stats:       statsFile,
		runtimes:    rnTimes,
	}, nil
}

// Init will initialize context in specified directory.
func Init(fs fsh.FS, dir string) error {
	const defaultToolsDir = "./bin/tools"

	dir, err := filepath.Abs(dir)
	if err != nil {
		return fmt.Errorf("get abs path: %w", err)
	}

	absToolsDir := filepath.Join(dir, defaultToolsDir)
	if err := fs.MkdirAll(absToolsDir, fsh.DefaultDirPerm); err != nil {
		return fmt.Errorf("create tools dir: %w", err)
	}

	targetSpecFile := filepath.Join(dir, specFilename)
	targetLockFile := filepath.Join(dir, lockFilename)
	targetStatsFile := filepath.Join(absToolsDir, statsFilename)

	switch _, err := fs.Stat(targetSpecFile); {
	default:
		return fmt.Errorf("check target spec file exists: %w", err)
	case err == nil:
		return errors.New("spec already exists")
	case os.IsNotExist(err):
		spec := structs.Spec{
			Dir:      defaultToolsDir,
			Tools:    make(structs.Tools, 0),
			Includes: make([]structs.Include, 0),
		}
		if err := fsh.WriteJson(fs, spec, targetSpecFile); err != nil {
			return fmt.Errorf("write init spec: %w", err)
		}

		lock := structs.Lock{
			Tools:   make(structs.Tools, 0),
			Remotes: make([]structs.RemoteSpec, 0),
		}
		if err := fsh.WriteJson(fs, lock, targetLockFile); err != nil {
			return fmt.Errorf("write init lock: %w", err)
		}

		_, errStats := fsh.ReadOrCreateJson(fs, targetStatsFile, structs.Stats{
			Version: StatsVer1,
			Tools:   make(map[string]time.Time),
		})
		if err := errStats; err != nil {
			return fmt.Errorf("write init stats: %w", err)
		}

		return nil
	}
}

func (c *Workdir) Save() error {
	if err := fsh.WriteJson(c.fs, *c.spec, c.getSpecFilename()); err != nil {
		return fmt.Errorf("write spec: %w", err)
	}

	if err := fsh.WriteJson(c.fs, *c.lock, c.getLockFilename()); err != nil {
		return fmt.Errorf("write lock: %w", err)
	}

	if err := c.saveStats(); err != nil {
		return fmt.Errorf("save stats: %w", err)
	}

	return nil
}

func (c *Workdir) AddInclude(ctx context.Context, source string, tags []string) (int, error) {
	// Check that source is exists and valid.
	remotes, err := remotes2.FetchRemote(ctx, c.fs, source, tags, nil)
	if err != nil {
		return 0, fmt.Errorf("fetch spec: %w", err)
	}

	wasAdded := c.spec.AddInclude(structs.Include{Src: source, Tags: tags})
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
	rt, err := c.runtimes.Get(runtime)
	if err != nil {
		return false, "", fmt.Errorf("get runtime: %w", err)
	}

	program, err = rt.Parse(ctx, program)
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

func (c *Workdir) RemoveTool(ctx context.Context, target string) error {
	ts, err := c.FindTool(target)
	if err != nil {
		return fmt.Errorf("find tool: %w", err)
	}

	if ts.Module.IsInstalled {
		rt, err := c.runtimes.Get(ts.Tool.Runtime)
		if err != nil {
			return fmt.Errorf("get runtime: %w", err)
		}

		if err := rt.Remove(ctx, ts.Tool); err != nil {
			return fmt.Errorf("remove tool: %w", err)
		}
	}

	_ = c.lock.Tools.Remove(ts.Tool)
	_ = c.spec.Tools.Remove(ts.Tool)
	delete(c.stats.Tools, ts.Tool.ID())

	return nil
}

func (c *Workdir) FindTool(name string) (*structs.ToolState, error) {
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
	const autoInstallProgram = true

	ts, err := c.FindTool(str)
	if err != nil {
		return err
	}

	rt, err := c.runtimes.GetInstall(ctx, ts.Tool.Runtime)
	if err != nil {
		return fmt.Errorf("get or install runtime: %w", err)
	}

	c.stats.Tools[ts.Tool.ID()] = time.Now()
	if err := c.saveStats(); err != nil {
		return fmt.Errorf("save stats: %w", err)
	}

RunProgram:
	if err := rt.Run(ctx, ts.Tool.Module, args...); err != nil {
		if errors.Is(err, structs.ErrToolNotInstalled) {
			if autoInstallProgram {
				if err := rt.Install(ctx, ts.Tool.Module); err != nil {
					return fmt.Errorf("auto-install not-installed program (%s) before run: %w", ts.Tool.Module, err)
				}

				goto RunProgram
			}

			return fmt.Errorf("run tool: %w", errors.Join(err, ErrToolNotInstalled))
		}

		var errRun structs.RunError
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
	if toolsDir := c.getToolsDir(); !fsh.IsExists(c.fs, toolsDir) {
		if err := c.fs.MkdirAll(toolsDir, fsh.DefaultDirPerm); err != nil {
			return fmt.Errorf("create target dir (%s): %w", toolsDir, err)
		}
	}

	c.lock.FromSpec(c.spec)

	errs := make(chan error, len(c.spec.Tools))

	sem := semaphore.NewWeighted(int64(maxWorkers))
	for _, tool := range c.lock.Tools.Filter(tags) {
		fmt.Println("Sync:", tool.Runtime, tool.Module, tool.Alias.ValDefault(""))

		rt, err := c.runtimes.GetInstall(ctx, tool.Runtime)
		if err != nil {
			return fmt.Errorf("get runtime: %w", err)
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
				if fsh.IsExists(c.fs, targetPath) {
					if err := c.fs.Remove(targetPath); err != nil {
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
				if err := c.fs.SymlinkIfPossible(installedPath, targetPath); err != nil {
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
		rt, err := c.runtimes.Get(tool.Runtime)
		if err != nil {
			return fmt.Errorf("get runtime: %w", err)
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

		c.spec.Tools.UpsertTool(tool)
		c.lock.Tools.UpsertTool(tool)
	}

	resRemotes := make([]structs.RemoteSpec, 0, len(c.spec.Includes))
	for _, inc := range c.spec.Includes {
		remotes, err := remotes2.FetchRemote(ctx, c.fs, inc.Src, inc.Tags, nil)
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
	specs, err := remotes2.FetchRemote(ctx, c.fs, source, tags, nil)
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

func (c *Workdir) GetTools(ctx context.Context) ([]structs.ToolState, error) {
	res := make([]structs.ToolState, 0, len(c.lock.Tools))
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

func (c *Workdir) RuntimeAdd(ctx context.Context, runtime string) error {
	if err := c.runtimes.EnsureInstalled(ctx, runtime); err != nil {
		return fmt.Errorf("install runtime: %w", err)
	}

	return nil
}

func (c *Workdir) RuntimeList() []string {
	return c.runtimes.List()
}

func (c *Workdir) getModuleInfo(ctx context.Context, tool structs.Tool) (*structs.ModuleInfo, error) {
	rt, err := c.runtimes.Get(tool.Runtime)
	if err != nil {
		return nil, fmt.Errorf("get runtime: %w", err)
	}

	mod, err := rt.GetModule(ctx, tool.Module)
	if err != nil {
		return nil, fmt.Errorf("get module (%s): %w", tool.Module, err)
	}

	return mod, nil
}

func (c *Workdir) saveStats() error {
	return fsh.WriteJson(c.fs, *c.stats, c.getStatsFilename())
}

func (c *Workdir) getToolLastUse(id string) optional.Val[time.Time] {
	if val, ok := c.stats.Tools[id]; ok {
		return optional.New(val)
	}

	return optional.Empty[time.Time]()
}

func (c *Workdir) getToolsDir() string {
	return filepath.Join(c.projectRoot, c.spec.Dir)
}

func (c *Workdir) getSpecFilename() string {
	return filepath.Join(c.projectRoot, specFilename)
}

func (c *Workdir) getLockFilename() string {
	return filepath.Join(c.projectRoot, lockFilename)
}

func (c *Workdir) getStatsFilename() string {
	return filepath.Join(c.getToolsDir(), statsFilename)
}
