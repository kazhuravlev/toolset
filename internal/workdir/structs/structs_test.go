package structs_test

import (
	"testing"

	"github.com/kazhuravlev/optional"

	"github.com/kazhuravlev/toolset/internal/workdir/structs"
	"github.com/stretchr/testify/require"
)

func TestRunError(t *testing.T) {
	t.Run("error_string", func(t *testing.T) {
		require.Equal(t, "0", structs.RunError{ExitCode: 0}.Error())
		require.Equal(t, "777", structs.RunError{ExitCode: 777}.Error())
	})

	t.Run("equality", func(t *testing.T) {
		re1 := error(structs.RunError{ExitCode: 111})
		re2 := error(structs.RunError{ExitCode: 111})

		require.ErrorIs(t, re1, re2)
	})
}

func TestTool(t *testing.T) {
	t.Run("id_depends_only_on_runtime_and_module", func(t *testing.T) {
		t1 := Tool("go", "some-mod", optional.Empty[string](), nil)
		require.Equal(t, "go:some-mod", t1.ID())
	})

	t.Run("IsSame", func(t *testing.T) {
		t.Run("runtime_must_equals", func(t *testing.T) {
			t1 := Tool("rt1", "", optional.Empty[string](), nil)
			t2 := Tool("rt2", "", optional.Empty[string](), nil)
			require.False(t, t1.IsSame(t2))
		})
		t.Run("module_must_equals", func(t *testing.T) {
			t1 := Tool("rt1", "mod1", optional.Empty[string](), nil)
			t2 := Tool("rt1", "mod2", optional.Empty[string](), nil)
			require.False(t, t1.IsSame(t2))
		})
		t.Run("module_version_does_not_matter", func(t *testing.T) {
			t1 := Tool("rt1", "mod1@v999", optional.Empty[string](), nil)
			t2 := Tool("rt1", "mod1@v888", optional.Empty[string](), nil)
			require.True(t, t1.IsSame(t2))
		})
		t.Run("alias_and_tags_not_matter", func(t *testing.T) {
			t1 := Tool("rt1", "mod1", optional.Empty[string](), nil)
			t2 := Tool("rt1", "mod1", optional.New("alias"), []string{"tag1"})
			require.True(t, t1.IsSame(t2))
		})
	})
}

func Tool(runtime, module string, alias optional.Val[string], tags []string) structs.Tool {
	return structs.Tool{
		Runtime: runtime,
		Module:  module,
		Alias:   alias,
		Tags:    tags,
	}
}
