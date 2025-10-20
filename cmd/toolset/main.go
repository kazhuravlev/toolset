package main

import (
	"errors"
	"fmt"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/kazhuravlev/toolset/internal/timeh"

	"github.com/kazhuravlev/optional"
	"github.com/kazhuravlev/toolset/internal/fsh"

	"github.com/jedib0t/go-pretty/v6/table"
	"github.com/kazhuravlev/toolset/internal/workdir"
	"github.com/kazhuravlev/toolset/internal/workdir/structs"
	cli "github.com/urfave/cli/v2"
)

const (
	keyParallel = "parallel"
	keyCopyFrom = "copy-from"
	keyInclude  = "include"
	keyTags     = "tags"
	keyUnused   = "unused"
)

var version = "unknown-dirty"

var flagParallel = &cli.IntFlag{
	Name:    keyParallel,
	Aliases: []string{"p"},
	Usage:   "Max parallel workers",
	Value:   4,
}

func main() {
	app := &cli.App{
		Name:  "toolset",
		Usage: "Manage local toolsets",
		Commands: []*cli.Command{
			{
				Name:  "version",
				Usage: "show toolset version",
				Description: `Display the current version of toolset.

	$ toolset version`,
				Action: func(c *cli.Context) error {
					fmt.Println("version:", version)
					return nil
				},
			},
			{
				Name:  "init",
				Usage: "init toolset in specified directory",
				Description: `Initialize a new toolset configuration in the specified directory.
Creates .toolset.json and .toolset.lock.json files to start managing tools.

	$ toolset init
	$ toolset init ./myproject

Optionally copy tools from an existing toolset:

	$ toolset init --copy-from=../other-project/.toolset.json`,
				Flags: []cli.Flag{
					&cli.StringFlag{
						Name:     keyCopyFrom,
						Usage:    "specify addr to source file that will be copied into new config",
						Required: false,
					},
				},
				Action: cmdInit,
				Args:   true,
			},
			{
				Name:  "add",
				Usage: "add tool to .toolset.json",
				Description: `Add tools to local configuration to fix the using version.

	$ toolset add <RUNTIME> <TOOL>
	$ toolset add go 				github.com/golangci/golangci-lint/cmd/golangci-lint@v1.61.0

At this point tool will not be installed. In order to install added tool please run

	$ toolset sync`,
				Action: withWorkdir(cmdAdd),
				Flags: []cli.Flag{
					&cli.StringFlag{
						Name:     keyCopyFrom,
						Usage:    "specify addr to source file that will be copied into current config",
						Required: false,
					},
					&cli.StringFlag{
						Name:     keyInclude,
						Usage:    "specify addr to source file that will be included into current config",
						Required: false,
					},
					&cli.StringSliceFlag{
						Name:     keyTags,
						Usage:    "add one or more tags to this tool",
						Required: false,
					},
				},
				Args: true,
			},
			{
				Name:  "sync",
				Usage: "install all required tools from toolset file",
				Description: `Install or update all tools defined in .toolset.json to match the configuration.
This command downloads, builds, and installs tools that are missing or outdated.
Updates .toolset.lock.json with installed versions.

	$ toolset sync
	$ toolset sync --parallel=8
	$ toolset sync --tags=linters,formatters

When cache is configured, sync automatically downloads from cache and uploads new builds.`,
				Action: withWorkdir(cmdSync),
				Flags: []cli.Flag{
					flagParallel,
					&cli.StringSliceFlag{
						Name:     keyTags,
						Usage:    "filter tools by tags",
						Required: false,
					},
				},
			},
			{
				Name:  "run",
				Usage: "run installed tool by its name",
				Description: `Execute an installed tool with the specified arguments.
The tool must be added and synced before running.

	$ toolset run golangci-lint --version
	$ toolset run gofumpt -l -w .
	$ toolset run <tool-name> [args...]

If tool is not added, run 'toolset add'. If not installed, run 'toolset sync'.`,
				Action: withWorkdir(cmdRun),
				Args:   true,
			},
			{
				Name:  "upgrade",
				Usage: "upgrade deps to the latest versions",
				Description: `Upgrade tools to their latest available versions and sync them.
Updates .toolset.json with new versions and installs them.

	$ toolset upgrade
	$ toolset upgrade golangci-lint
	$ toolset upgrade --tags=linters
	$ toolset upgrade --parallel=8

Upgrades all tools by default. Specify a module name or use --tags to filter.`,
				Action: withWorkdir(cmdUpgrade),
				Flags: []cli.Flag{
					flagParallel,
					&cli.StringSliceFlag{
						Name:     keyTags,
						Usage:    "filter tools by tags",
						Required: false,
					},
				},
				Args: true,
			},
			{
				Name:  "ensure",
				Usage: "ensure concrete version is exists. work like upsert semantic",
				Description: `Ensure a specific tool version exists in the configuration.
Works like upsert: adds the tool if missing, or updates if present with different version.

	$ toolset ensure go github.com/golangci/golangci-lint/cmd/golangci-lint@v1.61.0
	$ toolset ensure go github.com/golangci/golangci-lint/cmd/golangci-lint@v1.61.0 golangci
	$ toolset ensure go <module@version> [alias] --tags=linters

This does NOT install the tool. Run 'toolset sync' afterward to install.`,
				Action: withWorkdir(cmdEnsureModuleVersion),
				Flags: []cli.Flag{
					&cli.StringSliceFlag{
						Name:     keyTags,
						Usage:    "filter tools by tags",
						Required: false,
					},
				},
				Args: true,
			},
			{
				Name:  "list",
				Usage: "list of project tools and their stats",
				Description: `Display a table of all tools with runtime, version, installation status, and usage statistics.
Shows tool metadata including aliases, tags, and last usage time.

	$ toolset list
	$ toolset list --unused

Use --unused flag to find tools that have never been executed (helpful for cleanup).`,
				Flags: []cli.Flag{
					&cli.BoolFlag{
						Name:  keyUnused,
						Usage: "list tools that are unused. Useful when you want to check which tools can be deleted (or excluded from tag)",
						Value: false,
					},
				},
				Action: withWorkdir(cmdList),
			},
			{
				Name:  "which",
				Usage: "show path to the actual binary",
				Description: `Display the full filesystem path to the installed tool binary.
Useful for debugging or integrating with other tools.

	$ toolset which golangci-lint
	$ toolset which gofumpt goimports

Can query multiple tools at once.`,
				Action: withWorkdir(cmdWhich),
				Args:   true,
			},
			{
				Name:  "remove",
				Usage: "remove tool",
				Description: `Remove one or more tools from the toolset configuration.
Removes from .toolset.json but does not delete the installed binary.

	$ toolset remove golangci-lint
	$ toolset remove gofumpt goimports

Can remove multiple tools in a single command.`,
				Action: withWorkdir(cmdRemove),
				Args:   true,
			},
			{
				Name:   "info",
				Usage:  "show information and stats",
				Action: withWorkdir(cmdInfo),
				Args:   false,
			},
			{
				Name:   "clear-cache",
				Usage:  "clear all cache dir and stats",
				Action: withWorkdir(cmdClearCache),
				Args:   false,
			},
			{
				Name:  "runtime",
				Usage: "manage runtimes",
				Subcommands: []*cli.Command{
					{
						Name:  "add",
						Usage: "add new",
						Description: `Install runtime in local project dir.

$ toolset runtime add go@1.22`,
						Action: withWorkdir(cmdRuntimeAdd),
						Args:   true,
					},
					{
						Name:   "list",
						Usage:  "list all runtimes",
						Action: withWorkdir(cmdRuntimeList),
					},
				},
			},
		},
	}

	if err := app.Run(os.Args); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}

func cmdInit(c *cli.Context) error {
	ctx := c.Context

	fs := fsh.NewRealFS()

	targetDir := c.Args().First()
	if targetDir == "" {
		targetDir = "."
	}

	if err := workdir.Init(ctx, fs, targetDir); err != nil {
		return fmt.Errorf("init workdir: %w", err)
	}

	fmt.Println("Spec created")

	if val := c.String(keyCopyFrom); val != "" {
		wd, err := workdir.New(ctx, fs, targetDir)
		if err != nil {
			return fmt.Errorf("new workdir: %w", err)
		}

		count, err := wd.CopySource(ctx, val, nil)
		if err != nil {
			return fmt.Errorf("copy: %w", err)
		}

		if err := wd.Save(ctx); err != nil {
			return fmt.Errorf("save workdir: %w", err)
		}

		fmt.Println("Copied tools:", count)
	}

	return nil
}

func withWorkdir(fn func(c *cli.Context, wd *workdir.Workdir) error) func(c *cli.Context) error {
	return func(c *cli.Context) error {
		fs := fsh.NewRealFS()

		wd, err := workdir.New(c.Context, fs, "./")
		if err != nil {
			return fmt.Errorf("new workdir: %w", err)
		}

		return fn(c, wd)
	}
}

func cmdAdd(c *cli.Context, wd *workdir.Workdir) error {
	ctx := c.Context

	tags := c.StringSlice(keyTags)

	if val := c.String(keyCopyFrom); val != "" {
		count, err := wd.CopySource(ctx, val, tags)
		if err != nil {
			return fmt.Errorf("copy: %w", err)
		}

		if err := wd.Save(ctx); err != nil {
			return fmt.Errorf("save workdir: %w", err)
		}

		fmt.Println("Copied tools:", count)

		return nil
	}

	if val := c.String(keyInclude); val != "" {
		count, err := wd.AddInclude(ctx, val, tags)
		if err != nil {
			return fmt.Errorf("include: %w", err)
		}

		if err := wd.Save(ctx); err != nil {
			return fmt.Errorf("save workdir: %w", err)
		}

		fmt.Println("Included tools:", count)

		return nil
	}

	runtime := c.Args().First()
	module := c.Args().Get(1)

	var alias optional.Val[string]
	if c.Args().Len() == 3 {
		aliasStr := c.Args().Get(2)
		alias.Set(aliasStr)
	}

	wasAdded, mod, err := wd.Add(ctx, runtime, module, alias, tags)
	if err != nil {
		return fmt.Errorf("add module: %w", err)
	}

	if err := wd.Save(ctx); err != nil {
		return fmt.Errorf("save context: %w", err)
	}

	if !wasAdded {
		fmt.Printf("tool already exists in toolset: %s\n", mod)
	} else {
		fmt.Printf("tool added to toolset: %s\n", mod)
	}

	return nil
}

func cmdRuntimeAdd(c *cli.Context, wd *workdir.Workdir) error {
	ctx := c.Context

	runtime := c.Args().First()

	if err := wd.RuntimeAdd(ctx, runtime); err != nil {
		return fmt.Errorf("add runtime: %w", err)
	}

	if err := wd.Save(ctx); err != nil {
		return fmt.Errorf("save context: %w", err)
	}

	return nil
}

func cmdRuntimeList(_ *cli.Context, wd *workdir.Workdir) error {
	t := table.NewWriter()
	t.AppendHeader(table.Row{
		"Runtime",
	})

	list := wd.RuntimeList()

	rows := make([]table.Row, 0, len(list))
	for _, name := range list {
		rows = append(rows, table.Row{name})
	}

	t.AppendRows(rows)

	res := t.Render()
	fmt.Println(res)

	return nil
}

func cmdRun(c *cli.Context, wd *workdir.Workdir) error {
	ctx := c.Context

	target := c.Args().First()
	if target == "" {
		return fmt.Errorf("target is required")
	}

	if err := wd.RunTool(ctx, target, c.Args().Tail()...); err != nil {
		if errors.Is(err, workdir.ErrToolNotFoundInSpec) {
			fmt.Println("tool not added. Run `toolset add --help` to add this tool")
			os.Exit(1)
			return nil
		}

		if errors.Is(err, workdir.ErrToolNotInstalled) {
			fmt.Println("tool not installed. Run `toolset sync --help` to install tool before run")
			os.Exit(1)
			return nil
		}

		var errRun structs.RunError
		if errors.As(err, &errRun) {
			os.Exit(errRun.ExitCode)
			return nil
		}

		return fmt.Errorf("run tool: %w", err)
	}

	return nil
}

func cmdSync(c *cli.Context, wd *workdir.Workdir) error {
	ctx := c.Context

	maxWorkers := c.Int(keyParallel)
	tags := c.StringSlice(keyTags)

	if err := wd.Sync(ctx, maxWorkers, tags); err != nil {
		return fmt.Errorf("sync: %w", err)
	}

	if err := wd.Save(ctx); err != nil {
		return fmt.Errorf("save: %w", err)
	}

	return nil
}

func cmdUpgrade(c *cli.Context, wd *workdir.Workdir) error {
	ctx := c.Context

	maxWorkers := c.Int(keyParallel)
	tags := c.StringSlice(keyTags)
	module := c.Args().First()

	if module != "" && len(tags) != 0 {
		return fmt.Errorf("can't use both module and tags")
	}

	filter := func(tool structs.Tool) bool { return true }
	if module != "" {
		fmt.Println("upgrade module:", module)
		filter = func(tool structs.Tool) bool { return tool.ModuleName() == module }
	}

	if len(tags) != 0 {
		fmt.Println("upgrade modules by tags:", tags)
		filter = func(tool structs.Tool) bool {
			for _, tag := range tool.Tags {
				for _, tag2 := range tags {
					if tag == tag2 {
						return true
					}
				}
			}

			return false
		}
	}

	if err := wd.Upgrade(ctx, filter); err != nil {
		return fmt.Errorf("upgrade: %w", err)
	}

	if err := wd.Save(ctx); err != nil {
		return fmt.Errorf("save context: %w", err)
	}

	if err := wd.Sync(ctx, maxWorkers, tags); err != nil {
		return fmt.Errorf("sync: %w", err)
	}

	if err := wd.Save(ctx); err != nil {
		return fmt.Errorf("save context: %w", err)
	}

	return nil
}

func cmdList(c *cli.Context, wd *workdir.Workdir) error {
	ctx := c.Context

	onlyUnused := c.Bool(keyUnused)

	tools, err := wd.GetTools(ctx)
	if err != nil {
		return fmt.Errorf("get tools: %w", err)
	}

	if onlyUnused {
		tools2 := make([]structs.ToolState, 0, len(tools))
		for _, tool := range tools {
			if tool.LastUse.HasVal() {
				continue
			}

			tools2 = append(tools2, tool)
		}

		tools = tools2
	}

	sort.SliceStable(tools, func(i, j int) bool {
		return tools[i].Tool.ModuleName() < tools[j].Tool.ModuleName()
	})

	rows := make([]table.Row, 0, len(tools))
	for _, ts := range tools {
		lastUse := "---"
		if val, ok := ts.LastUse.Get(); ok {
			lastUse = timeh.Duration(time.Since(val))
		}

		rows = append(rows, table.Row{
			ts.Tool.Runtime,
			ts.Module.Name,
			ts.Module.Mod.Version(),
			ts.Module.IsInstalled,
			lastUse,
			ts.Module.IsPrivate,
			ts.Tool.Alias.ValDefault("---"),
			strings.Join(ts.Tool.Tags, ","),
			ts.Tool.Module,
		})
	}

	t := table.NewWriter()
	t.AppendHeader(table.Row{
		"Runtime",
		"Name",
		"Version",
		"Installed",
		"Last Usage",
		"Private",
		"Alias",
		"Tags",
		"Module",
	})

	t.AppendRows(rows)

	res := t.Render()
	fmt.Println(res)

	return nil
}

func cmdWhich(c *cli.Context, wd *workdir.Workdir) error {
	targets := c.Args().Slice()
	if len(targets) == 0 {
		return fmt.Errorf("target is required")
	}

	for _, target := range targets {
		ts, err := wd.FindTool(target)
		if err != nil {
			if errors.Is(err, workdir.ErrToolNotFoundInSpec) {
				fmt.Println("tool not added. Run `toolset add --help` to add tool")
				os.Exit(1)
				return nil
			}

			if errors.Is(err, workdir.ErrToolNotInstalled) {
				fmt.Println("tool not installed. Run `toolset sync --help` to install tool before run")
				os.Exit(1)
				return nil
			}

			return fmt.Errorf("find tool: %w", err)
		}

		fmt.Println(ts.Module.BinPath)
	}

	return nil
}

func cmdRemove(c *cli.Context, wd *workdir.Workdir) error {
	ctx := c.Context

	targets := c.Args().Slice()
	if len(targets) == 0 {
		return fmt.Errorf("target is required")
	}

	for _, target := range targets {
		if err := wd.RemoveTool(ctx, target); err != nil {
			return fmt.Errorf("remove tool (%s): %w", target, err)
		}
	}

	if err := wd.Save(ctx); err != nil {
		return fmt.Errorf("save: %w", err)
	}

	return nil
}

func cmdInfo(_ *cli.Context, wd *workdir.Workdir) error {
	info, err := wd.GetSystemInfo()
	if err != nil {
		return fmt.Errorf("get system info: %w", err)
	}

	t := table.NewWriter()
	t.AppendHeader(table.Row{
		"Property",
		"Value",
	})

	rows := []table.Row{
		{"Version:", version},
		{"Cache Size:", humanizeBytes(info.Storage.TotalBytes)},
		{"Cache dir:", info.Locations.CacheDir},
		{"Toolset File:", info.Locations.ToolsetFile},
		{"Toolset Lock File:", info.Locations.ToolsetLockFile},
		{"Project Root Dir:", info.Locations.ProjectRootDir},
		{"Current Dir:", info.Locations.CurrentDir},
		{"Stats File:", info.Locations.StatsFile},
	}

	for _, env := range info.Envs {
		rows = append(rows, table.Row{"ENV:" + env[0], env[1]})
	}

	t.AppendRows(rows)

	res := t.Render()
	fmt.Println(res)

	return nil
}

func cmdClearCache(_ *cli.Context, wd *workdir.Workdir) error {
	info, err := wd.GetSystemInfo()
	if err != nil {
		return fmt.Errorf("get system info: %w", err)
	}

	fmt.Println("Removing cache dir:", info.Locations.CacheDir)

	if err := os.RemoveAll(info.Locations.CacheDir); err != nil {
		return fmt.Errorf("remove cache dir: %w", err)
	}

	fmt.Println("Done!")

	return nil
}

func cmdEnsureModuleVersion(c *cli.Context, wd *workdir.Workdir) error {
	ctx := c.Context

	tags := c.StringSlice(keyTags)
	runtime := c.Args().First()
	module := c.Args().Get(1)

	if runtime == "" || module == "" {
		return fmt.Errorf("runtime and module is required")
	}

	var alias optional.Val[string]
	if c.Args().Len() == 3 {
		aliasStr := c.Args().Get(2)
		alias.Set(aliasStr)
	}

	mod, err := wd.Ensure(ctx, runtime, module, alias, tags)
	if err != nil {
		return fmt.Errorf("ensure module: %w", err)
	}

	fmt.Println("Module added:", runtime, mod)

	if err := wd.Save(ctx); err != nil {
		return fmt.Errorf("save workdir: %w", err)
	}

	return nil
}

func humanizeBytes(bytes int64) string {
	const unit = 1024
	if bytes < unit {
		return fmt.Sprintf("%d B", bytes)
	}
	div, exp := int64(unit), 0
	for n := bytes / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(bytes)/float64(div), "KMGTPE"[exp])
}
