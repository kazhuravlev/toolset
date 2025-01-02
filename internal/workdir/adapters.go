package workdir

import (
	"time"

	"github.com/kazhuravlev/optional"
	"github.com/kazhuravlev/toolset/internal/workdir/structs"
)

func adaptToolState(tool structs.Tool, mod *structs.ModuleInfo, lastUse optional.Val[time.Time]) structs.ToolState {
	return structs.ToolState{
		Tool:    tool,
		LastUse: lastUse,
		Module:  *mod,
	}
}
