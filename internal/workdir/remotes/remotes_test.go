package remotes_test

import (
	"context"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/kazhuravlev/toolset/internal/workdir/remotes"

	"github.com/kazhuravlev/optional"
	"github.com/kazhuravlev/toolset/internal/fsh"
	"github.com/kazhuravlev/toolset/internal/workdir/structs"

	"github.com/stretchr/testify/require"
)

func TestParseRemote(t *testing.T) {
	t.Run("valid_cases", func(t *testing.T) {
		f := func(uri string, exp remotes.SourceUri) {
			res, err := remotes.ParseRemote(uri)
			require.NoError(t, err)
			require.NotNil(t, res)
			require.Equal(t, exp, res)
		}

		// NOTE: on Windows it looks like D:\path\to\file.txt
		pp, _ := filepath.Abs("/path/to/file.txt")
		f("/path/to/file.txt",
			remotes.SourceUriFile{Path: pp})
		f("http://127.0.0.1:8000/path/to/file.txt",
			remotes.SourceUriUrl{URL: "http://127.0.0.1:8000/path/to/file.txt"})
		f("https://127.0.0.1:8000/path/to/file.txt",
			remotes.SourceUriUrl{URL: "https://127.0.0.1:8000/path/to/file.txt"})
		f("git+ssh://127.0.0.1:/path/to/file.txt",
			remotes.SourceUriGit{Addr: "127.0.0.1", Path: "/path/to/file.txt"})
		f("git+https://127.0.0.1:/path/to/file.txt",
			remotes.SourceUriGit{Addr: "https://127.0.0.1", Path: "/path/to/file.txt"})
	})

	t.Run("invalid_cases", func(t *testing.T) {
		res, err := remotes.ParseRemote("ftp://127.0.0.1:8000/path/to/file.txt")
		require.Error(t, err)
		require.Nil(t, res)
	})
}

func TestFetchRemote(t *testing.T) {
	t.Run("file_src", func(t *testing.T) {
		if runtime.GOOS == "windows" {
			t.Skip("skip for Windows")
		}

		ctx := context.Background()
		fs := fsh.NewMemFS(map[string]string{
			"/.toolset.json": `{
			"dir": "./bin/tools",
			"tools": [
				{
					"runtime": "go",
					"module": "golang.org/x/tools/cmd/goimports@v0.28.0",
					"alias": null,
					"tags": ["tag1"]
				}
			],
			"includes": [
				{
					"src": "/.toolset.json",
					"tags": ["tag3"]
				}
			]
		}`,
		})

		res, err := remotes.FetchRemote(ctx, fs, "/.toolset.json", []string{"tag2"}, nil)
		require.NoError(t, err)
		require.Len(t, res, 1)
		require.Equal(t, structs.RemoteSpec{
			Source: "/.toolset.json",
			Spec: structs.Spec{
				Dir: "./bin/tools",
				Tools: structs.Tools{
					{
						Runtime: "go",
						Module:  "golang.org/x/tools/cmd/goimports@v0.28.0",
						Alias:   optional.Empty[string](),
						Tags:    []string{"tag1"},
					},
				},
				Includes: []structs.Include{
					{
						Src:  "/.toolset.json",
						Tags: []string{"tag3"},
					},
				},
			},
			Tags: []string{"tag2"},
		}, res[0])
	})

	t.Run("git_https_src", func(t *testing.T) {
		ctx := context.Background()
		fs := fsh.NewRealFS()

		res, err := remotes.FetchRemote(ctx, fs, "git+https://gist.github.com/3f16049ce3f9f478e6b917237b2c0d88.git:/sample-toolset.json", nil, nil)
		require.NoError(t, err)
		require.Len(t, res, 1)
		require.Equal(t, structs.RemoteSpec{
			Source: "git+https://gist.github.com/3f16049ce3f9f478e6b917237b2c0d88.git:/sample-toolset.json",
			Spec: structs.Spec{
				Dir: "./bin/tools",
				Tools: structs.Tools{
					{Runtime: "go", Module: "golang.org/x/tools/cmd/stringer@v0.26.0", Alias: optional.Empty[string](), Tags: nil},
					{Runtime: "go", Module: "github.com/kazhuravlev/options-gen/cmd/options-gen@v0.33.0", Alias: optional.Empty[string](), Tags: nil},
					{Runtime: "go", Module: "golang.org/x/tools/cmd/goimports@v0.26.0", Alias: optional.Empty[string](), Tags: nil},
					{Runtime: "go", Module: "mvdan.cc/gofumpt@v0.7.0", Alias: optional.Empty[string](), Tags: nil},
					{Runtime: "go", Module: "github.com/kazhuravlev/structspec/cmd/structspec@v0.4.2", Alias: optional.Empty[string](), Tags: nil},
					{Runtime: "go", Module: "gotest.tools/gotestsum@v1.12.0", Alias: optional.Empty[string](), Tags: nil},
					{Runtime: "go", Module: "github.com/hexdigest/gowrap/cmd/gowrap@v1.4.0", Alias: optional.Empty[string](), Tags: nil},
					{Runtime: "go", Module: "github.com/vburenin/ifacemaker@v1.2.1", Alias: optional.Empty[string](), Tags: nil},
					{Runtime: "go", Module: "github.com/golangci/golangci-lint/cmd/golangci-lint@v1.61.0", Alias: optional.Empty[string](), Tags: nil},
				},
				Includes: nil,
			},
			Tags: nil,
		}, res[0])
	})
}
