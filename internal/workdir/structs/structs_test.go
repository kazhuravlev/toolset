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

func TestTools(t *testing.T) {
	tool1 := Tool("r1", "m1", optional.Empty[string](), []string{"tag1"})
	tool2 := Tool("r2", "m2", optional.Empty[string](), []string{"tag2"})

	t.Run("tool_can_be_added_only_once", func(t *testing.T) {
		var tools structs.Tools
		tools.Add(tool1)
		tools.Add(tool1)
		require.Len(t, tools, 1)
		require.Equal(t, tool1, tools[0])
	})

	t.Run("remove_not_exists_tool", func(t *testing.T) {
		var tools structs.Tools
		tools.Remove(tool1)
		require.Len(t, tools, 0)
	})

	t.Run("remove_exists_tool", func(t *testing.T) {
		var tools structs.Tools
		tools.Add(tool1)
		require.Len(t, tools, 1)
		tools.Remove(tool1)
		require.Len(t, tools, 0)
	})

	t.Run("upsert_tool", func(t *testing.T) {
		var tools structs.Tools
		tools.UpsertTool(tool1)
		require.Len(t, tools, 1)

		tools.UpsertTool(tool1)
		require.Len(t, tools, 1)
	})

	t.Run("filter_tools", func(t *testing.T) {
		var tools structs.Tools

		t.Run("filter_empty_tools", func(t *testing.T) {
			tools.Filter(nil)
			require.Len(t, tools, 0)

			tools.Filter([]string{})
			require.Len(t, tools, 0)
		})

		tools.UpsertTool(tool1)
		tools.UpsertTool(tool2)
		require.Len(t, tools, 2)

		t.Run("filter_tools", func(t *testing.T) {
			tools2 := tools.Filter([]string{"tag1"})
			require.Len(t, tools2, 1)
			require.Equal(t, tool1, tools2[0])
			require.Len(t, tools, 2, "filtered tools should have the same length")
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
