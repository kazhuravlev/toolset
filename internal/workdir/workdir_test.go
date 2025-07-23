package workdir_test

import (
	"context"
	"os"
	"runtime"
	"testing"

	"github.com/kazhuravlev/toolset/internal/fsh"
	"github.com/kazhuravlev/toolset/internal/workdir"
	"github.com/kazhuravlev/toolset/internal/workdir/structs"
	"github.com/stretchr/testify/require"
)

func TestInit(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("skip for Windows")
	}

	// TODO(zhuravlev): improve tests

	ctx := context.Background()
	const dir = "/dir"

	fs := fsh.NewMemFS(nil)
	require.NoError(t, workdir.Init(fs, dir))

	tree, err := fs.GetTree("/")
	require.NoError(t, err)
	require.Equal(t, []string{
		"/",
		"/dir",
		"/dir/.toolset.json",
		"/dir/.toolset.lock.json",
		"/home-dir",
		"/home-dir/.cache",
		"/home-dir/.cache/toolset",
		"/home-dir/.cache/toolset/stats.json",
	}, tree)

	wd, err := workdir.New(ctx, fs, dir)
	require.NoError(t, err)
	require.NotEmpty(t, wd)

	require.NoError(t, wd.Save())
	require.Equal(t, []string{"go"}, wd.RuntimeList())

	tools, err := wd.GetTools(ctx)
	require.NoError(t, err)
	require.Equal(t, []structs.ToolState{}, tools)

	require.ErrorIs(t, wd.RemoveTool(ctx, "unknown-tool"), workdir.ErrToolNotFoundInSpec)
}

func TestCustomDir(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("skip for Windows")
	}

	ctx := context.Background()
	const dir = "./project"

	fs := fsh.NewMemFS(nil)

	require.NoError(t, os.Setenv(workdir.EnvCacheDir, "/cache"))
	require.NoError(t, os.Setenv(workdir.EnvSpecDir, ".some-local-dir/"))

	require.NoError(t, workdir.Init(fs, dir))

	tree, err := fs.GetTree("/")
	require.NoError(t, err)
	require.Equal(t, []string{
		"/",
		"/cache",
		"/cache/stats.json",
		"/project",
		"/project/.some-local-dir",
		"/project/.some-local-dir/.toolset.json",
		"/project/.some-local-dir/.toolset.lock.json",
	}, tree)

	wd, err := workdir.New(ctx, fs, dir+"/sub-dir/example")
	require.NoError(t, err)
	require.NotEmpty(t, wd)

	{
		tree, err := fs.GetTree("/")
		require.NoError(t, err)
		require.Equal(t, []string{
			"/",
			"/cache",
			"/cache/stats.json",
			"/project",
			"/project/.some-local-dir",
			"/project/.some-local-dir/.toolset.json",
			"/project/.some-local-dir/.toolset.lock.json",
		}, tree)
	}
}
