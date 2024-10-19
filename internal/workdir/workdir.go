package workdir

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/kazhuravlev/optional"
	"golang.org/x/sync/semaphore"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

const (
	RuntimeGo       = "go"
	SpecFilename    = ".toolset.json"
	LockFilename    = ".toolset.lock.json"
	DefaultToolsDir = "./bin/tools"
)

type Workdir struct {
	Workdir string
	Spec    *Spec
	Lock    *Lock
}

func New() (*Workdir, error) {
	// Make abs path to spec.
	toolsetFilename, err := filepath.Abs(SpecFilename)
	if err != nil {
		return nil, fmt.Errorf("get abs spec path: %w", err)
	}

	// Check that file is exists in current or parent directories.
	for {
		if _, err := os.Stat(toolsetFilename); os.IsNotExist(err) {
			parentDir := filepath.Dir(filepath.Dir(toolsetFilename))
			if filepath.Dir(parentDir) == parentDir {
				return nil, errors.New("unable to find spec in fs tree")
			}

			toolsetFilename = filepath.Join(parentDir, SpecFilename)
			continue
		}

		break
	}

	baseDir := filepath.Dir(toolsetFilename)

	spec, err := readSpec(toolsetFilename)
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
		bb, err := os.ReadFile(filepath.Join(baseDir, LockFilename))
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
						wCtx.Spec.Tools.Add(tool)
						wCtx.Lock.Tools.Add(tool)
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
		Workdir: baseDir,
		Spec:    spec,
		Lock:    &lockFile,
	}, nil
}

func (c *Workdir) GetToolsDir() string {
	return filepath.Join(c.Workdir, c.Spec.Dir)
}

func (c *Workdir) SpecFilename() string {
	return filepath.Join(c.Workdir, SpecFilename)
}

func (c *Workdir) LockFilename() string {
	return filepath.Join(c.Workdir, LockFilename)
}

func (c *Workdir) Save() error {
	if err := writeSpec(c.SpecFilename(), *c.Spec); err != nil {
		return fmt.Errorf("write spec: %w", err)
	}

	if err := writeLock(c.LockFilename(), *c.Lock); err != nil {
		return fmt.Errorf("write lock: %w", err)
	}

	return nil
}

func (c *Workdir) AddInclude(ctx context.Context, source string, tags []string) (int, error) {
	// Check that source is exists and valid.
	remotes, err := fetchRemoteSpec(ctx, source, tags)
	if err != nil {
		return 0, fmt.Errorf("fetch spec: %w", err)
	}

	wasAdded := c.Spec.AddInclude(Include{Src: source, Tags: tags})
	if !wasAdded {
		return 0, nil
	}

	c.Lock.Remotes = append(c.Lock.Remotes, remotes...)

	var count int
	for _, remote := range remotes {
		for _, tool := range remote.Spec.Tools {
			tool.Tags = append(tool.Tags, remote.Tags...)
			c.Lock.Tools.Add(tool)
			count++
		}
	}

	return count, nil
}

func (c *Workdir) AddGo(ctx context.Context, goBinary string, alias optional.Val[string], tags []string) (bool, string, error) {
	goBinaryWoVersion := strings.Split(goBinary, at)[0]

	_, goModule, err := getGoModule(ctx, goBinary)
	if err != nil {
		return false, "", fmt.Errorf("get go module version: %w", err)
	}

	if strings.Contains(goBinary, "@latest") || !strings.Contains(goBinary, at) {
		goBinary = fmt.Sprintf("%s@%s", goBinaryWoVersion, goModule.Version)
	}

	tool := Tool{
		Runtime: RuntimeGo,
		Module:  goBinary,
		Alias:   alias,
		Tags:    tags,
	}
	wasAdded := c.Spec.Tools.Add(tool)
	if wasAdded {
		c.Lock.Tools.Add(tool)
	}

	return wasAdded, goBinaryWoVersion, nil
}

func (c *Workdir) FindTool(str string) (*Tool, error) {
	for _, tool := range c.Lock.Tools {
		if tool.Runtime != RuntimeGo {
			continue
		}

		// TODO(zhuravlev): do a validation before any actions
		if !strings.Contains(tool.Module, at) {
			return nil, fmt.Errorf("go tool (%s) must have a version, at least `latest`", tool.Module)
		}

		binName := getGoBinFromMod(tool.Module)
		if binName != str {
			continue
		}

		return &tool, nil
	}

	return nil, fmt.Errorf("tool (%s) not found", str)
}

func (c *Workdir) RunTool(ctx context.Context, str string, args ...string) error {
	tool, err := c.FindTool(str)
	if err != nil {
		return err
	}

	goBinary := getGoInstalledBinary(c.Workdir, c.Spec.Dir, tool.Module)
	cmd := exec.CommandContext(ctx, goBinary, args...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("run tool: %w", err)
	}

	return nil
}

func (c *Workdir) getToolDir(tool Tool) string {
	switch tool.Runtime {
	default:
		panic("unknown runtime")
	case RuntimeGo:
		return filepath.Join(c.GetToolsDir(), getGoModDir(tool.Module))
	}
}

func (c *Workdir) Sync(ctx context.Context, maxWorkers int, tags []string) error {
	toolsDir := c.GetToolsDir()
	if !isExists(toolsDir) {
		fmt.Println("Target dir not exists. Creating...", toolsDir)
		if err := os.MkdirAll(toolsDir, 0o755); err != nil {
			return fmt.Errorf("create target dir (%s): %w", toolsDir, err)
		}
	}

	fmt.Println("Target dir:", toolsDir)

	{
		c.Lock.Tools = make(Tools, 0)
		for _, tool := range c.Spec.Tools {
			c.Lock.Tools.Add(tool)
		}

		for _, remote := range c.Lock.Remotes {
			for _, tool := range remote.Spec.Tools {
				tool.Tags = append(tool.Tags, remote.Tags...)
				c.Lock.Tools.Add(tool)
			}
		}
	}

	errs := make(chan error, len(c.Spec.Tools))

	sem := semaphore.NewWeighted(int64(maxWorkers))
	for _, tool := range c.Lock.Tools.Filter(tags) {
		fmt.Println("Sync:", tool.Runtime, tool.Module, tool.Alias.ValDefault(""))
		if tool.Runtime != RuntimeGo {
			return fmt.Errorf("unsupported runtime (%s) for tool (%s)", tool.Runtime, tool.Module)
		}

		if !strings.Contains(tool.Module, at) {
			return fmt.Errorf("go tool (%s) must have a version, at least `latest`", tool.Module)
		}

		// NOTE(zhuravlev): do not install tool in case it's directory is exists.
		if isExists(c.getToolDir(tool)) {
			continue
		}

		if err := sem.Acquire(ctx, 1); err != nil {
			return fmt.Errorf("acquire semaphore: %w", err)
		}

		go func() {
			defer sem.Release(1)

			if err := goInstall(c.Workdir, tool.Module, c.Spec.Dir, tool.Alias); err != nil {
				errs <- fmt.Errorf("install tool (%s): %w", tool.Module, err)
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
	for _, tool := range c.Spec.Tools.Filter(tags) {
		if tool.Runtime != RuntimeGo {
			return fmt.Errorf("unsupported runtime (%s) for tool (%s)", tool.Runtime, tool.Module)
		}

		_, goModule, err := getGoModule(ctx, tool.Module)
		if err != nil {
			return fmt.Errorf("get go module version: %w", err)
		}

		goBinaryWoVersion := strings.Split(tool.Module, at)[0]
		latestModule := fmt.Sprintf("%s@%s", goBinaryWoVersion, goModule.Version)

		if tool.Module == latestModule {
			continue
		}

		fmt.Println("Upgrade:", tool.Module, "=>", latestModule)

		tool.Module = latestModule

		c.Spec.Tools.AddOrUpdateTool(tool)
		c.Lock.Tools.AddOrUpdateTool(tool)
	}

	var resRemotes []RemoteSpec
	for _, inc := range c.Spec.Includes {
		remotes, err := fetchRemoteSpec(ctx, inc.Src, inc.Tags)
		if err != nil {
			return fmt.Errorf("fetch remotes: %w", err)
		}

		// FIXME(zhuravlev): remove tools from prev remotes before add a new one.
		resRemotes = append(resRemotes, remotes...)
		for _, remote := range remotes {
			for _, tool := range remote.Spec.Tools {
				tool.Tags = append(tool.Tags, remote.Tags...)
				c.Lock.Tools.Add(tool)
			}
		}
	}

	c.Lock.Remotes = resRemotes

	return nil
}

// CopySource will add all tools from source.
// Source can be a path to file or a http url.
func (c *Workdir) CopySource(ctx context.Context, source string, tags []string) (int, error) {
	specs, err := fetchRemoteSpec(ctx, source, tags)
	if err != nil {
		return 0, fmt.Errorf("fetch spec: %w", err)
	}

	var count int
	for _, spec := range specs {
		for _, tool := range spec.Spec.Tools {
			tool.Tags = append(tool.Tags, tags...)
			if c.Spec.Tools.Add(tool) {
				c.Lock.Tools.Add(tool)
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

	targetSpecFile := filepath.Join(dir, SpecFilename)
	targetLockFile := filepath.Join(dir, LockFilename)

	switch _, err := os.Stat(targetSpecFile); {
	default:
		return "", fmt.Errorf("check target spec file exists: %w", err)
	case err == nil:
		return "", errors.New("spec already exists")
	case os.IsNotExist(err):
		spec := Spec{
			Dir:      DefaultToolsDir,
			Tools:    make([]Tool, 0),
			Includes: make([]Include, 0),
		}
		if err := writeSpec(targetSpecFile, spec); err != nil {
			return "", fmt.Errorf("write init spec: %w", err)
		}

		lock := Lock{
			Tools:   make([]Tool, 0),
			Remotes: make([]RemoteSpec, 0),
		}
		if err := writeLock(targetLockFile, lock); err != nil {
			return "", fmt.Errorf("write init lock: %w", err)
		}

		return targetSpecFile, nil
	}
}
