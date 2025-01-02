package fsh_test

import (
	"os"
	"testing"

	"github.com/kazhuravlev/toolset/internal/fsh"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type SampleJson struct {
	Field string `json:"field"`
}

func TestIsExists(t *testing.T) {
	fs := fsh.NewMemFS(map[string]string{
		"/foo/bar": "foo",
	})
	require.False(t, fsh.IsExists(fs, "/not/exists/path"))
	require.True(t, fsh.IsExists(fs, "/foo/bar"))
}

func TestReadJson(t *testing.T) {
	t.Run("error_on_file_not_exists", func(t *testing.T) {
		fs := fsh.NewMemFS(nil)
		res, err := fsh.ReadJson[SampleJson](fs, "/not/exists/path")
		require.Error(t, err)
		require.ErrorIs(t, err, os.ErrNotExist)
		assert.Nil(t, res)
	})

	t.Run("not_a_json_file", func(t *testing.T) {
		fs := fsh.NewMemFS(map[string]string{
			"/data.xml": "<xml></xml>",
		})

		res, err := fsh.ReadJson[SampleJson](fs, "/data.xml")
		require.Error(t, err)
		require.Nil(t, res)
	})

	t.Run("valid_file", func(t *testing.T) {
		fs := fsh.NewMemFS(nil)
		input := SampleJson{Field: "value"}
		require.NoError(t, fsh.WriteJson(fs, input, "/data.json"))

		res, err := fsh.ReadJson[SampleJson](fs, "/data.json")
		require.NoError(t, err)
		require.Equal(t, input, *res)
	})
}

func TestWriteJson(t *testing.T) {
	t.Run("write_new_file_and_rewrite_it", func(t *testing.T) {
		fs := fsh.NewMemFS(nil)
		input := SampleJson{Field: "value"}
		require.NoError(t, fsh.WriteJson(fs, input, "/data.json"))
		require.NoError(t, fsh.WriteJson(fs, input, "/data.json"))
		require.NoError(t, fsh.WriteJson(fs, input, "/data.json"))
	})

	t.Run("bad_input_structure", func(t *testing.T) {
		fs := fsh.NewMemFS(nil)
		input := map[struct{}]int{{}: 1}
		require.Error(t, fsh.WriteJson(fs, input, "/data.json"))
	})
}

func TestReadOrCreateJson(t *testing.T) {
	t.Run("auto_create_file_when_not_exists", func(t *testing.T) {
		fs := fsh.NewMemFS(nil)

		// 1. file not exists
		require.False(t, fsh.IsExists(fs, "/data.json"))

		// 2. auto-create file with default content
		res, err := fsh.ReadOrCreateJson[SampleJson](fs, "/data.json", SampleJson{Field: "default-value"})
		require.NoError(t, err)
		require.Equal(t, SampleJson{Field: "default-value"}, *res)

		// 3. file exists
		require.True(t, fsh.IsExists(fs, "/data.json"))

		// 4. content is equal to written before
		res, err = fsh.ReadOrCreateJson[SampleJson](fs, "/data.json", SampleJson{Field: "another-default"})
		require.NoError(t, err)
		require.Equal(t, SampleJson{Field: "default-value"}, *res)
	})
}
