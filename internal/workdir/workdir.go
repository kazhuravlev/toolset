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
	runtimeGo       = "go"
	specFilename    = ".toolset.json"
	lockFilename    = ".toolset.lock.json"
	defaultToolsDir = "./bin/tools"
)

type Workdir struct {
	dir  string
	spec *Spec
	lock *Lock
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
	}, nil
}

func (c *Workdir) GetToolsDir() string {
	return filepath.Join(c.dir, c.spec.Dir)
}

func (c *Workdir) SpecFilename() string {
	return filepath.Join(c.dir, specFilename)
}

func (c *Workdir) LockFilename() string {
	return filepath.Join(c.dir, lockFilename)
}

func (c *Workdir) Save() error {
	if err := writeJson(*c.spec, c.SpecFilename()); err != nil {
		return fmt.Errorf("write spec: %w", err)
	}

	if err := writeJson(*c.lock, c.LockFilename()); err != nil {
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

func (c *Workdir) Add(ctx context.Context, runtime, goBinary string, alias optional.Val[string], tags []string) (bool, string, error) {
	if runtime != runtimeGo {
		return false, "", fmt.Errorf("unsupported runtime: %s", runtime)
	}

	if goBinary == "" {
		return false, "", errors.New("golang binary not provided")
	}

	goBinaryWoVersion := strings.Split(goBinary, runtimego.At)[0]

	_, goModule, err := runtimego.GetGoModule(ctx, goBinary)
	if err != nil {
		return false, "", fmt.Errorf("get go module version: %w", err)
	}

	if strings.Contains(goBinary, "@latest") || !strings.Contains(goBinary, runtimego.At) {
		goBinary = fmt.Sprintf("%s@%s", goBinaryWoVersion, goModule.Version)
	}

	tool := Tool{
		Runtime: runtimeGo,
		Module:  goBinary,
		Alias:   alias,
		Tags:    tags,
	}
	wasAdded := c.spec.Tools.Add(tool)
	if wasAdded {
		c.lock.Tools.Add(tool)
	}

	return wasAdded, goBinaryWoVersion, nil
}

func (c *Workdir) FindTool(str string) (*Tool, error) {
	for _, tool := range c.lock.Tools {
		if tool.Runtime != runtimeGo {
			continue
		}

		// TODO(zhuravlev): do a validation before any actions
		if !strings.Contains(tool.Module, runtimego.At) {
			return nil, fmt.Errorf("go tool (%s) must have a version, at least `latest`", tool.Module)
		}

		binName := runtimego.GetGoBinFromMod(tool.Module)
		if binName != str {
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

	goBinary := runtimego.GetGoInstalledBinary(c.dir, c.spec.Dir, tool.Module)
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
	case runtimeGo:
		return filepath.Join(c.GetToolsDir(), runtimego.GetGoModDir(tool.Module))
	}
}

// Sync will read the locked tools and try to install the desired version. It will skip the installation in
// case when we have a desired version.
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
		if tool.Runtime != runtimeGo {
			return fmt.Errorf("unsupported runtime (%s) for tool (%s)", tool.Runtime, tool.Module)
		}

		if !strings.Contains(tool.Module, runtimego.At) {
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

			if err := runtimego.GoInstall(c.dir, tool.Module, c.spec.Dir, tool.Alias); err != nil {
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
	for _, tool := range c.spec.Tools.Filter(tags) {
		if tool.Runtime != runtimeGo {
			return fmt.Errorf("unsupported runtime (%s) for tool (%s)", tool.Runtime, tool.Module)
		}

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
