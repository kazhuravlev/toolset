package prog_test

import (
	"testing"

	"github.com/kazhuravlev/toolset/internal/prog"

	"github.com/stretchr/testify/require"
)

func TestLatest(t *testing.T) {
	mod := prog.NewLatest("mod1")
	require.Equal(t, "mod1", mod.Name())
	require.Equal(t, "latest", mod.Version())
	require.Equal(t, "mod1@latest", mod.S())
	require.Equal(t, true, mod.IsLatest())
	require.Equal(t, mod, mod.AsLatest())
}

func TestNotLatest(t *testing.T) {
	mod := prog.NewVer("mod1", "ver1")
	require.Equal(t, "mod1", mod.Name())
	require.Equal(t, "ver1", mod.Version())
	require.Equal(t, "mod1@ver1", mod.S())
	require.Equal(t, false, mod.IsLatest())
	require.Equal(t, "mod1@latest", mod.AsLatest().S())
}

func TestEmpty(t *testing.T) {
	require.Panics(t, func() {
		prog.NewVer("mod1", "")
	})
}
