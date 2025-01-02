package workdir

import (
	"github.com/stretchr/testify/require"
	"testing"
)

func Test_parseSourceURI(t *testing.T) {
	t.Run("valid_cases", func(t *testing.T) {
		f := func(uri string, exp SourceUri) {
			res, err := parseSourceURI(uri)
			require.NoError(t, err)
			require.NotNil(t, res)
			require.Equal(t, exp, res)
		}

		f("/path/to/file.txt",
			SourceUriFile{Path: "/path/to/file.txt"})
		f("http://127.0.0.1:8000/path/to/file.txt",
			SourceUriUrl{URL: "http://127.0.0.1:8000/path/to/file.txt"})
		f("https://127.0.0.1:8000/path/to/file.txt",
			SourceUriUrl{URL: "https://127.0.0.1:8000/path/to/file.txt"})
		f("git+ssh://127.0.0.1:/path/to/file.txt",
			SourceUriGit{Addr: "127.0.0.1", Path: "/path/to/file.txt"})
		f("git+https://127.0.0.1:/path/to/file.txt",
			SourceUriGit{Addr: "https://127.0.0.1", Path: "/path/to/file.txt"})
	})

	t.Run("invalid_cases", func(t *testing.T) {
		res, err := parseSourceURI("ftp://127.0.0.1:8000/path/to/file.txt")
		require.Error(t, err)
		require.Nil(t, res)
	})
}
