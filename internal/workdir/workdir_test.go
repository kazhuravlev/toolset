package workdir_test

import (
	"testing"

	"github.com/kazhuravlev/toolset/internal/fsh"
	"github.com/kazhuravlev/toolset/internal/workdir"
	"github.com/stretchr/testify/require"
)

func TestInit(t *testing.T) {
	fs := fsh.NewMemFS(nil)
	const dir = "/dir"
	require.NoError(t, workdir.Init(fs, dir))

	tree, err := fs.GetTree(dir)
	require.NoError(t, err)
	require.Equal(t, []string{
		"/dir",
		"/dir/.toolset.json",
		"/dir/.toolset.lock.json",
		"/dir/bin",
		"/dir/bin/tools",
		"/dir/bin/tools/.stats.json",
	}, tree)
}
