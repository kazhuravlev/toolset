package runtimes_test

import (
	"context"
	"github.com/kazhuravlev/toolset/internal/fsh"
	"github.com/kazhuravlev/toolset/internal/workdir/runtimes"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestRuntimes_Discover(t *testing.T) {
	ctx := context.Background()
	fs := fsh.NewMemFS(nil)
	rt, err := runtimes.New(fs, "/tmp/tools")
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
}
