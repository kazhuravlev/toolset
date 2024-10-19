package workdir

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/kazhuravlev/optional"
	"golang.org/x/sync/semaphore"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"time"
)

const (
	RuntimeGo       = "go"
	SpecFilename    = ".toolset.json"
	LockFilename    = ".toolset.lock.json"
	DefaultToolsDir = "./bin/tools"
)

type Context struct {
	Workdir string
	Spec    *Spec
	Lock    *Lock
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

					if _, err := InitContext(baseDir); err != nil {
						return nil, fmt.Errorf("re-init toolset: %w", err)
					}

					wCtx, err := NewContext()
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

	return &Context{
		Workdir: baseDir,
		Spec:    spec,
		Lock:    &lockFile,
	}, nil
}

func (c *Context) GetToolsDir() string {
	return filepath.Join(c.Workdir, c.Spec.Dir)
}

func (c *Context) SpecFilename() string {
	return filepath.Join(c.Workdir, SpecFilename)
}

func (c *Context) LockFilename() string {
	return filepath.Join(c.Workdir, LockFilename)
}

func (c *Context) Save() error {
	if err := writeSpec(c.SpecFilename(), *c.Spec); err != nil {
		return fmt.Errorf("write spec: %w", err)
	}

	if err := writeLock(c.LockFilename(), *c.Lock); err != nil {
		return fmt.Errorf("write lock: %w", err)
	}

	return nil
}

func (c *Context) AddInclude(ctx context.Context, source string, tags []string) (int, error) {
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

func (c *Context) AddGo(ctx context.Context, goBinary string, alias optional.Val[string], tags []string) (bool, string, error) {
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

func (c *Context) FindTool(str string) (*Tool, error) {
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

func (c *Context) getToolDir(tool Tool) string {
	switch tool.Runtime {
	default:
		panic("unknown runtime")
	case RuntimeGo:
		return filepath.Join(c.GetToolsDir(), getGoModDir(tool.Module))
	}
}

func isExists(path string) bool {
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return false
	}

	return true
}

func (c *Context) Sync(ctx context.Context, maxWorkers int, tags []string) error {
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
func (c *Context) Upgrade(ctx context.Context, tags []string) error {
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
func (c *Context) CopySource(ctx context.Context, source string, tags []string) (int, error) {
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

// InitContext will initialize context in specified
func InitContext(dir string) (string, error) {
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

type Tool struct {
	// Name of runtime
	Runtime string `json:"runtime"`
	// Path to module with version
	Module string `json:"module"`
	// Alias create a link in tools. Works like exposing some tools
	Alias optional.Val[string] `json:"alias"`
	Tags  []string             `json:"tags"`
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

// TODO(zhuravlev): migrate from string to object
type Include struct {
	Src  string   `json:"src"`
	Tags []string `json:"tags"`
}

func (i Include) IsSame(include Include) bool {
	return i.Src == include.Src
}

func (i *Include) UnmarshalJSON(bb []byte) error {
	var incStruct struct {
		Src  string   `json:"src"`
		Tags []string `json:"tags"`
	}
	if err := json.Unmarshal(bb, &incStruct); err != nil {
		// NOTE: Migration: probably this is an old version of include. This version is just a string.
		var inc string
		if errStr := json.Unmarshal(bb, &inc); errStr != nil {
			return fmt.Errorf("unmarshal Include: %w", errors.Join(err, errStr))
		}

		i.Src = inc
		i.Tags = []string{}
		return nil
	}

	*i = incStruct

	return nil
}

type Lock struct {
	Tools   Tools        `json:"tools"`
	Remotes []RemoteSpec `json:"remotes"`
}

type RemoteSpec struct {
	Source string   `json:"Source"` // TODO(zhuravlev): make it lowercase, add migration
	Spec   Spec     `json:"Spec"`
	Tags   []string `json:"Tags"`
}

type Tools []Tool

func (tools *Tools) Add(tool Tool) bool {
	for _, t := range *tools {
		if t.IsSame(tool) {
			return false
		}
	}

	*tools = append(*tools, tool)

	return true
}

func (tools *Tools) AddOrUpdateTool(tool Tool) {
	for i, t := range *tools {
		if t.IsSame(tool) {
			(*tools)[i] = tool
			return
		}
	}

	*tools = append(*tools, tool)
}

func (tools *Tools) Filter(tags []string) Tools {
	if len(tags) == 0 {
		return *tools
	}

	res := make(Tools, 0)

	for _, t := range *tools {
		isTarget := slices.ContainsFunc(t.Tags, func(tag string) bool {
			return slices.Contains(tags, tag)
		})
		if !isTarget {
			continue
		}

		res = append(res, t)
	}

	return res
}

type Spec struct {
	Dir      string    `json:"dir"`
	Tools    Tools     `json:"tools"`
	Includes []Include `json:"includes"`
}

func (s *Spec) AddInclude(include Include) bool {
	for _, inc := range s.Includes {
		if inc.IsSame(include) {
			return false
		}
	}

	s.Includes = append(s.Includes, include)
	return true
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

func writeLock(path string, lock Lock) error {
	file, err := os.OpenFile(path, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o644)
	if err != nil {
		return fmt.Errorf("open lock: %w", err)
	}

	enc := json.NewEncoder(file)
	enc.SetIndent("", "\t")

	if err := enc.Encode(lock); err != nil {
		return fmt.Errorf("marshal lock: %w", err)
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

	installedPath := getGoInstalledBinary(baseDir, goBinDir, mod)

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

type SourceUri interface {
	isSourceUri()
}

type SourceUriFile struct {
	Path string
}

func (SourceUriFile) isSourceUri() {}

type SourceUriUrl struct {
	URL string
}

func (SourceUriUrl) isSourceUri() {}

type SourceUriGit struct {
	Addr string
	Path string
}

func (SourceUriGit) isSourceUri() {}

func parseSourceURI(uri string) (SourceUri, error) {
	sourceURL, err := url.Parse(uri)
	if err != nil {
		return nil, fmt.Errorf("parse source uri: %w", err)
	}

	switch sourceURL.Scheme {
	default:
		return nil, fmt.Errorf("unsupported source uri scheme (%s)", sourceURL.Scheme)
	case "":
		// TODO(zhuravlev): make path absolute
		return SourceUriFile{Path: uri}, nil
	case "http", "https":
		return SourceUriUrl{URL: uri}, nil
	case "git+ssh":
		parts := strings.Split(uri, ":")
		pathToFile := parts[len(parts)-1]

		return SourceUriGit{
			Addr: strings.TrimSuffix(strings.TrimPrefix(uri, "git+ssh://"), ":"+pathToFile),
			Path: pathToFile,
		}, nil
	case "git+https":
		parts := strings.Split(uri, ":")
		pathToFile := parts[len(parts)-1]

		return SourceUriGit{
			Addr: strings.TrimSuffix(strings.TrimPrefix(uri, "git+"), ":"+pathToFile),
			Path: pathToFile,
		}, nil
	}
}

func fetchRemoteSpec(ctx context.Context, source string, tags []string) ([]RemoteSpec, error) {
	srcURI, err := parseSourceURI(source)
	if err != nil {
		return nil, fmt.Errorf("parse source uri: %w", err)
	}

	var buf []byte
	switch srcURI := srcURI.(type) {
	default:
		return nil, errors.New("unsupported source uri")
	case SourceUriUrl:
		fmt.Println("Include from url:", srcURI.URL)

		req, err := http.NewRequestWithContext(ctx, http.MethodGet, srcURI.URL, nil)
		if err != nil {
			return nil, fmt.Errorf("create request: %w", err)
		}

		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			return nil, fmt.Errorf("fetch source: %w", err)
		}
		defer resp.Body.Close()

		bb, err := io.ReadAll(resp.Body)
		if err != nil {
			return nil, fmt.Errorf("read response body: %w", err)
		}

		buf = bb
	case SourceUriFile:
		fmt.Println("Include from file:", srcURI.Path)

		bb, err := os.ReadFile(srcURI.Path)
		if err != nil {
			return nil, fmt.Errorf("read file: %w", err)
		}

		buf = bb
	case SourceUriGit:
		fmt.Println("Include from git:", srcURI.Addr, "file:", srcURI.Path)

		targetDir, err := os.MkdirTemp(os.TempDir(), "toolset")
		if err != nil {
			return nil, fmt.Errorf("create temp dir: %w", err)
		}

		args := []string{
			"clone",
			"--depth", "1",
			srcURI.Addr,
			targetDir,
		}

		cmd := exec.CommandContext(ctx, "git", args...)
		cmd.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0")
		cmd.Stdin = nil
		cmd.Stdout = io.Discard
		cmdErr := bytes.NewBufferString("")
		cmd.Stderr = cmdErr
		if err := cmd.Run(); err != nil {
			return nil, fmt.Errorf("clone git repo (%s): %w", strings.TrimSpace(cmdErr.String()), err)
		}

		targetFile := filepath.Join(targetDir, srcURI.Path)
		bb, err := os.ReadFile(targetFile)
		if err != nil {
			return nil, fmt.Errorf("read file: %w", err)
		}

		if err := os.RemoveAll(targetDir); err != nil {
			return nil, fmt.Errorf("remove temp dir: %w", err)
		}

		buf = bb
	}

	var spec Spec
	if err := json.Unmarshal(buf, &spec); err != nil {
		return nil, fmt.Errorf("parse source: %w", err)
	}

	var res []RemoteSpec
	for _, inc := range spec.Includes {
		// FIXME(zhuravlev): add cycle detection
		incSpecs, err := fetchRemoteSpec(ctx, inc.Src, append(slices.Clone(tags), inc.Tags...))
		if err != nil {
			return nil, fmt.Errorf("fetch one of remotes (%s): %w", inc, err)
		}

		res = append(res, incSpecs...)
	}

	return append(res, RemoteSpec{
		Spec:   spec,
		Source: source,
		Tags:   tags,
	}), nil
}
