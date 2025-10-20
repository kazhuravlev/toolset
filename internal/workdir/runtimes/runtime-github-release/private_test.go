package runtimegh

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestParse(t *testing.T) {
	t.Run("valid GitHub releases", func(t *testing.T) {
		tests := []struct {
			name        string
			input       string
			wantOwner   string
			wantRepo    string
			wantVersion string
		}{
			{
				name:        "golangci-lint",
				input:       "golangci/golangci-lint@v1.61.0",
				wantOwner:   "golangci",
				wantRepo:    "golangci-lint",
				wantVersion: "v1.61.0",
			},
			{
				name:        "another tool",
				input:       "owner/repo@v2.5.0",
				wantOwner:   "owner",
				wantRepo:    "repo",
				wantVersion: "v2.5.0",
			},
			{
				name:        "tool with patch version",
				input:       "someowner/sometool@v1.2.3",
				wantOwner:   "someowner",
				wantRepo:    "sometool",
				wantVersion: "v1.2.3",
			},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				result, err := parse(tt.input)
				require.NoError(t, err)
				require.NotNil(t, result)
				require.Equal(t, tt.wantRepo, result.Program)
				require.Equal(t, tt.wantOwner+"/"+tt.wantRepo, result.Mod.Name())
				require.Equal(t, tt.wantVersion, result.Mod.Version())
			})
		}
	})

	t.Run("invalid inputs", func(t *testing.T) {
		tests := []struct {
			name    string
			input   string
			wantErr string
		}{
			{
				name:    "empty string",
				input:   "",
				wantErr: "program name not provided",
			},
			{
				name:    "missing version",
				input:   "owner/repo",
				wantErr: "invalid github repository: should be owner/proj@v1.2.3",
			},
			{
				name:    "missing owner",
				input:   "repo@v1.0.0",
				wantErr: "invalid github path: should be owner/proj",
			},
			{
				name:    "too many slashes",
				input:   "owner/sub/repo@v1.0.0",
				wantErr: "invalid github path: should be owner/proj",
			},
			{
				name:    "invalid semver",
				input:   "owner/repo@1.0.0",
				wantErr: "non-semver versions is not supported",
			},
			{
				name:    "invalid semver format",
				input:   "owner/repo@latest",
				wantErr: "non-semver versions is not supported",
			},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				result, err := parse(tt.input)
				require.Error(t, err)
				require.Nil(t, result)
				require.Contains(t, err.Error(), tt.wantErr)
			})
		}
	})
}
