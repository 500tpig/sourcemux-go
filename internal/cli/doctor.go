package cli

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
)

type doctorOutput struct {
	ConfigFile     string        `json:"config_file"`
	OK             bool          `json:"ok"`
	MinimumProfile string        `json:"minimum_profile"`
	Checks         []doctorCheck `json:"checks"`
	Notes          []string      `json:"notes,omitempty"`
}

type doctorCheck struct {
	Name   string `json:"name"`
	Status string `json:"status"`
	Detail string `json:"detail,omitempty"`
}

func runDoctor(args []string) int {
	fs := flag.NewFlagSet("doctor", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	jsonOut := fs.Bool("json", false, "Emit JSON")
	probe := fs.Bool("probe", false, "Opt in to live provider probes; default doctor is local-only")
	if err := fs.Parse(args); err != nil {
		if err == flag.ErrHelp {
			return 0
		}
		return 2
	}
	if *probe {
		return runProbeNamed("doctor --probe", probeArgs(*jsonOut))
	}

	cfg, err := loadConfig()
	out := doctorOutput{
		ConfigFile:     currentConfigPath(),
		MinimumProfile: "off",
		Notes: []string{
			"doctor is local-only and sends zero provider requests by default",
			"use `sourcemux cli doctor --probe` or `sourcemux cli probe` to opt in to live probes",
		},
	}
	if err != nil {
		out.Checks = append(out.Checks, doctorCheck{Name: "config_load", Status: "error", Detail: err.Error()})
		return emitDoctor(*jsonOut, out)
	}
	out.MinimumProfile = cfg.MinimumProfile

	out.Checks = append(out.Checks,
		doctorCheck{Name: "config_load", Status: "ok", Detail: "config parsed"},
		doctorCheck{Name: "main_search", Status: configuredStatus(cfg.MainSearchConfigured), Detail: fmt.Sprintf("%d Grok endpoint(s)", len(cfg.GrokEndpoints))},
		doctorCheck{Name: "docs_search", Status: configuredStatus(cfg.DocsSearchConfigured), Detail: "Exa is the standard docs_search provider; Context7 does not count toward minimum_profile"},
		doctorCheck{Name: "context7", Status: configuredStatus(len(cfg.Context7Endpoints) > 0), Detail: fmt.Sprintf("%d optional endpoint(s)", len(cfg.Context7Endpoints))},
		doctorCheck{Name: "web_fetch", Status: configuredStatus(cfg.WebFetchConfigured), Detail: "Jina Reader is configured by URL and can run without a key"},
		doctorCheck{Name: "tinyfish", Status: configuredStatus(cfg.TinyFishEnabled && len(cfg.TinyFishKeys) > 0), Detail: fmt.Sprintf("%d key(s)", len(cfg.TinyFishKeys))},
		doctorCheck{Name: "tavily", Status: configuredStatus(cfg.TavilyEnabled && cfg.TavilyAPIKey != ""), Detail: "optional fallback provider"},
	)
	return emitDoctor(*jsonOut, out)
}

func probeArgs(jsonOut bool) []string {
	if jsonOut {
		return []string{"--json"}
	}
	return nil
}

func configuredStatus(ok bool) string {
	if ok {
		return "ok"
	}
	return "missing"
}

func emitDoctor(asJSON bool, out doctorOutput) int {
	out.OK = true
	for _, check := range out.Checks {
		if check.Status == "error" {
			out.OK = false
			break
		}
		if out.MinimumProfile == "standard" && (check.Name == "main_search" || check.Name == "docs_search" || check.Name == "web_fetch") {
			if check.Status != "ok" {
				out.OK = false
			}
		}
	}
	if asJSON {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		_ = enc.Encode(out)
		if out.OK {
			return 0
		}
		return 3
	}

	fmt.Printf("Config file: %s\n", out.ConfigFile)
	fmt.Printf("Minimum profile: %s\n", out.MinimumProfile)
	fmt.Println("\nChecks:")
	for _, check := range out.Checks {
		if check.Detail != "" {
			fmt.Printf("  - %s: %s (%s)\n", check.Name, check.Status, check.Detail)
		} else {
			fmt.Printf("  - %s: %s\n", check.Name, check.Status)
		}
	}
	if len(out.Notes) > 0 {
		fmt.Println("\nNotes:")
		for _, note := range out.Notes {
			fmt.Printf("  - %s\n", note)
		}
	}
	if out.OK {
		return 0
	}
	return 3
}
