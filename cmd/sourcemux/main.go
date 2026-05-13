package main

import (
	"os"

	"github.com/500tpig/sourcemux-go/internal/app"
)

var (
	version = "dev"
	commit  = "none"
	date    = "unknown"
)

func main() {
	app.SetVersionInfo(version, commit, date)
	os.Exit(app.Run(os.Args[1:]))
}
