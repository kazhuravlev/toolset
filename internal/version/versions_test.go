package version

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestParseVersion(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		wantMajor int
		wantMinor int
		wantPatch int
		wantPart  bool
		wantErr   bool
	}{
		{
			name:      "full version",
			input:     "1.25.4",
			wantMajor: 1,
			wantMinor: 25,
			wantPatch: 4,
			wantPart:  false,
			wantErr:   false,
		},
		{
			name:      "full version with go prefix",
			input:     "go1.25.4",
			wantMajor: 1,
			wantMinor: 25,
			wantPatch: 4,
			wantPart:  false,
			wantErr:   false,
		},
		{
			name:      "partial version",
			input:     "1.25",
			wantMajor: 1,
			wantMinor: 25,
			wantPatch: 0,
			wantPart:  true,
			wantErr:   false,
		},
		{
			name:      "partial version with go prefix",
			input:     "go1.25",
			wantMajor: 1,
			wantMinor: 25,
			wantPatch: 0,
			wantPart:  true,
			wantErr:   false,
		},
		{
			name:    "invalid format",
			input:   "1.25.4.5",
			wantErr: true,
		},
		{
			name:    "invalid format - single number",
			input:   "1",
			wantErr: true,
		},
		{
			name:    "invalid format - non-numeric",
			input:   "1.x.4",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			major, minor, patch, isPartial, err := parseVersion(tt.input)
			require.Equal(t, tt.wantErr, err != nil)
			require.Equal(t, tt.wantMajor, major, "major version mismatch")
			require.Equal(t, tt.wantMinor, minor, "minor version mismatch")
			require.Equal(t, tt.wantPatch, patch, "patch version mismatch")
			require.Equal(t, tt.wantPart, isPartial, "isPartial mismatch")
		})
	}
}

func TestResolvePartialVersion(t *testing.T) {
	// Skip if running in CI without network access
	if testing.Short() {
		t.Skip("Skipping network test in short mode")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	tests := []struct {
		name    string
		input   string
		wantErr bool
	}{
		{
			name:    "partial version - 1.24",
			input:   "1.24",
			wantErr: false,
		},
		{
			name:    "partial version with go prefix",
			input:   "go1.24",
			wantErr: false,
		},
		{
			name:    "full version",
			input:   "1.24.9",
			wantErr: false,
		},
		{
			name:    "non-existent version",
			input:   "1.999",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := ResolvePartialVersion(ctx, tt.input)
			require.Equal(t, tt.wantErr, err != nil)
			if tt.wantErr {
				return
			}

			// Verify result format
			_, _, _, _, err = parseVersion(result)
			require.NoError(t, err)

			t.Logf("Resolved %s to %s", tt.input, result)
		})
	}
}

func TestListAvailableVersions(t *testing.T) {
	// Skip if running in CI without network access
	if testing.Short() {
		t.Skip("Skipping network test in short mode")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	versions, err := listAvailableVersions(ctx)
	if err != nil {
		t.Fatalf("listAvailableVersions() error = %v", err)
	}

	require.NotEmpty(t, versions)

	// Check that versions have expected format
	for _, v := range versions {
		require.NotEmpty(t, v.Version)
	}
}
