package fsh_test

import (
	"context"
	"os"
	"runtime"
	"testing"
	"time"

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
	ctx := context.Background()

	t.Run("error_on_file_not_exists", func(t *testing.T) {
		fs := fsh.NewMemFS(nil)
		res, err := fsh.ReadJson[SampleJson](ctx, fs, "/not/exists/path")
		require.Error(t, err)
		require.ErrorIs(t, err, os.ErrNotExist)
		assert.Nil(t, res)
	})

	t.Run("not_a_json_file", func(t *testing.T) {
		fs := fsh.NewMemFS(map[string]string{
			"/data.xml": "<xml></xml>",
		})

		res, err := fsh.ReadJson[SampleJson](ctx, fs, "/data.xml")
		require.Error(t, err)
		require.Nil(t, res)
	})

	t.Run("valid_file", func(t *testing.T) {
		fs := fsh.NewMemFS(nil)
		input := SampleJson{Field: "value"}
		require.NoError(t, fsh.WriteJson(ctx, fs, input, "/data.json"))

		res, err := fsh.ReadJson[SampleJson](ctx, fs, "/data.json")
		require.NoError(t, err)
		require.Equal(t, input, *res)
	})
}

func TestWriteJson(t *testing.T) {
	ctx := context.Background()

	t.Run("write_new_file_and_rewrite_it", func(t *testing.T) {
		fs := fsh.NewMemFS(nil)
		input := SampleJson{Field: "value"}
		require.NoError(t, fsh.WriteJson(ctx, fs, input, "/data.json"))
		require.NoError(t, fsh.WriteJson(ctx, fs, input, "/data.json"))
		require.NoError(t, fsh.WriteJson(ctx, fs, input, "/data.json"))
	})

	t.Run("bad_input_structure", func(t *testing.T) {
		fs := fsh.NewMemFS(nil)
		input := map[struct{}]int{{}: 1}
		require.Error(t, fsh.WriteJson(ctx, fs, input, "/data.json"))
	})
}

func TestReadOrCreateJson(t *testing.T) {
	ctx := context.Background()

	t.Run("auto_create_file_when_not_exists", func(t *testing.T) {
		if runtime.GOOS == "windows" {
			t.Skip("skip for Windows")
		}

		fs := fsh.NewMemFS(nil)

		// 1. file not exists
		require.False(t, fsh.IsExists(fs, "/data.json"))

		// 2. auto-create file with default content
		res, err := fsh.ReadOrCreateJson[SampleJson](ctx, fs, "/data.json", SampleJson{Field: "default-value"})
		require.NoError(t, err)
		require.Equal(t, SampleJson{Field: "default-value"}, *res)

		// 3. file exists
		require.True(t, fsh.IsExists(fs, "/data.json"))

		// 4. content is equal to written before
		res, err = fsh.ReadOrCreateJson[SampleJson](ctx, fs, "/data.json", SampleJson{Field: "another-default"})
		require.NoError(t, err)
		require.Equal(t, SampleJson{Field: "default-value"}, *res)
	})
}

func TestLocks(t *testing.T) {
	ctx := context.Background()

	t.Run("auto_create_file_when_not_exists", func(t *testing.T) {
		if runtime.GOOS == "windows" {
			t.Skip("skip for Windows")
		}

		fs := fsh.NewRealFS()

		const filename = "/tmp/test.lock"

		// Lock file
		unlock, err := fs.Lock(ctx, filename)
		require.NoError(t, err)

		ch := make(chan struct{})
		go func() {
			// Lock file again. It will stuck.
			unlock, err := fs.Lock(ctx, filename)
			require.NoError(t, err)

			close(ch)
			unlock()
		}()

		select {
		case <-ch:
			t.Fatal("File should be locked")
		case <-time.After(1 * time.Second):
		}

		unlock()
		select {
		case <-ch:
		case <-time.After(1 * time.Second):
			t.Fatal("File should be unlocked")
		}
	})
}
