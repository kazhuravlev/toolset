package main

import (
	"errors"
	"fmt"
	"os"
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
				Name: "version",
				Action: func(c *cli.Context) error {
					fmt.Println("version:", version)
					return nil
				},
			},
			{
				Name:  "init",
				Usage: "init toolset in specified directory",
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
				Name:   "sync",
				Usage:  "install all required tools from toolset file",
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
				Name:   "run",
				Usage:  "run installed tool by its name",
				Action: withWorkdir(cmdRun),
				Args:   true,
			},
			{
				Name:   "upgrade",
				Usage:  "upgrade deps to the latest versions",
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
				Name:  "list",
				Usage: "list of project tools and their stats",
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
				Name:   "which",
				Usage:  "show path to the actual binary",
				Action: withWorkdir(cmdWhich),
				Args:   true,
			},
			{
				Name:   "remove",
				Usage:  "remove tool",
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
	fs := fsh.NewRealFS()

	targetDir := c.Args().First()
	if targetDir == "" {
		targetDir = "."
	}

	if err := workdir.Init(fs, targetDir); err != nil {
		return fmt.Errorf("init workdir: %w", err)
	}

	fmt.Println("Spec created")

	if val := c.String(keyCopyFrom); val != "" {
		wd, err := workdir.New(c.Context, fs, targetDir)
		if err != nil {
			return fmt.Errorf("new workdir: %w", err)
		}

		count, err := wd.CopySource(c.Context, val, nil)
		if err != nil {
			return fmt.Errorf("copy: %w", err)
		}

		if err := wd.Save(); err != nil {
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
	tags := c.StringSlice(keyTags)

	if val := c.String(keyCopyFrom); val != "" {
		count, err := wd.CopySource(c.Context, val, tags)
		if err != nil {
			return fmt.Errorf("copy: %w", err)
		}

		if err := wd.Save(); err != nil {
			return fmt.Errorf("save workdir: %w", err)
		}

		fmt.Println("Copied tools:", count)

		return nil
	}

	if val := c.String(keyInclude); val != "" {
		count, err := wd.AddInclude(c.Context, val, tags)
		if err != nil {
			return fmt.Errorf("include: %w", err)
		}

		if err := wd.Save(); err != nil {
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

	wasAdded, mod, err := wd.Add(c.Context, runtime, module, alias, tags)
	if err != nil {
		return fmt.Errorf("add module: %w", err)
	}

	if err := wd.Save(); err != nil {
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
	runtime := c.Args().First()

	if err := wd.RuntimeAdd(c.Context, runtime); err != nil {
		return fmt.Errorf("add runtime: %w", err)
	}

	if err := wd.Save(); err != nil {
		return fmt.Errorf("save context: %w", err)
	}

	return nil
}

func cmdRuntimeList(c *cli.Context, wd *workdir.Workdir) error {
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
	target := c.Args().First()
	if target == "" {
		return fmt.Errorf("target is required")
	}

	if err := wd.RunTool(c.Context, target, c.Args().Tail()...); err != nil {
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

	if err := wd.Save(); err != nil {
		return fmt.Errorf("save: %w", err)
	}

	return nil
}

func cmdUpgrade(c *cli.Context, wd *workdir.Workdir) error {
	ctx := c.Context

	maxWorkers := c.Int(keyParallel)
	tags := c.StringSlice(keyTags)

	if err := wd.Upgrade(c.Context, tags); err != nil {
		return fmt.Errorf("upgrade: %w", err)
	}

	if err := wd.Save(); err != nil {
		return fmt.Errorf("save context: %w", err)
	}

	if err := wd.Sync(ctx, maxWorkers, tags); err != nil {
		return fmt.Errorf("sync: %w", err)
	}

	if err := wd.Save(); err != nil {
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
	targets := c.Args().Slice()
	if len(targets) == 0 {
		return fmt.Errorf("target is required")
	}

	for _, target := range targets {
		if err := wd.RemoveTool(c.Context, target); err != nil {
			return fmt.Errorf("remove tool (%s): %w", target, err)
		}
	}

	if err := wd.Save(); err != nil {
		return fmt.Errorf("save: %w", err)
	}

	return nil
}

func cmdInfo(c *cli.Context, wd *workdir.Workdir) error {
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
		{"Cache Dir:", info.Locations.CacheDir},
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
