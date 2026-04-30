package cli

import "flag"

// parsePositional fully consumes args, supporting flags interleaved with
// positional arguments. Standard flag.FlagSet.Parse stops at the first
// non-flag token, which is hostile for CLIs where users naturally type the
// query first (e.g. `cli plan "my query" --depth deep`). This helper loops
// Parse + collect-positional until everything is consumed.
//
// It returns the collected positional args (in the order they appeared) and
// any parse error from the flag package. On error, partial positionals are
// still returned so callers can surface a useful message; most callers will
// just exit 2 on err.
func parsePositional(fs *flag.FlagSet, args []string) ([]string, error) {
	var positional []string
	rest := args
	for {
		if err := fs.Parse(rest); err != nil {
			return positional, err
		}
		if fs.NArg() == 0 {
			return positional, nil
		}
		positional = append(positional, fs.Arg(0))
		rest = fs.Args()[1:]
	}
}
