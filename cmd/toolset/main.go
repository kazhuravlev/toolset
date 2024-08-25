package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/kazhuravlev/optional"
	cli "github.com/urfave/cli/v2"
)

const (
	RuntimeGo    = "go"
	specFilename = ".toolset.json"
)

var version string

type Tool struct {
	Runtime string               `json:"runtime"`
	Module  string               `json:"module"`
	Alias   optional.Val[string] `json:"alias"`
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

func main() {
	app := &cli.App{
		Name:  "toolset",
		Usage: "Manage local toolsets",
		Commands: []*cli.Command{
			{
				Name:   "sync",
				Action: cmdSync,
			},
			{
				Name:   "add",
				Usage:  "add tool",
				Action: cmdAdd,
				Args:   true,
			},
			{
				Name:   "run",
				Usage:  "toolset run golangci-lint",
				Action: cmdRun,
				Args:   true,
			},
		},
	}

	if err := app.Run(os.Args); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
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

func getGoLatestVersion(link string) (string, string, error) {
	module, err := getGoModuleName(link)
	if err != nil {
		return "", "", fmt.Errorf("get go module name: %w", err)
	}

	// TODO: use a proxy from env
	// Get the latest version
	resp, err := http.Get(fmt.Sprintf("https://proxy.golang.org/%s/@latest", module))
	if err != nil {
		return "", "", fmt.Errorf("get go module: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", "", fmt.Errorf("unable to get module: %s", resp.Status)
	}

	var mod GoModule
	if err := json.NewDecoder(resp.Body).Decode(&mod); err != nil {
		return "", "", fmt.Errorf("unable to decode module: %w", err)
	}

	return module, mod.Version, nil
}

func cmdAdd(c *cli.Context) error {
	runtime := c.Args().First()
	if runtime != RuntimeGo {
		return fmt.Errorf("unsupported runtime: %s", runtime)
	}

	goBinary := c.Args().Get(1)
	if goBinary == "" {
		return fmt.Errorf("no module name provided")
	}

	spec, err := readSpec(specFilename)
	if err != nil {
		spec = &Spec{
			Dir:   "./bin/tools",
			Tools: nil,
		}
	}

	goBinaryWoVersion := strings.Split(goBinary, "@")[0]

	if strings.Contains(goBinary, "@latest") || !strings.Contains(goBinary, "@") {
		_, latestVersion, err := getGoLatestVersion(goBinary)
		if err != nil {
			return fmt.Errorf("get go module version: %w", err)
		}

		goBinary = fmt.Sprintf("%s@%s", goBinaryWoVersion, latestVersion)
	}

	wasAdded := spec.AddTool(Tool{
		Runtime: RuntimeGo,
		Module:  goBinary,
		Alias:   optional.Val[string]{},
	})

	if err := writeSpec(specFilename, *spec); err != nil {
		return fmt.Errorf("write spec: %w", err)
	}

	if !wasAdded {
		fmt.Printf("tool already exists in toolset: %s\n", goBinaryWoVersion)
	} else {
		fmt.Printf("tool added to toolset: %s\n", goBinaryWoVersion)
	}

	return nil
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

func readSpec(path string) (*Spec, error) {
	bb, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read spec (%s): %w", path, err)
	}

	var spec Spec
	if err := json.Unmarshal(bb, &spec); err != nil {
		return nil, fmt.Errorf("unmarshal spec (%s): %w", path, err)
	}

	return &spec, nil
}

func cmdSync(*cli.Context) error {
	spec, err := readSpec(specFilename)
	if err != nil {
		return fmt.Errorf("read spec (%s): %w", specFilename, err)
	}

	absTargetDir, err := filepath.Abs(spec.Dir)
	if err != nil {
		return fmt.Errorf("get abs path: %w", err)
	}

	if _, err := os.Stat(absTargetDir); os.IsNotExist(err) {
		fmt.Println("Target dir not exists. Creating...")
		if err := os.MkdirAll(absTargetDir, 0o755); err != nil {
			return fmt.Errorf("create target dir (%s): %w", absTargetDir, err)
		}
	}

	fmt.Println("Target dir:", absTargetDir)

	// TODO: remove all unknown aliases

	for _, tool := range spec.Tools {
		fmt.Println("Sync:", tool.Runtime, tool.Module, tool.Alias.ValDefault(""))
		if tool.Runtime != RuntimeGo {
			return fmt.Errorf("unsupported runtime (%s) for tool (%s)", tool.Runtime, tool.Module)
		}

		if !strings.Contains(tool.Module, "@") {
			return fmt.Errorf("go tool (%s) must have a version, at least `latest`", tool.Module)
		}

		if err := goInstall(tool.Module, absTargetDir, tool.Alias); err != nil {
			return fmt.Errorf("install tool (%s): %w", tool.Module, err)
		}
	}

	return nil
}

func cmdRun(c *cli.Context) error {
	target := c.Args().First()
	if target == "" {
		return fmt.Errorf("target is required")
	}

	spec, err := readSpec(specFilename)
	if err != nil {
		return fmt.Errorf("read spec (%s): %w", specFilename, err)
	}

	for _, tool := range spec.Tools {
		if tool.Runtime != RuntimeGo {
			return fmt.Errorf("unsupported runtime (%s) for tool (%s)", tool.Runtime, tool.Module)
		}

		if !strings.Contains(tool.Module, "@") {
			return fmt.Errorf("go tool (%s) must have a version, at least `latest`", tool.Module)
		}

		binName := getGoBinFromMod(tool.Module)
		if binName == target {
			cmd := exec.CommandContext(c.Context, getGoInstalledBinary(spec.Dir, tool.Module), c.Args().Tail()...)
			cmd.Stdin = os.Stdin
			cmd.Stdout = os.Stdout
			cmd.Stderr = os.Stderr
			if err := cmd.Run(); err != nil {
				return fmt.Errorf("run tool: %w", err)
			}

			return nil
		}
	}

	return fmt.Errorf("target (%s) not found", target)
}

func getGoInstalledBinary(goBinDir, mod string) string {
	modDir := filepath.Join(goBinDir, getGoModDir(mod))
	return filepath.Join(modDir, getGoBinFromMod(mod))
}

func goInstall(mod, goBinDir string, alias optional.Val[string]) error {
	const golang = "go"

	modDir := filepath.Join(goBinDir, getGoModDir(mod))
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

	installedPath := getGoInstalledBinary(goBinDir, mod)

	if alias, ok := alias.Get(); ok {
		targetPath := filepath.Join(goBinDir, alias)
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

// getGoBinFromMod returns a binary name that installed by `go install`
// github.com/golangci/golangci-lint/cmd/golangci-lint@v1.55.2 ==> golangci-lint
func getGoBinFromMod(mod string) string {
	const at = "@"

	if strings.Contains(mod, at) {
		mod = strings.Split(mod, "@")[0]
	}

	return filepath.Base(mod)
}

// getGoModDir returns a dir that will keep all mod-related stuff for specific version.
func getGoModDir(mod string) string {
	parts := strings.Split(mod, "@")
	binName := filepath.Base(parts[0])
	version := parts[1]

	return fmt.Sprintf(".%s___%s", binName, version)
}
