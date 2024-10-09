package workdir

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/kazhuravlev/optional"
	"golang.org/x/sync/semaphore"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

const (
	RuntimeGo        = "go"
	SpecFilename     = ".toolset.json"
	SnapshotFilename = ".snapshot.json"
	DefaultToolsDir  = "./bin/tools"
)

type Context struct {
	Workdir string
	Spec    *Spec
}

func NewContext() (*Context, error) {
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

	return &Context{
		Workdir: baseDir,
		Spec:    spec,
	}, nil
}

func (c *Context) GetToolsDir() string {
	return filepath.Join(c.Workdir, c.Spec.Dir)
}

func (c *Context) SpecFilename() string {
	return filepath.Join(c.Workdir, SpecFilename)
}

func (c *Context) Save() error {
	if err := writeSpec(c.SpecFilename(), *c.Spec); err != nil {
		return fmt.Errorf("write spec: %w", err)
	}

	return nil
}

func (c *Context) AddGo(ctx context.Context, goBinary string, alias optional.Val[string]) (bool, string, error) {
	goBinaryWoVersion := strings.Split(goBinary, at)[0]

	_, goModule, err := getGoModule(ctx, goBinary)
	if err != nil {
		return false, "", fmt.Errorf("get go module version: %w", err)
	}

	if strings.Contains(goBinary, "@latest") || !strings.Contains(goBinary, at) {
		goBinary = fmt.Sprintf("%s@%s", goBinaryWoVersion, goModule.Version)
	}

	wasAdded := c.Spec.AddTool(Tool{
		Runtime: RuntimeGo,
		Module:  goBinary,
		Alias:   alias,
	})

	return wasAdded, goBinaryWoVersion, nil
}

func (c *Context) FindTool(str string) (*Tool, error) {
	for _, tool := range c.Spec.Tools {
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

func (c *Context) RunTool(ctx context.Context, str string, args ...string) error {
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

func (c *Context) Sync(ctx context.Context, maxWorkers int) error {
	toolsDir := c.GetToolsDir()
	if _, err := os.Stat(toolsDir); os.IsNotExist(err) {
		fmt.Println("Target dir not exists. Creating...", toolsDir)
		if err := os.MkdirAll(toolsDir, 0o755); err != nil {
			return fmt.Errorf("create target dir (%s): %w", toolsDir, err)
		}
	}

	fmt.Println("Target dir:", toolsDir)

	// TODO: remove all unknown aliases

	errs := make(chan error, len(c.Spec.Tools))

	sem := semaphore.NewWeighted(int64(maxWorkers))
	for _, tool := range c.Spec.Tools {
		fmt.Println("Sync:", tool.Runtime, tool.Module, tool.Alias.ValDefault(""))
		if tool.Runtime != RuntimeGo {
			return fmt.Errorf("unsupported runtime (%s) for tool (%s)", tool.Runtime, tool.Module)
		}

		if !strings.Contains(tool.Module, at) {
			return fmt.Errorf("go tool (%s) must have a version, at least `latest`", tool.Module)
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

func (c *Context) Upgrade(ctx context.Context) error {
	for _, tool := range c.Spec.Tools {
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

		c.Spec.AddOrUpdateTool(tool)
	}

	return nil
}

// InitContext will initialize context in specified
func InitContext(dir string) (string, error) {
	dir, err := filepath.Abs(dir)
	if err != nil {
		return "", fmt.Errorf("get abs path: %w", err)
	}

	targetSpecFile := filepath.Join(dir, SpecFilename)

	switch _, err := os.Stat(targetSpecFile); {
	default:
		return "", fmt.Errorf("check target spec file exists: %w", err)
	case err == nil:
		return "", errors.New("spec already exists")
	case os.IsNotExist(err):
		spec := Spec{
			Dir:   DefaultToolsDir,
			Tools: make([]Tool, 0),
		}
		if err := writeSpec(targetSpecFile, spec); err != nil {
			return "", fmt.Errorf("write init spec: %w", err)
		}

		return targetSpecFile, nil
	}
}

type Tool struct {
	// Name of runtime
	Runtime string `json:"runtime"`
	// Path to module with version
	Module string `json:"module"`
	// Alias create a link in tools. Works like exposing some tools
	Alias optional.Val[string] `json:"alias"`
}

func (t Tool) IsSame(tool Tool) bool {
	if t.Runtime != RuntimeGo {
		panic("not implemented")
	}

	if t.Runtime != tool.Runtime {
		return false
	}

	m1 := strings.Split(t.Module, "@")[0]
	m2 := strings.Split(tool.Module, "@")[0]

	return m1 == m2
}

type Spec struct {
	Dir   string `json:"dir"`
	Tools []Tool `json:"tools"`
}

func (s *Spec) AddTool(tool Tool) bool {
	for _, t := range s.Tools {
		if t.IsSame(tool) {
			return false
		}
	}

	s.Tools = append(s.Tools, tool)
	return true
}

func (s *Spec) AddOrUpdateTool(tool Tool) {
	for i, t := range s.Tools {
		if t.IsSame(tool) {
			s.Tools[i] = tool
			return
		}
	}

	s.Tools = append(s.Tools, tool)
}

func readSpec(path string) (*Spec, error) {
	bb, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read spec file (%s): %w", path, err)
	}

	var spec Spec
	if err := json.Unmarshal(bb, &spec); err != nil {
		return nil, fmt.Errorf("unmarshal spec (%s): %w", path, err)
	}

	return &spec, nil
}

func writeSpec(path string, spec Spec) error {
	file, err := os.OpenFile(path, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o644)
	if err != nil {
		return fmt.Errorf("open spec: %w", err)
	}

	enc := json.NewEncoder(file)
	enc.SetIndent("", "\t")

	if err := enc.Encode(spec); err != nil {
		return fmt.Errorf("marshal spec: %w", err)
	}

	return nil
}

type GoModule struct {
	Version string    `json:"Version"`
	Time    time.Time `json:"Time"`
	Origin  struct {
		VCS  string `json:"VCS"`
		URL  string `json:"URL"`
		Hash string `json:"Hash"`
		Ref  string `json:"Ref"`
	} `json:"Origin"`
}

func getGoModuleName(link string) (string, error) {
	link = strings.Split(link, "@")[0]

	for {
		// TODO: use a local proxy if configured.
		resp, err := http.Get(fmt.Sprintf("https://proxy.golang.org/%s/@latest", link))
		if err != nil {
			return "", fmt.Errorf("do request to golang proxy: %w", err)
		}
		resp.Body.Close()

		if resp.StatusCode == http.StatusOK {
			return link, nil
		}

		if resp.StatusCode == http.StatusNotFound {
			parts := strings.Split(link, "/")
			if len(parts) == 1 {
				break
			}

			link = strings.Join(parts[:len(parts)-1], "/")
		}
	}

	return "", errors.New("unknown module")
}

func getGoModule(ctx context.Context, link string) (string, *GoModule, error) {
	module, err := getGoModuleName(link)
	if err != nil {
		return "", nil, fmt.Errorf("get go module name: %w", err)
	}

	// TODO: use a proxy from env
	// Get the latest version
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, fmt.Sprintf("https://proxy.golang.org/%s/@latest", module), nil)
	if err != nil {
		return "", nil, fmt.Errorf("create request: %w", err)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", nil, fmt.Errorf("get go module: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", nil, fmt.Errorf("unable to get module: %s", resp.Status)
	}

	var mod GoModule
	if err := json.NewDecoder(resp.Body).Decode(&mod); err != nil {
		return "", nil, fmt.Errorf("unable to decode module: %w", err)
	}

	return module, &mod, nil
}

func getGoInstalledBinary(baseDir, goBinDir, mod string) string {
	modDir := filepath.Join(baseDir, goBinDir, getGoModDir(mod))
	return filepath.Join(modDir, getGoBinFromMod(mod))
}

func goInstall(baseDir, mod, goBinDir string, alias optional.Val[string]) error {
	const golang = "go"

	modDir := filepath.Join(baseDir, goBinDir, getGoModDir(mod))
	if err := os.MkdirAll(modDir, 0o755); err != nil {
		return fmt.Errorf("create mod dir (%s): %w", modDir, err)
	}

	cmd := &exec.Cmd{
		Path: golang,
		Args: []string{golang, "install", mod},
		Env: append(os.Environ(),
			"GOBIN="+modDir,
		),
	}

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

	installedPath := getGoInstalledBinary(baseDir, goBinDir, mod)

	if alias, ok := alias.Get(); ok {
		targetPath := filepath.Join(baseDir, goBinDir, alias)
		if _, err := os.Stat(targetPath); err == nil {
			if err := os.Remove(targetPath); err != nil {
				return fmt.Errorf("remove alias (%s): %w", targetPath, err)
			}
		}

		if err := os.Symlink(installedPath, targetPath); err != nil {
			return fmt.Errorf("symlink %s to %s: %w", installedPath, targetPath, err)
		}
	}

	return nil
}

const at = "@"

// getGoBinFromMod returns a binary name that installed by `go install`
// github.com/golangci/golangci-lint/cmd/golangci-lint@v1.55.2 ==> golangci-lint
func getGoBinFromMod(mod string) string {
	// github.com/user/repo@v1.0.0 => github.com/user/repo
	if strings.Contains(mod, at) {
		mod = strings.Split(mod, at)[0]
	}

	// github.com/user/repo/cmd/some/program => program
	if strings.Contains(mod, "/cmd/") {
		mod = strings.Split(mod, "/cmd/")[1]
		return filepath.Base(mod)
	}

	parts := strings.Split(mod, "/")
	// github.com/user/repo/v3 => repo
	if strings.HasPrefix(parts[len(parts)-1], "v") {
		prevPart := parts[len(parts)-2]
		return prevPart
	}

	return filepath.Base(mod)
}

// getGoModDir returns a dir that will keep all mod-related stuff for specific version.
func getGoModDir(mod string) string {
	binName := getGoBinFromMod(mod)
	parts := strings.Split(mod, at)
	version := parts[1]

	return fmt.Sprintf(".%s___%s", binName, version)
}
