package cli

import (
	"fmt"
	"strings"

	cfgpkg "github.com/500tpig/sourcemux-go/internal/config"
)

func minimumProfileError(cfg *cfgpkg.Config) string {
	if cfg == nil || cfg.MinimumProfile != "standard" {
		return ""
	}
	var missing []string
	if !cfg.MainSearchConfigured {
		missing = append(missing, "main_search")
	}
	if !cfg.DocsSearchConfigured {
		missing = append(missing, "docs_search")
	}
	if !cfg.WebFetchConfigured {
		missing = append(missing, "web_fetch")
	}
	if len(missing) == 0 {
		return ""
	}
	return fmt.Sprintf("minimum_profile=standard missing required capability provider(s): %s", strings.Join(missing, ", "))
}
