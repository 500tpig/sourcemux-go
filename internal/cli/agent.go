package cli

import (
	"encoding/json"
	"os"

	"github.com/500tpig/sourcemux-go/internal/tools"
)

func emitAgent(out tools.AgentOutput) int {
	enc := json.NewEncoder(os.Stdout)
	_ = enc.Encode(out)
	if out.Status == "error" {
		return 1
	}
	return 0
}

func emitAgentError(mode, msg string, code int) int {
	enc := json.NewEncoder(os.Stdout)
	_ = enc.Encode(tools.BuildAgentErrorOutput(mode, msg))
	return code
}
