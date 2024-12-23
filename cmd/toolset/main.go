package main

import (
	"errors"
	"fmt"
	"github.com/jedib0t/go-pretty/v6/table"
	"github.com/kazhuravlev/optional"
	"github.com/kazhuravlev/toolset/internal/workdir"
	"github.com/kazhuravlev/toolset/internal/workdir/structs"
	cli "github.com/urfave/cli/v2"
	"os"
	"strings"
	"time"
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
				Name:   "init",
				Usage:  "init toolset in specified directory",
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
				Action: cmdAdd,
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
				Action: cmdSync,
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
				Action: cmdRun,
				Args:   true,
			},
			{
				Name:   "upgrade",
				Usage:  "upgrade deps to the latest versions",
				Action: cmdUpgrade,
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
				Action: cmdList,
			},
			{
				Name:   "which",
				Usage:  "show path to the actual binary",
				Action: cmdWhich,
				Args:   true,
			},
		},
	}

	if err := app.Run(os.Args); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}

func cmdInit(c *cli.Context) error {
	targetDir := c.Args().First()
	if targetDir == "" {
		targetDir = "."
	}

	absSpecName, err := workdir.Init(targetDir)
	if err != nil {
		return fmt.Errorf("init workdir: %w", err)
	}

	fmt.Println("Spec created:", absSpecName)

	return nil
}

func cmdAdd(c *cli.Context) error {
	tags := c.StringSlice(keyTags)

	wd, err := workdir.New()
	if err != nil {
		return fmt.Errorf("new workdir: %w", err)
	}

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

	wasAdded, mod, err := wd.Add(c.Context, runtime, module, optional.Empty[string](), tags)
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

func cmdRun(c *cli.Context) error {
	target := c.Args().First()
	if target == "" {
		return fmt.Errorf("target is required")
	}

	wd, err := workdir.New()
	if err != nil {
		return fmt.Errorf("new workdir: %w", err)
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

		var errRun *structs.RunError
		if errors.As(err, &errRun) {
			os.Exit(errRun.ExitCode)
			return nil
		}

		return fmt.Errorf("run tool: %w", err)
	}

	return nil
}

func cmdSync(c *cli.Context) error {
	ctx := c.Context

	maxWorkers := c.Int(keyParallel)
	tags := c.StringSlice(keyTags)

	wd, err := workdir.New()
	if err != nil {
		return fmt.Errorf("new workdir: %w", err)
	}

	if err := wd.Sync(ctx, maxWorkers, tags); err != nil {
		return fmt.Errorf("sync: %w", err)
	}

	if err := wd.Save(); err != nil {
		return fmt.Errorf("save: %w", err)
	}

	return nil
}

func cmdUpgrade(c *cli.Context) error {
	ctx := c.Context

	maxWorkers := c.Int(keyParallel)
	tags := c.StringSlice(keyTags)

	wd, err := workdir.New()
	if err != nil {
		return fmt.Errorf("new workdir: %w", err)
	}

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

func cmdList(c *cli.Context) error {
	ctx := c.Context

	onlyUnused := c.Bool(keyUnused)

	wd, err := workdir.New()
	if err != nil {
		return fmt.Errorf("new workdir: %w", err)
	}

	tools, err := wd.GetTools(ctx)
	if err != nil {
		return fmt.Errorf("get tools: %w", err)
	}

	if onlyUnused {
		tools2 := make([]workdir.ToolState, 0, len(tools))
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
			lastUse = duration(time.Since(val))
		}

		rows = append(rows, table.Row{
			ts.Tool.Runtime,
			ts.Module.Name,
			ts.Module.Version,
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

func cmdWhich(c *cli.Context) error {
	targets := c.Args().Slice()
	if len(targets) == 0 {
		return fmt.Errorf("target is required")
	}

	wd, err := workdir.New()
	if err != nil {
		return fmt.Errorf("new workdir: %w", err)
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

func duration(d time.Duration) string {
	if d == 0 {
		return "0s"
	}

	days := d / (24 * time.Hour)
	d -= days * 24 * time.Hour

	hours := d / time.Hour
	d -= hours * time.Hour

	minutes := d / time.Minute
	d -= minutes * time.Minute

	seconds := d / time.Second

	// Build the human-readable string
	var result string
	if days > 0 {
		result += fmt.Sprintf("%dd ", days)
	}

	if hours > 0 {
		result += fmt.Sprintf("%dh ", hours)
	}

	if minutes > 0 {
		result += fmt.Sprintf("%dm ", minutes)
	}

	if seconds > 0 {
		result += fmt.Sprintf("%ds ", seconds)
	}

	return result
}
