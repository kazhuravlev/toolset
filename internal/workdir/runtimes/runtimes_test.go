package runtimes_test

import (
	"context"
	"github.com/kazhuravlev/toolset/internal/fsh"
	"github.com/kazhuravlev/toolset/internal/workdir/runtimes"
	"github.com/spf13/afero"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestRuntimes(t *testing.T) {
	ctx := context.Background()
	fs := fsh.NewRealFS()
	tmpDir, err := afero.TempDir(fs, "", "bin-tools")
	require.NoError(t, err)

	rt, err := runtimes.New(fs, tmpDir)
	require.NoError(t, err)
	require.NotNil(t, rt)

	require.Empty(t, rt.List())

	res, err := rt.Get("unknown-runtime")
	require.ErrorIs(t, err, runtimes.ErrNotFound)
	require.Empty(t, res)

	require.NoError(t, rt.Discover(ctx))
	require.Equal(t, []string{"go"}, rt.List())

	res, err = rt.Get("go")
	require.NoError(t, err)
	require.NotEmpty(t, res)

	res, err = rt.GetInstall(ctx, "go")
	require.NoError(t, err)
	require.NotEmpty(t, res)

	res, err = rt.GetInstall(ctx, "go@1.22.10")
	require.NoError(t, err)
	require.NotEmpty(t, res)

	require.Equal(t, []string{"go", "go@1.22.10"}, rt.List())
}
