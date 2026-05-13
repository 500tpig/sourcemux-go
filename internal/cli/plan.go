package cli

import (
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/500tpig/sourcemux-go/internal/tools"
)

func runPlan(args []string) int {
	fs := flag.NewFlagSet("plan", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	depth := fs.String("depth", "standard", "Research depth: quick, standard, or deep")
	platform := fs.String("platform", "", "Optional platform focus, e.g. 'GitHub, Reddit'")
	positional, err := parsePositional(fs, args)
	if err != nil {
		return 2
	}
	if len(positional) == 0 {
		fmt.Fprintln(os.Stderr, "plan: query is required")
		fs.Usage()
		return 2
	}
	query := strings.Join(positional, " ")
	fmt.Println(tools.BuildSearchPlan(query, *depth, *platform))
	return 0
}
