package cli

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/500tpig/sourcemux-go/internal/config"
	"github.com/500tpig/sourcemux-go/internal/engine"
	"github.com/500tpig/sourcemux-go/internal/tools"
)

type smartAnswerRunner interface {
	Run(ctx context.Context, opts tools.SmartAnswerOptions) (tools.SmartAnswerResult, error)
}

func runSmartAnswer(args []string) int {
	return runSmartAnswerWithRunner(args, nil)
}

func runSmartAnswerWithRunner(args []string, runner smartAnswerRunner) int {
	fs := flag.NewFlagSet("smart-answer", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	depth := fs.String("depth", "standard", "Research depth: quick, standard, or deep")
	profile := fs.String("profile", tools.SearchProfileAuto, "Research search profile: auto, default, heavy, or another configured profile")
	platform := fs.String("platform", "", "Optional platform focus, e.g. 'GitHub, Reddit'")
	maxFetches := fs.Int("max-fetches", 0, "Maximum ranked URLs to fetch during research")
	reasoningEndpoint := fs.String("reasoning-endpoint", "", "Optional reasoning endpoint name from reasoningEndpoints")
	reasoningModel := fs.String("reasoning-model", "", "Optional one-shot reasoning model override, e.g. deepseek-v4-pro")
	timeout := fs.Duration("timeout", 360*time.Second, "End-to-end smart answer timeout")
	jsonOut := fs.Bool("json", false, "Emit JSON")
	var domains repeatedStringFlag
	fs.Var(&domains, "domain", "Domain/site allow-list entry for research; may be repeated")

	positional, err := parsePositional(fs, args)
	if err != nil {
		return 2
	}
	timeoutProvided := flagWasProvided(fs, "timeout")
	if len(positional) == 0 {
		fmt.Fprintln(os.Stderr, "smart-answer: query is required")
		fs.Usage()
		return 2
	}
	query := strings.Join(positional, " ")
	opts := tools.SmartAnswerOptions{
		Query:             query,
		Depth:             *depth,
		Profile:           *profile,
		Platform:          *platform,
		Domains:           domains,
		MaxFetches:        *maxFetches,
		ReasoningEndpoint: *reasoningEndpoint,
		ReasoningModel:    *reasoningModel,
	}

	if runner == nil {
		cfg, err := loadConfig()
		if err != nil {
			return reportSmartAnswerErr(*jsonOut, tools.SmartAnswerResult{Query: query}, fmt.Sprintf("config: %v", err))
		}
		if !timeoutProvided && cfg.SearchPolicy.Timeout > 0 {
			*timeout = cfg.SearchPolicy.Timeout + 60*time.Second
		}
		runner = buildSmartAnswerRunner(cfg)
	}

	ctx, cancel := context.WithTimeout(context.Background(), *timeout)
	defer cancel()

	res, err := runner.Run(ctx, opts)
	if err != nil {
		if res.Query == "" {
			res.Query = query
		}
		return reportSmartAnswerErr(*jsonOut, res, err.Error())
	}
	if *jsonOut {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		_ = enc.Encode(res)
		return 0
	}
	fmt.Println(tools.FormatSmartAnswerResult(res))
	return 0
}

func buildSmartAnswerRunner(cfg *config.Config) smartAnswerRunner {
	return &tools.SmartAnswerer{
		Researcher: buildResearchRunner(cfg),
		Reasoner:   engine.NewReasoningPool(cfg.ReasoningEndpoints),
	}
}

func reportSmartAnswerErr(asJSON bool, res tools.SmartAnswerResult, msg string) int {
	res.Error = msg
	if asJSON {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		_ = enc.Encode(res)
		return 1
	}
	fmt.Fprintln(os.Stderr, msg)
	return 1
}
