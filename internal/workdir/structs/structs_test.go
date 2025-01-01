package structs_test

import (
	"testing"

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
