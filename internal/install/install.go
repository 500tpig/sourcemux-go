// Package install implements the top-level SourceMux installer surface.
package install

import (
	"crypto/sha256"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/500tpig/sourcemux-go/internal/config"
)

const (
	defaultScope          = "project"
	defaultUserConfigPath = "~/.config/sourcemux/sourcemux.json"
	manifestName          = ".sourcemux-install.json"
	skillName             = "sourcemux-routing"
)

const installUsage = `Usage: sourcemux bootstrap <target...> [flags]
       sourcemux bootstrap update <target...> [flags]
       sourcemux bootstrap list-agents [--json]
       sourcemux bootstrap status [target...] [--scope project|user] [--config-status] [--json]
       sourcemux install <target...> [flags]
       sourcemux install update <target...> [flags]
       sourcemux install list-agents [--json]
       sourcemux install status [target...] [--scope project|user] [--config-status] [--json]

Targets:
  codex, claude-code, gemini, opencode, copilot, cursor, trellis, mcp-json, stdio

Flags:
  --agent <name>      Add an agent target; can be repeated.
  --all               Select all installable targets.
  --scope <scope>     Install scope: project or user (default: project).
  --binary <path>     SourceMux binary path for generated commands.
  --config <path>     SourceMux config path passed to MCP/CLI snippets.
                      Defaults by scope: user=~/.config/sourcemux/sourcemux.json, project=./sourcemux.json.
  --write-config      Safely merge supported MCP client config files.
  --dry-run           Print planned changes without writing files.
  --force             Back up and replace conflicting generated skill files.
  --json              Emit machine-readable JSON.
  --help, -h          Show this usage.

Examples:
  sourcemux bootstrap list-agents
  sourcemux bootstrap codex claude-code --scope user
  sourcemux bootstrap update codex --scope user
  sourcemux bootstrap codex --scope project
  sourcemux bootstrap --agent codex --agent opencode --dry-run --json
  sourcemux bootstrap --all --dry-run
`

const uninstallUsage = `Usage: sourcemux uninstall <target...> [flags]

Flags:
  --agent <name>      Add an agent target; can be repeated.
  --all               Select all targets with generated skill files.
  --scope <scope>     Install scope: project or user (default: project).
  --write-config      Safely remove supported SourceMux MCP config entries.
  --dry-run           Print planned removals without deleting files.
  --force             Back up and remove modified or unmanaged generated skill files.
  --json              Emit machine-readable JSON.
  --help, -h          Show this usage.

Examples:
  sourcemux uninstall --all --scope user --write-config --dry-run
  sourcemux uninstall --all --scope user --write-config --force
  sourcemux uninstall codex --scope project --write-config
  sourcemux uninstall codex --scope project --force
`

type SupportLevel string

const (
	SupportFull       SupportLevel = "full"
	SupportSkillFirst SupportLevel = "skill-first"
	SupportProfile    SupportLevel = "profile"
	SupportPrintOnly  SupportLevel = "print-only"
)

type Target struct {
	Name        string       `json:"name"`
	Aliases     []string     `json:"aliases,omitempty"`
	Support     SupportLevel `json:"support"`
	Tier        int          `json:"tier"`
	Description string       `json:"description"`
	ProjectRoot string       `json:"project_skill_root,omitempty"`
	UserRoot    string       `json:"user_skill_root,omitempty"`
	Skill       bool         `json:"skill"`
	MCP         string       `json:"mcp"`
}

type TargetStatus struct {
	Name                string        `json:"name"`
	Support             SupportLevel  `json:"support"`
	Scope               string        `json:"scope"`
	SkillPath           string        `json:"skill_path,omitempty"`
	ManifestPath        string        `json:"manifest_path,omitempty"`
	Installed           bool          `json:"installed"`
	Managed             bool          `json:"managed"`
	Modified            bool          `json:"modified"`
	InstallMode         string        `json:"install_mode,omitempty"`
	Notes               []string      `json:"notes,omitempty"`
	Issues              []StatusIssue `json:"issues,omitempty"`
	BinaryStatus        *PathStatus   `json:"binary_status,omitempty"`
	RuntimeConfigStatus *PathStatus   `json:"runtime_config_status,omitempty"`
	ScopeStatus         *ScopeStatus  `json:"scope_status,omitempty"`
	ConfigStatus        *ConfigStatus `json:"config_status,omitempty"`
}

type ConfigStatus struct {
	Supported    bool   `json:"supported"`
	Path         string `json:"path,omitempty"`
	Exists       bool   `json:"exists"`
	EntryPresent bool   `json:"entry_present"`
	Matches      bool   `json:"matches"`
	Drifted      bool   `json:"drifted"`
	Status       string `json:"status"`
	Error        string `json:"error,omitempty"`
	Message      string `json:"message,omitempty"`
}

type PathStatus struct {
	Path            string `json:"path,omitempty"`
	Expected        string `json:"expected,omitempty"`
	Exists          bool   `json:"exists"`
	MatchesExpected bool   `json:"matches_expected"`
	Status          string `json:"status"`
	Message         string `json:"message,omitempty"`
}

type ScopeStatus struct {
	Requested string `json:"requested"`
	Detected  string `json:"detected,omitempty"`
	SkillPath string `json:"skill_path,omitempty"`
	Status    string `json:"status"`
	Message   string `json:"message,omitempty"`
}

type StatusIssue struct {
	Code     string `json:"code"`
	Severity string `json:"severity"`
	Message  string `json:"message"`
	Path     string `json:"path,omitempty"`
	Expected string `json:"expected,omitempty"`
	Repair   string `json:"repair,omitempty"`
}

type Plan struct {
	Mode       string       `json:"mode"`
	Scope      string       `json:"scope"`
	ConfigFile string       `json:"config_file"`
	Binary     string       `json:"binary"`
	DryRun     bool         `json:"dry_run"`
	Actions    []PlanAction `json:"actions"`
	Warnings   []string     `json:"warnings,omitempty"`
}

type PlanAction struct {
	Type      string   `json:"type"`
	Target    string   `json:"target"`
	Path      string   `json:"path,omitempty"`
	Manifest  string   `json:"manifest_path,omitempty"`
	Status    string   `json:"status"`
	Command   []string `json:"command,omitempty"`
	MCPJSON   string   `json:"mcp_json,omitempty"`
	Snippet   string   `json:"snippet,omitempty"`
	Backup    string   `json:"backup,omitempty"`
	Message   string   `json:"message,omitempty"`
	Sensitive bool     `json:"sensitive,omitempty"`
	MCPMode   bool     `json:"mcp_mode,omitempty"`
}

type options struct {
	Scope        string
	ConfigPath   string
	BinaryPath   string
	DryRun       bool
	Force        bool
	JSON         bool
	All          bool
	WriteConfig  bool
	ConfigStatus bool
	Agents       []string
}

type installManifest struct {
	Version       int    `json:"version"`
	Generator     string `json:"generator"`
	Target        string `json:"target"`
	SkillPath     string `json:"skill_path"`
	Binary        string `json:"binary"`
	ConfigFile    string `json:"config_file"`
	MCPMode       bool   `json:"mcp_mode,omitempty"`
	ContentSHA256 string `json:"content_sha256"`
	InstalledAt   string `json:"installed_at"`
}

var targets = []Target{
	{
		Name:        "codex",
		Support:     SupportFull,
		Tier:        1,
		Description: "Codex skill plus SourceMux MCP stdio JSON/command guidance.",
		ProjectRoot: ".agents/skills",
		UserRoot:    "~/.codex/skills",
		Skill:       true,
		MCP:         "codex mcp add + config.toml",
	},
	{
		Name:        "claude-code",
		Aliases:     []string{"claude"},
		Support:     SupportFull,
		Tier:        1,
		Description: "Claude Code skill plus claude mcp add-json guidance.",
		ProjectRoot: ".claude/skills",
		UserRoot:    "~/.claude/skills",
		Skill:       true,
		MCP:         "claude mcp add + mcp-json",
	},
	{
		Name:        "gemini",
		Aliases:     []string{"gemini-cli"},
		Support:     SupportFull,
		Tier:        1,
		Description: "Gemini CLI skill plus MCP stdio JSON/command guidance.",
		ProjectRoot: ".gemini/skills",
		UserRoot:    "~/.gemini/skills",
		Skill:       true,
		MCP:         "gemini mcp add + settings.json",
	},
	{
		Name:        "opencode",
		Support:     SupportFull,
		Tier:        1,
		Description: "OpenCode skill plus MCP stdio JSON/command guidance.",
		ProjectRoot: ".opencode/skills",
		UserRoot:    "~/.opencode/skills",
		Skill:       true,
		MCP:         "opencode.json",
	},
	{
		Name:        "copilot",
		Aliases:     []string{"github-copilot"},
		Support:     SupportSkillFirst,
		Tier:        2,
		Description: "GitHub Copilot agent skill first; MCP config is emitted as JSON.",
		ProjectRoot: ".github/skills",
		UserRoot:    "~/.copilot/skills",
		Skill:       true,
		MCP:         "mcp-json",
	},
	{
		Name:        "cursor",
		Support:     SupportSkillFirst,
		Tier:        2,
		Description: "Cursor routing skill first; MCP config is emitted as JSON.",
		ProjectRoot: ".agents/skills",
		UserRoot:    "~/.agents/skills",
		Skill:       true,
		MCP:         "mcp-json",
	},
	{
		Name:        "trellis",
		Support:     SupportProfile,
		Tier:        2,
		Description: "Trellis profile skill installed into shared agent skills.",
		ProjectRoot: ".agents/skills",
		UserRoot:    "~/.agents/skills",
		Skill:       true,
		MCP:         "none",
	},
	{
		Name:        "mcp-json",
		Support:     SupportPrintOnly,
		Tier:        2,
		Description: "Print copyable MCP JSON for manual client configuration.",
		MCP:         "print",
	},
	{
		Name:        "stdio",
		Support:     SupportPrintOnly,
		Tier:        2,
		Description: "Print the exact stdio command for manual client configuration.",
		MCP:         "print",
	},
}

// RunInstall executes the top-level `sourcemux install` command.
func RunInstall(args []string, configPath string) int {
	if len(args) == 0 {
		fmt.Fprint(os.Stdout, installUsage)
		return 0
	}
	switch args[0] {
	case "-h", "--help":
		fmt.Fprint(os.Stdout, installUsage)
		return 0
	case "list-agents", "list-targets":
		return runListAgents(args[1:])
	case "status":
		return runStatus(args[1:], configPath)
	case "update":
		return runInstallMode("update", args[1:], configPath)
	}

	return runInstallMode("install", args, configPath)
}

func runInstallMode(mode string, args []string, configPath string) int {
	opts, err := parseOptions(args, configPath, false)
	if err != nil {
		if errors.Is(err, flag.ErrHelp) {
			fmt.Fprint(os.Stdout, installUsage)
			return 0
		}
		fmt.Fprintf(os.Stderr, "install argument error: %v\n", err)
		return 2
	}
	plan, err := BuildPlan(mode, opts)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s error: %v\n", mode, err)
		return 2
	}
	if !opts.DryRun {
		printPreApplyBackupNotice(plan)
		if err := ApplyPlan(plan, opts); err != nil {
			fmt.Fprintf(os.Stderr, "%s error: %v\n", mode, err)
			return 1
		}
	}
	printPlan(plan, opts.JSON)
	return 0
}

// RunUninstall executes the top-level `sourcemux uninstall` command.
func RunUninstall(args []string, configPath string) int {
	if len(args) == 0 || args[0] == "-h" || args[0] == "--help" {
		fmt.Fprint(os.Stdout, uninstallUsage)
		return 0
	}
	opts, err := parseOptions(args, configPath, true)
	if err != nil {
		if errors.Is(err, flag.ErrHelp) {
			fmt.Fprint(os.Stdout, uninstallUsage)
			return 0
		}
		fmt.Fprintf(os.Stderr, "uninstall argument error: %v\n", err)
		return 2
	}
	plan, err := BuildPlan("uninstall", opts)
	if err != nil {
		fmt.Fprintf(os.Stderr, "uninstall error: %v\n", err)
		return 2
	}
	if !opts.DryRun {
		printPreApplyBackupNotice(plan)
		if err := ApplyPlan(plan, opts); err != nil {
			fmt.Fprintf(os.Stderr, "uninstall error: %v\n", err)
			return 1
		}
	}
	printPlan(plan, opts.JSON)
	return 0
}

func runListAgents(args []string) int {
	fs := flag.NewFlagSet("install list-agents", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	asJSON := fs.Bool("json", false, "Emit JSON")
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		return 2
	}
	if fs.NArg() != 0 {
		fmt.Fprintf(os.Stderr, "install list-agents does not accept positional arguments\n")
		return 2
	}
	if *asJSON {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		_ = enc.Encode(targets)
		return 0
	}
	fmt.Fprintln(os.Stdout, "Target          Support       Tier  Skill  MCP")
	for _, t := range targets {
		fmt.Fprintf(os.Stdout, "%-15s %-13s %-5d %-6v %s\n", t.Name, t.Support, t.Tier, t.Skill, t.MCP)
	}
	return 0
}

func runStatus(args []string, configPath string) int {
	opts, err := parseOptions(args, configPath, false)
	if err != nil {
		if errors.Is(err, flag.ErrHelp) {
			fmt.Fprint(os.Stdout, installUsage)
			return 0
		}
		fmt.Fprintf(os.Stderr, "install status argument error: %v\n", err)
		return 2
	}
	selected, err := resolveTargets(opts)
	if err != nil {
		fmt.Fprintf(os.Stderr, "install status error: %v\n", err)
		return 2
	}
	var statuses []TargetStatus
	bin, err := resolveBinaryPath(opts.BinaryPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "install status error: %v\n", err)
		return 2
	}
	cfgPath := ""
	if opts.ConfigStatus {
		cfgPath, err = resolveConfigPath(opts.Scope, opts.ConfigPath)
		if err != nil {
			fmt.Fprintf(os.Stderr, "install status error: resolve config path: %v\n", err)
			return 2
		}
	}
	for _, target := range selected {
		statuses = append(statuses, statusFor(target, opts.Scope, opts.ConfigStatus, bin, cfgPath))
	}
	if opts.JSON {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		_ = enc.Encode(statuses)
		return 0
	}
	for _, s := range statuses {
		state := "not installed"
		if s.Installed {
			state = "installed"
		}
		if s.SkillPath == "" {
			fmt.Fprintf(os.Stdout, "%s: %s (%s)\n", s.Name, s.Support, strings.Join(s.Notes, "; "))
		} else {
			mode := s.InstallMode
			if mode == "" {
				mode = "unknown"
			}
			fmt.Fprintf(os.Stdout, "%s: %s at %s [%s, %s]\n", s.Name, state, s.SkillPath, s.Support, mode)
		}
		if s.ConfigStatus != nil {
			cs := s.ConfigStatus
			if cs.Supported {
				fmt.Fprintf(os.Stdout, "  config: %s at %s\n", cs.Status, cs.Path)
			} else {
				fmt.Fprintf(os.Stdout, "  config: unsupported\n")
			}
		}
		for _, issue := range s.Issues {
			fmt.Fprintf(os.Stdout, "  issue: %s: %s\n", issue.Code, issue.Message)
		}
	}
	return 0
}

func BuildPlan(mode string, opts options) (Plan, error) {
	if opts.Scope == "" {
		opts.Scope = defaultScope
	}
	if opts.Scope != "project" && opts.Scope != "user" {
		return Plan{}, fmt.Errorf("unsupported scope %q (want project or user)", opts.Scope)
	}
	selected, err := resolveTargets(opts)
	if err != nil {
		return Plan{}, err
	}
	bin, err := resolveBinaryPath(opts.BinaryPath)
	if err != nil {
		return Plan{}, err
	}
	cfgPath, err := resolveConfigPath(opts.Scope, opts.ConfigPath)
	if err != nil {
		return Plan{}, fmt.Errorf("resolve config path: %w", err)
	}
	plan := Plan{
		Mode:       mode,
		Scope:      opts.Scope,
		ConfigFile: cfgPath,
		Binary:     bin,
		DryRun:     opts.DryRun,
	}
	if looksTemporaryExecutable(bin) {
		plan.Warnings = append(plan.Warnings, "binary path looks temporary; build or install sourcemux first, or pass --binary /absolute/path/to/sourcemux")
	}
	for _, target := range selected {
		switch mode {
		case "install", "update":
			addInstallActions(&plan, target, opts.Scope, opts.WriteConfig)
			if opts.WriteConfig {
				if err := addConfigWriteAction(&plan, target, opts.Scope); err != nil {
					return Plan{}, err
				}
			}
		case "uninstall":
			addUninstallActions(&plan, target, opts.Scope, opts.WriteConfig, opts.Force)
			if opts.WriteConfig {
				if err := addConfigRemoveAction(&plan, target, opts.Scope); err != nil {
					return Plan{}, err
				}
			}
		default:
			return Plan{}, fmt.Errorf("unsupported mode %q", mode)
		}
	}
	if len(plan.Actions) == 0 {
		return Plan{}, fmt.Errorf("no actions for selected targets")
	}
	return plan, nil
}

func ApplyPlan(plan Plan, opts options) error {
	for i := range plan.Actions {
		action := &plan.Actions[i]
		switch action.Type {
		case "write_file":
			content := []byte(routingSkill(plan.Binary, plan.ConfigFile, plan.Scope, action.MCPMode))
			manifest := installManifest{
				Version:       1,
				Generator:     "sourcemux install",
				Target:        action.Target,
				SkillPath:     action.Path,
				Binary:        plan.Binary,
				ConfigFile:    plan.ConfigFile,
				MCPMode:       action.MCPMode,
				ContentSHA256: contentSHA256(content),
				InstalledAt:   time.Now().UTC().Format(time.RFC3339),
			}
			status, backup, err := writeGeneratedSkill(action.Path, content, opts.Force, manifest)
			if err != nil {
				return err
			}
			action.Status = status
			action.Backup = backup
		case "remove_file":
			status, backup, err := removeGeneratedSkill(action.Path, action.Target, opts.Force)
			if err != nil {
				return err
			}
			action.Status = status
			action.Backup = backup
		case "merge_config":
			status, backup, err := writeMCPConfig(action.Target, action.Path, plan.Binary, plan.ConfigFile, action.Backup)
			if err != nil {
				return err
			}
			action.Status = status
			action.Backup = backup
		case "remove_config":
			status, backup, err := removeMCPConfig(action.Target, action.Path, action.Backup)
			if err != nil {
				return err
			}
			action.Status = status
			action.Backup = backup
		case "config_snippet", "mcp_json", "shell_command", "stdio", "note":
			if action.Status == "" {
				action.Status = "informational"
			}
		default:
			return fmt.Errorf("unknown action type %q", action.Type)
		}
	}
	return nil
}

func parseOptions(args []string, configPath string, uninstall bool) (options, error) {
	opts := options{Scope: defaultScope, ConfigPath: strings.TrimSpace(configPath)}
	var positionals []string
	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch {
		case arg == "--agent":
			if i+1 >= len(args) || strings.TrimSpace(args[i+1]) == "" {
				return opts, fmt.Errorf("--agent requires a value")
			}
			opts.Agents = append(opts.Agents, args[i+1])
			i++
		case strings.HasPrefix(arg, "--agent="):
			value := strings.TrimSpace(strings.TrimPrefix(arg, "--agent="))
			if value == "" {
				return opts, fmt.Errorf("--agent requires a value")
			}
			opts.Agents = append(opts.Agents, value)
		case arg == "--scope":
			if i+1 >= len(args) || strings.TrimSpace(args[i+1]) == "" {
				return opts, fmt.Errorf("--scope requires a value")
			}
			opts.Scope = args[i+1]
			i++
		case strings.HasPrefix(arg, "--scope="):
			opts.Scope = strings.TrimSpace(strings.TrimPrefix(arg, "--scope="))
		case arg == "--binary":
			if i+1 >= len(args) || strings.TrimSpace(args[i+1]) == "" {
				return opts, fmt.Errorf("--binary requires a path")
			}
			opts.BinaryPath = args[i+1]
			i++
		case strings.HasPrefix(arg, "--binary="):
			opts.BinaryPath = strings.TrimSpace(strings.TrimPrefix(arg, "--binary="))
			if opts.BinaryPath == "" {
				return opts, fmt.Errorf("--binary requires a path")
			}
		case arg == "--config" || arg == "-c":
			if i+1 >= len(args) || strings.TrimSpace(args[i+1]) == "" {
				return opts, fmt.Errorf("%s requires a path", arg)
			}
			opts.ConfigPath = args[i+1]
			i++
		case strings.HasPrefix(arg, "--config="):
			opts.ConfigPath = strings.TrimSpace(strings.TrimPrefix(arg, "--config="))
			if opts.ConfigPath == "" {
				return opts, fmt.Errorf("--config requires a path")
			}
		case arg == "--dry-run":
			opts.DryRun = true
		case arg == "--force":
			opts.Force = true
		case arg == "--write-config":
			opts.WriteConfig = true
		case arg == "--config-status":
			opts.ConfigStatus = true
		case arg == "--json":
			opts.JSON = true
		case arg == "--all":
			opts.All = true
		case arg == "-h" || arg == "--help":
			return opts, flag.ErrHelp
		case strings.HasPrefix(arg, "-"):
			return opts, fmt.Errorf("unknown flag %q", arg)
		default:
			positionals = append(positionals, arg)
		}
	}
	opts.Agents = append(opts.Agents, positionals...)
	return opts, nil
}

func resolveTargets(opts options) ([]Target, error) {
	var selected []Target
	if opts.All {
		for _, target := range targets {
			if target.Name == "mcp-json" || target.Name == "stdio" {
				continue
			}
			selected = append(selected, target)
		}
		return selected, nil
	}
	if len(opts.Agents) == 0 {
		for _, target := range targets {
			if target.Name == "mcp-json" || target.Name == "stdio" {
				continue
			}
			selected = append(selected, target)
		}
		return selected, nil
	}
	seen := map[string]bool{}
	for _, name := range opts.Agents {
		target, ok := lookupTarget(name)
		if !ok {
			return nil, fmt.Errorf("unknown target %q", name)
		}
		if seen[target.Name] {
			continue
		}
		selected = append(selected, target)
		seen[target.Name] = true
	}
	return selected, nil
}

func lookupTarget(name string) (Target, bool) {
	needle := strings.ToLower(strings.TrimSpace(name))
	for _, target := range targets {
		if target.Name == needle {
			return target, true
		}
		for _, alias := range target.Aliases {
			if alias == needle {
				return target, true
			}
		}
	}
	return Target{}, false
}

func addInstallActions(plan *Plan, target Target, scope string, writeConfig bool) {
	mcpMode := targetMCPMode(target, writeConfig)
	if target.Skill {
		path, err := skillPath(target, scope)
		if err != nil {
			plan.Warnings = append(plan.Warnings, fmt.Sprintf("%s: %v", target.Name, err))
		} else {
			plan.Actions = append(plan.Actions, PlanAction{
				Type:     "write_file",
				Target:   target.Name,
				Path:     path,
				Manifest: manifestPath(path),
				Status:   "planned",
				Message:  installSkillMessage(mcpMode),
				MCPMode:  mcpMode,
			})
		}
	}
	if !writeConfig && target.Skill {
		plan.Warnings = append(plan.Warnings, fmt.Sprintf("%s: generated skill will be CLI-first; pass --write-config to request MCP setup guidance for supported clients.", target.Name))
	}
	switch target.Name {
	case "codex":
		if writeConfig {
			plan.Actions = append(plan.Actions, codexCommandAction(*plan), codexConfigSnippetAction(*plan, scope))
		}
	case "claude-code":
		if writeConfig {
			plan.Actions = append(plan.Actions, claudeCommandAction(*plan, scope), mcpJSONAction(*plan, target.Name))
		}
	case "gemini":
		if writeConfig {
			plan.Actions = append(plan.Actions, geminiCommandAction(*plan, scope), geminiConfigSnippetAction(*plan, scope))
		}
	case "opencode":
		if writeConfig {
			plan.Actions = append(plan.Actions, opencodeConfigSnippetAction(*plan, scope))
		}
	case "stdio":
		plan.Actions = append(plan.Actions, stdioAction(*plan, target.Name))
	case "mcp-json":
		plan.Actions = append(plan.Actions, mcpJSONAction(*plan, target.Name))
	default:
		if writeConfig && target.MCP == "mcp-json" {
			plan.Actions = append(plan.Actions, mcpJSONAction(*plan, target.Name))
		}
	}
	switch target.Support {
	case SupportSkillFirst:
		if writeConfig {
			plan.Warnings = append(plan.Warnings, fmt.Sprintf("%s: MCP config is emitted as JSON for manual copy in this MVP.", target.Name))
		} else {
			plan.Warnings = append(plan.Warnings, fmt.Sprintf("%s: no MCP JSON is emitted unless --write-config is passed.", target.Name))
		}
	case SupportProfile:
		plan.Warnings = append(plan.Warnings, fmt.Sprintf("%s: Trellis is a profile, not a separate runtime agent.", target.Name))
	}
}

func installSkillMessage(mcpMode bool) string {
	if mcpMode {
		return "Install SourceMux routing skill with MCP-aware routing because supported MCP config is requested."
	}
	return "Install SourceMux CLI-first routing skill."
}

func targetMCPMode(target Target, writeConfig bool) bool {
	if !writeConfig {
		return false
	}
	switch strings.TrimSpace(target.MCP) {
	case "", "none":
		return false
	default:
		return true
	}
}

func addUninstallActions(plan *Plan, target Target, scope string, writeConfig bool, force bool) {
	if !target.Skill {
		plan.Actions = append(plan.Actions, PlanAction{
			Type:    "note",
			Target:  target.Name,
			Status:  "informational",
			Message: "Print-only target has no generated skill file to remove.",
		})
		return
	}
	path, err := skillPath(target, scope)
	if err != nil {
		plan.Warnings = append(plan.Warnings, fmt.Sprintf("%s: %v", target.Name, err))
		return
	}
	plan.Actions = append(plan.Actions, PlanAction{
		Type:     "remove_file",
		Target:   target.Name,
		Path:     path,
		Manifest: manifestPath(path),
		Status:   "planned",
		Message:  "Remove SourceMux routing skill file if manifest and hash prove it is generated by SourceMux.",
	})
	if force {
		last := &plan.Actions[len(plan.Actions)-1]
		last.Backup = plannedBackupPath(path)
		last.Message = "Remove SourceMux-managed routing skill; if modified, back up the skill before removal."
	}
	if !writeConfig {
		plan.Warnings = append(plan.Warnings, fmt.Sprintf("%s: MCP client config is not removed unless --write-config is passed.", target.Name))
	}
}

func addConfigWriteAction(plan *Plan, target Target, scope string) error {
	action, ok, err := planConfigWriteAction(target.Name, scope, plan.Binary, plan.ConfigFile)
	if err != nil {
		return fmt.Errorf("%s MCP config merge: %w", target.Name, err)
	}
	if !ok {
		plan.Actions = append(plan.Actions, PlanAction{
			Type:    "note",
			Target:  target.Name,
			Status:  "informational",
			Message: "No verified safe MCP config writer exists for this target yet; no external agent CLI will be invoked.",
		})
		return nil
	}
	plan.Actions = append(plan.Actions, action)
	return nil
}

func addConfigRemoveAction(plan *Plan, target Target, scope string) error {
	action, ok, err := planConfigRemoveAction(target.Name, scope)
	if err != nil {
		return fmt.Errorf("%s MCP config removal: %w", target.Name, err)
	}
	if !ok {
		plan.Actions = append(plan.Actions, PlanAction{
			Type:    "note",
			Target:  target.Name,
			Status:  "informational",
			Message: "No verified safe MCP config remover exists for this target yet; no external agent CLI will be invoked.",
		})
		return nil
	}
	plan.Actions = append(plan.Actions, action)
	return nil
}

func statusFor(target Target, scope string, includeConfig bool, binary string, configPath string) (status TargetStatus) {
	status = TargetStatus{Name: target.Name, Support: target.Support, Scope: scope}
	defer func() {
		if includeConfig {
			cs := configStatusFor(target.Name, scope, binary, configPath)
			status.ConfigStatus = &cs
		}
	}()
	if !target.Skill {
		status.Notes = append(status.Notes, "print-only target")
		return status
	}
	path, err := skillPath(target, scope)
	if err != nil {
		status.Notes = append(status.Notes, err.Error())
		return status
	}
	status.SkillPath = path
	status.ManifestPath = manifestPath(path)
	skillData, skillErr := os.ReadFile(path)
	manifest, manifestErr := readManifest(status.ManifestPath)
	if skillErr == nil {
		status.Installed = true
	} else if !errors.Is(skillErr, os.ErrNotExist) {
		status.Notes = append(status.Notes, skillErr.Error())
	}
	if manifestErr == nil {
		status.Managed = true
		if manifest.MCPMode {
			status.InstallMode = "mcp-configured"
		} else {
			status.InstallMode = "cli-only"
		}
		if manifest.Target != target.Name {
			status.Notes = append(status.Notes, fmt.Sprintf("manifest target is %q", manifest.Target))
		}
		if scopeStatus, issue := manifestScopeStatus(target, scope, path, manifest.SkillPath); scopeStatus != nil {
			status.ScopeStatus = scopeStatus
			if issue != nil {
				status.Issues = append(status.Issues, *issue)
			}
		}
		binaryStatus, binaryIssues := binaryPathStatus(target.Name, scope, manifest.Binary, binary)
		status.BinaryStatus = &binaryStatus
		status.Issues = append(status.Issues, binaryIssues...)
		if includeConfig {
			configFileStatus, configIssues := runtimeConfigPathStatus(target.Name, scope, manifest.ConfigFile, configPath)
			status.RuntimeConfigStatus = &configFileStatus
			status.Issues = append(status.Issues, configIssues...)
		}
		if status.Installed {
			currentHash := contentSHA256(skillData)
			status.Modified = currentHash != manifest.ContentSHA256
			if status.Modified {
				status.Notes = append(status.Notes, "skill content differs from SourceMux manifest hash")
			}
		} else {
			status.Notes = append(status.Notes, "manifest exists but skill file is missing")
		}
	} else if errors.Is(manifestErr, os.ErrNotExist) {
		if status.Installed {
			status.InstallMode = "unmanaged"
			status.Notes = append(status.Notes, "skill file exists without SourceMux manifest; uninstall will refuse to remove it")
		}
	} else {
		status.Notes = append(status.Notes, manifestErr.Error())
	}
	if !status.Installed {
		if scopeStatus, issue := wrongScopeStatus(target, scope); scopeStatus != nil {
			status.ScopeStatus = scopeStatus
			status.Issues = append(status.Issues, *issue)
		}
	}
	return status
}

func binaryPathStatus(target, scope, path, expected string) (PathStatus, []StatusIssue) {
	status := PathStatus{
		Path:            strings.TrimSpace(path),
		Expected:        strings.TrimSpace(expected),
		Status:          "ok",
		MatchesExpected: samePath(path, expected),
	}
	repair := binaryRepair(target, scope)
	if status.Path == "" {
		status.Status = "missing"
		status.Message = "Generated skill manifest does not record a SourceMux binary path."
		return status, []StatusIssue{newStatusIssue("missing_binary", status.Message, "", expected, repair)}
	}
	exists, err := regularFileExists(status.Path)
	if err != nil {
		status.Status = "error"
		status.Message = err.Error()
		return status, []StatusIssue{newStatusIssue("binary_path_error", status.Message, status.Path, expected, repair)}
	}
	status.Exists = exists
	switch {
	case !exists:
		status.Status = "missing"
		status.Message = fmt.Sprintf("Generated skill points at missing SourceMux binary path %s.", status.Path)
		return status, []StatusIssue{newStatusIssue("missing_binary", status.Message, status.Path, expected, repair)}
	case looksTemporaryExecutable(status.Path):
		status.Status = "stale"
		status.Message = fmt.Sprintf("Generated skill points at temporary SourceMux binary path %s.", status.Path)
		return status, []StatusIssue{newStatusIssue("stale_binary", status.Message, status.Path, expected, repair)}
	case status.Expected != "" && !status.MatchesExpected:
		status.Status = "stale"
		status.Message = fmt.Sprintf("Generated skill binary path %s differs from the requested/current SourceMux binary %s.", status.Path, status.Expected)
		return status, []StatusIssue{newStatusIssue("stale_binary", status.Message, status.Path, expected, repair)}
	default:
		status.Message = "Generated skill binary path exists and matches the requested/current SourceMux binary."
		return status, nil
	}
}

func runtimeConfigPathStatus(target, scope, path, expected string) (PathStatus, []StatusIssue) {
	status := PathStatus{
		Path:            strings.TrimSpace(path),
		Expected:        strings.TrimSpace(expected),
		Status:          "ok",
		MatchesExpected: samePath(path, expected),
	}
	repair := configRepair(target, scope, expected)
	if status.Path == "" {
		status.Status = "missing"
		status.Message = "Generated skill manifest does not record a SourceMux config path."
		return status, []StatusIssue{newStatusIssue("missing_config", status.Message, "", expected, repair)}
	}
	exists, err := regularFileExists(status.Path)
	if err != nil {
		status.Status = "error"
		status.Message = err.Error()
		return status, []StatusIssue{newStatusIssue("config_path_error", status.Message, status.Path, expected, repair)}
	}
	status.Exists = exists
	switch {
	case !exists:
		status.Status = "missing"
		status.Message = fmt.Sprintf("Generated skill points at missing SourceMux config path %s.", status.Path)
		return status, []StatusIssue{newStatusIssue("missing_config", status.Message, status.Path, expected, repair)}
	case status.Expected != "" && !status.MatchesExpected:
		status.Status = "stale"
		status.Message = fmt.Sprintf("Generated skill config path %s differs from the requested scope/config path %s.", status.Path, status.Expected)
		return status, []StatusIssue{newStatusIssue("stale_config", status.Message, status.Path, expected, repair)}
	default:
		status.Message = "Generated skill config path exists and matches the requested scope/config path."
		return status, nil
	}
}

func manifestScopeStatus(target Target, requestedScope, currentSkillPath, manifestSkillPath string) (*ScopeStatus, *StatusIssue) {
	manifestSkillPath = strings.TrimSpace(manifestSkillPath)
	if manifestSkillPath == "" || samePath(manifestSkillPath, currentSkillPath) {
		return nil, nil
	}
	detectedScope := scopeForSkillPath(target, manifestSkillPath)
	statusValue := "stale"
	code := "stale_skill_path"
	message := fmt.Sprintf("Generated skill manifest path %s differs from requested %s-scope path %s.", manifestSkillPath, requestedScope, currentSkillPath)
	if detectedScope != "" && detectedScope != requestedScope {
		statusValue = "wrong_scope"
		code = "wrong_scope"
		message = fmt.Sprintf("Generated skill manifest was created for %s scope, but status is checking %s scope.", detectedScope, requestedScope)
	}
	scopeStatus := &ScopeStatus{
		Requested: requestedScope,
		Detected:  detectedScope,
		SkillPath: manifestSkillPath,
		Status:    statusValue,
		Message:   message,
	}
	issue := newStatusIssue(code, message, manifestSkillPath, currentSkillPath, scopeRepair(target.Name, requestedScope, detectedScope))
	return scopeStatus, &issue
}

func wrongScopeStatus(target Target, requestedScope string) (*ScopeStatus, *StatusIssue) {
	otherScope := oppositeScope(requestedScope)
	if otherScope == "" {
		return nil, nil
	}
	otherPath, err := skillPath(target, otherScope)
	if err != nil {
		return nil, nil
	}
	exists, err := regularFileExists(otherPath)
	if err != nil || !exists {
		return nil, nil
	}
	message := fmt.Sprintf("%s is not installed for %s scope, but a %s-scope skill exists at %s.", target.Name, requestedScope, otherScope, otherPath)
	scopeStatus := &ScopeStatus{
		Requested: requestedScope,
		Detected:  otherScope,
		SkillPath: otherPath,
		Status:    "wrong_scope",
		Message:   message,
	}
	issue := newStatusIssue("wrong_scope", message, otherPath, "", scopeRepair(target.Name, requestedScope, otherScope))
	return scopeStatus, &issue
}

func oppositeScope(scope string) string {
	switch scope {
	case "project":
		return "user"
	case "user":
		return "project"
	default:
		return ""
	}
}

func scopeForSkillPath(target Target, path string) string {
	for _, scope := range []string{"project", "user"} {
		scopePath, err := skillPath(target, scope)
		if err == nil && samePath(path, scopePath) {
			return scope
		}
	}
	return ""
}

func newStatusIssue(code, message, path, expected, repair string) StatusIssue {
	return StatusIssue{
		Code:     code,
		Severity: "warning",
		Message:  message,
		Path:     strings.TrimSpace(path),
		Expected: strings.TrimSpace(expected),
		Repair:   strings.TrimSpace(repair),
	}
}

func binaryRepair(target, scope string) string {
	return fmt.Sprintf("Run sourcemux bootstrap update %s --scope %s --binary /absolute/path/to/sourcemux and keep the configured --config path.", target, scope)
}

func configRepair(target, scope, expected string) string {
	if strings.TrimSpace(expected) == "" {
		return fmt.Sprintf("Run sourcemux bootstrap status %s --scope %s --config-status with the intended explicit --config path, then update the skill.", target, scope)
	}
	return fmt.Sprintf("Create %s or run sourcemux bootstrap update %s --scope %s --config %s; do not invent a hidden config fallback.", shellQuote(expected), target, scope, shellQuote(expected))
}

func scopeRepair(target, requestedScope, detectedScope string) string {
	if detectedScope == "" {
		return fmt.Sprintf("Reinstall or update %s with --scope %s so the generated skill path matches the requested scope.", target, requestedScope)
	}
	return fmt.Sprintf("Run sourcemux bootstrap status %s --scope %s --config-status to inspect the existing install, or reinstall/update with --scope %s.", target, detectedScope, requestedScope)
}

func regularFileExists(path string) (bool, error) {
	if strings.TrimSpace(path) == "" {
		return false, nil
	}
	info, err := os.Stat(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return false, nil
		}
		return false, err
	}
	if info.IsDir() {
		return false, fmt.Errorf("%s is a directory, not a file", path)
	}
	return true, nil
}

func samePath(a, b string) bool {
	a = comparablePath(a)
	b = comparablePath(b)
	return a != "" && b != "" && a == b
}

func comparablePath(path string) string {
	path = strings.TrimSpace(path)
	if path == "" {
		return ""
	}
	if resolved, err := filepath.EvalSymlinks(path); err == nil {
		path = resolved
	}
	if abs, err := filepath.Abs(path); err == nil {
		path = abs
	}
	return filepath.Clean(path)
}

func skillPath(target Target, scope string) (string, error) {
	root := target.ProjectRoot
	if scope == "user" {
		root = target.UserRoot
	}
	if root == "" {
		return "", fmt.Errorf("target has no skill path for scope %s", scope)
	}
	root, err := expandPath(root)
	if err != nil {
		return "", err
	}
	return filepath.Join(root, skillName, "SKILL.md"), nil
}

func expandPath(path string) (string, error) {
	if path == "~" || strings.HasPrefix(path, "~/") {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		if path == "~" {
			return home, nil
		}
		path = filepath.Join(home, strings.TrimPrefix(path, "~/"))
	}
	if filepath.IsAbs(path) {
		return filepath.Clean(path), nil
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	return abs, nil
}

func executablePath() (string, error) {
	exe, err := os.Executable()
	if err == nil && strings.TrimSpace(exe) != "" {
		if abs, absErr := filepath.Abs(exe); absErr == nil {
			return abs, nil
		}
		return exe, nil
	}
	if len(os.Args) > 0 && strings.TrimSpace(os.Args[0]) != "" {
		return filepath.Abs(os.Args[0])
	}
	return "", fmt.Errorf("cannot determine sourcemux executable path")
}

func resolveBinaryPath(path string) (string, error) {
	if strings.TrimSpace(path) == "" {
		return executablePath()
	}
	return expandPath(path)
}

func resolveConfigPath(scope, path string) (string, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		path = defaultConfigPathForScope(scope)
	}
	return expandPath(path)
}

func defaultConfigPathForScope(scope string) string {
	if scope == "user" {
		return defaultUserConfigPath
	}
	return config.DefaultConfigPath()
}

func looksTemporaryExecutable(path string) bool {
	cleaned := filepath.ToSlash(filepath.Clean(path))
	return strings.Contains(cleaned, "/go-build") ||
		strings.Contains(cleaned, "/T/go-build") ||
		strings.HasSuffix(filepath.Base(cleaned), ".test")
}

func mcpJSONAction(plan Plan, target string) PlanAction {
	return PlanAction{
		Type:    "mcp_json",
		Target:  target,
		Status:  "informational",
		MCPJSON: mcpJSON(plan.Binary, plan.ConfigFile),
		Message: "Copy this MCP JSON into clients that do not have a verified automatic writer yet.",
	}
}

func codexCommandAction(plan Plan) PlanAction {
	return PlanAction{
		Type:    "shell_command",
		Target:  "codex",
		Status:  "informational",
		Command: []string{"codex", "mcp", "add", "sourcemux", "--", plan.Binary, "--config", plan.ConfigFile},
		Message: "Codex CLI command for adding the SourceMux stdio MCP server.",
	}
}

func codexConfigSnippetAction(plan Plan, scope string) PlanAction {
	path := "~/.codex/config.toml"
	if scope == "project" {
		path = ".codex/config.toml"
	}
	return PlanAction{
		Type:    "config_snippet",
		Target:  "codex",
		Path:    path,
		Status:  "informational",
		Snippet: codexTOMLSnippet(plan.Binary, plan.ConfigFile),
		Message: "Codex config.toml snippet for exact scope-controlled setup.",
	}
}

func claudeCommandAction(plan Plan, scope string) PlanAction {
	return PlanAction{
		Type:    "shell_command",
		Target:  "claude-code",
		Status:  "informational",
		Command: []string{"claude", "mcp", "add", "--transport", "stdio", "--scope", scope, "sourcemux", "--", plan.Binary, "--config", plan.ConfigFile},
		Message: "Claude Code command for adding the SourceMux stdio MCP server.",
	}
}

func geminiCommandAction(plan Plan, scope string) PlanAction {
	return PlanAction{
		Type:    "shell_command",
		Target:  "gemini",
		Status:  "informational",
		Command: []string{"gemini", "mcp", "add", "--scope", scope, "sourcemux", plan.Binary, "--", "--config", plan.ConfigFile},
		Message: "Gemini CLI command for adding the SourceMux stdio MCP server.",
	}
}

func geminiConfigSnippetAction(plan Plan, scope string) PlanAction {
	path := "~/.gemini/settings.json"
	if scope == "project" {
		path = ".gemini/settings.json"
	}
	return PlanAction{
		Type:    "config_snippet",
		Target:  "gemini",
		Path:    path,
		Status:  "informational",
		Snippet: geminiSettingsSnippet(plan.Binary, plan.ConfigFile),
		Message: "Gemini settings.json snippet for manual merge.",
	}
}

func opencodeConfigSnippetAction(plan Plan, scope string) PlanAction {
	path := "~/.config/opencode/opencode.json"
	if scope == "project" {
		path = "opencode.json"
	}
	return PlanAction{
		Type:    "config_snippet",
		Target:  "opencode",
		Path:    path,
		Status:  "informational",
		Snippet: opencodeConfigSnippet(plan.Binary, plan.ConfigFile),
		Message: "OpenCode JSON/JSONC config snippet for manual merge.",
	}
}

func stdioAction(plan Plan, target string) PlanAction {
	return PlanAction{
		Type:    "stdio",
		Target:  target,
		Status:  "informational",
		Command: []string{plan.Binary, "--config", plan.ConfigFile},
		Message: "Use this command as the SourceMux stdio MCP server.",
	}
}

func mcpJSON(binary, configPath string) string {
	payload := map[string]any{
		"mcpServers": map[string]any{
			"sourcemux": map[string]any{
				"command": binary,
				"args":    []string{"--config", configPath},
			},
		},
	}
	data, _ := json.MarshalIndent(payload, "", "  ")
	return string(data)
}

func codexTOMLSnippet(binary, configPath string) string {
	return fmt.Sprintf(`[mcp_servers.sourcemux]
command = %q
args = [%q, %q]
`, binary, "--config", configPath)
}

func geminiSettingsSnippet(binary, configPath string) string {
	payload := map[string]any{
		"mcpServers": map[string]any{
			"sourcemux": map[string]any{
				"command": binary,
				"args":    []string{"--config", configPath},
			},
		},
	}
	return marshalSnippet(payload)
}

func opencodeConfigSnippet(binary, configPath string) string {
	payload := map[string]any{
		"$schema": "https://opencode.ai/config.json",
		"mcp": map[string]any{
			"sourcemux": map[string]any{
				"type":    "local",
				"command": []string{binary, "--config", configPath},
				"enabled": true,
			},
		},
	}
	return marshalSnippet(payload)
}

func marshalSnippet(payload any) string {
	data, _ := json.MarshalIndent(payload, "", "  ")
	return string(data)
}

func routingSkill(binary, configPath, scope string, mcpMode bool) string {
	configFlag := "--config " + shellQuote(configPath)
	commandPrefix := shellQuote(binary) + " " + configFlag
	searchPolicy, policyNote := routingSearchPolicy(configPath)
	agentProfile := searchPolicy.AgentProfile
	fallbackAfter := fmt.Sprintf("%ds", searchPolicy.FallbackAfterSec)
	callerTimeout := fmt.Sprintf("%ds", searchPolicy.TimeoutSec)
	quickSearch := fmt.Sprintf(`%s search "query" --profile %s --fallback-after %s --timeout %s --json`, commandPrefix, agentProfile, fallbackAfter, callerTimeout)
	twitterSearch := fmt.Sprintf(`%s search "query" --platform Twitter --profile %s --fallback-after %s --timeout %s --json`, commandPrefix, agentProfile, fallbackAfter, callerTimeout)
	heavySearch := fmt.Sprintf(`%s search "query" --profile heavy --fallback-after %s --timeout %s --json`, commandPrefix, fallbackAfter, callerTimeout)
	complexHeavySearch := fmt.Sprintf(`%s search "complex query" --profile heavy --fallback-after %s --timeout %s --json`, commandPrefix, fallbackAfter, callerTimeout)
	policySummary := fmt.Sprintf(`- Effective searchPolicy: defaultProfile=%s, agentProfile=%s, autoPreference=%s, fallbackAfterSec=%d, timeoutSec=%d.
- %s
- Generated quick search examples use --profile %s --fallback-after %s --timeout %s. Explicit --profile, --fallback-after, --grok-pool-timeout, and --timeout always override this policy.`,
		searchPolicy.DefaultProfile,
		searchPolicy.AgentProfile,
		searchPolicy.AutoPreference,
		searchPolicy.FallbackAfterSec,
		searchPolicy.TimeoutSec,
		policyNote,
		agentProfile,
		fallbackAfter,
		callerTimeout,
	)
	modeLabel := "project development mode"
	if scope == "user" {
		modeLabel = "public user mode"
	}
	deepIntentLabels := "Deep search, \u6df1\u5ea6\u641c\u7d22, deep research, \u6df1\u5ea6\u8c03\u7814, complex comparison, \u590d\u6742\u5bf9\u6bd4, or verification/\u6838\u9a8c"
	cliPolicy := `- Treat this skill as the routing/decision layer and the SourceMux CLI as the execution layer.
- Use the SourceMux CLI by default for search, fetch, docs search, research, source verification, URL mapping, and saved artifacts.
- Every SourceMux CLI command must include the configured --config path shown below.
- Keep fetched content compact; summarize instead of pasting full pages unless explicitly requested.
- User-facing research/search must preserve fallback. Do not use --no-fallback unless the user explicitly asks to diagnose a Grok/profile/endpoint or you are doing a clearly labeled diagnostic probe.
- --grok-pool-timeout 0 --no-fallback is diagnostics-only. Never use it for broad/current research, source discovery, project lists, citations, or answering the user's substantive question.
- Use direct provider commands only when the capability rules below call for them; otherwise honor SourceMux policy-first routing unless the user explicitly asks.
- Ordinary known URLs use SourceMux fetch --profile auto. Default fetch is policy-first / quality-first: GitHub URLs use repository-aware routing, ordinary pages prefer Firecrawl when configured, and cheap/zero-key requests use --profile cheap.
- Do not call Jina directly unless the user explicitly asks for cheap, zero-key, or diagnostic mode. Use SourceMux fetch --profile cheap for that route.
- Use firecrawl-scrape as an explicit one-off hard-page command only when the user asks for Firecrawl-specific controls; ordinary hard pages should start with fetch --profile auto.
- Use firecrawl-map only for site structure discovery, URL inventory, or relevance-filtered URL discovery; do not use it for ordinary single-page extraction.
- Do not install, configure, or call Firecrawl MCP. Firecrawl is available through SourceMux CLI direct commands and ordinary SourceMux fetch routing when configured.
- Follow the effective searchPolicy below for generated one-shot search defaults. Use --profile default when the user explicitly asks for a fast, low-cost, or lightweight search.
- Use --profile heavy only when the user asks to force heavy/multi-agent search or when diagnosing whether heavy is configured.
- Multi-agent search models must be configured in grokEndpoints[] with a search profile such as heavy; reasoningEndpoints[] alone is only for final synthesis.
- Do not assume any specific endpoint name; rely on the active sourcemux.json.
- Never print API keys, provider dashboard exports, private endpoints, or local credential files.`
	if mcpMode {
		cliPolicy = `- Treat this skill as the routing/decision layer and SourceMux tools/CLI as execution surfaces.
- Use SourceMux MCP tools for quick interactive search, fetch, docs search, source verification, URL mapping, and compact research.
- Use the SourceMux CLI for deep research, reproducible JSON, large outputs, shell/script chaining, or saved artifacts.
- Every SourceMux CLI command must include the configured --config path shown below.
- Keep fetched content compact; summarize instead of pasting full pages unless explicitly requested.
- User-facing research/search must preserve fallback. Do not use --no-fallback unless the user explicitly asks to diagnose a Grok/profile/endpoint or you are doing a clearly labeled diagnostic probe.
- --grok-pool-timeout 0 --no-fallback is diagnostics-only. Never use it for broad/current research, source discovery, project lists, citations, or answering the user's substantive question.
- Use direct provider commands only when the capability rules below call for them; otherwise honor SourceMux policy-first routing unless the user explicitly asks.
- Ordinary known URLs use SourceMux fetch --profile auto. Default fetch is policy-first / quality-first: GitHub URLs use repository-aware routing, ordinary pages prefer Firecrawl when configured, and cheap/zero-key requests use --profile cheap.
- Do not call Jina directly unless the user explicitly asks for cheap, zero-key, or diagnostic mode. Use SourceMux fetch --profile cheap for that route.
- Use firecrawl-scrape as an explicit one-off hard-page command only when the user asks for Firecrawl-specific controls; ordinary hard pages should start with fetch --profile auto.
- Use firecrawl-map only for site structure discovery, URL inventory, or relevance-filtered URL discovery; do not use it for ordinary single-page extraction.
- Do not install, configure, or call Firecrawl MCP. Firecrawl is available through SourceMux CLI direct commands and ordinary SourceMux fetch routing when configured.
- Follow the effective searchPolicy below for generated one-shot search defaults. Use --profile default when the user explicitly asks for a fast, low-cost, or lightweight search.
- Use --profile heavy only when the user asks to force heavy/multi-agent search or when diagnosing whether heavy is configured.
- Multi-agent search models must be configured in grokEndpoints[] with a search profile such as heavy; reasoningEndpoints[] alone is only for final synthesis.
- Do not assume any specific endpoint name; rely on the active sourcemux.json.
- Never print API keys, provider dashboard exports, private endpoints, or local credential files.`
	}
	return fmt.Sprintf(`---
name: sourcemux-routing
description: Route web search, research, source fetching, docs lookup, and SourceMux profile/model selection through SourceMux; includes strict guardrails for default vs heavy vs diagnostics-only no-fallback routing.
---

# SourceMux routing

Use SourceMux as the default web research capability.

## Routing policy

%s

## Effective searchPolicy

%s

## Mode selection

Choose one mode before running commands:

| Mode | Use when | Command pattern |
| --- | --- | --- |
| Quick search | Fresh/current facts, community feedback, one-hop discovery | %s |
| Broad research | Project lists, comparisons, current source discovery, citation-heavy work | %s research "topic" --depth standard --profile auto --json |
| Deep planning | %s where decomposition is useful before execution | %s plan "topic" --json --depth deep |
| Deep evidence | Same as broad research, but user asks for deeper/stronger coverage | %s research "topic" --depth deep --profile auto --json |
| Explicit heavy search | User asks to use heavy/multi-agent search directly | %s |
| Final synthesis | Evidence is collected and the user wants an answer/plan | %s smart-answer "question" --profile auto --json |
| Diagnostics | User asks whether Grok/heavy/profile/endpoint itself works | %s search "ping" --profile heavy --grok-pool-timeout 0 --no-fallback --timeout 120s --json |

If a normal search returns a fallback engine such as Exa, TinyFish, or Tavily, treat that as a valid source-discovery result. Fetch key URLs next; do not rerun with --no-fallback unless the user is debugging the Grok profile itself.

## Profile policy

| Profile | Intended use | Notes |
| --- | --- | --- |
| auto | Agent/search/research default | Resolves according to searchPolicy.autoPreference: intent-based, heavy-first, or default-first; safely falls back to default when heavy is unavailable. |
| heavy | Explicit multi-agent search | Use --profile heavy when the user asks to force heavy or to fail if heavy is not configured; keep fallback available with --fallback-after for user-facing search. |
| default | Fast/lightweight search | Raw search uses this when searchPolicy.defaultProfile=default, and agents may use it when the user explicitly asks for quick, low-cost, or non-heavy search. |

## Capability routing

| User intent | Prefer | Why |
| --- | --- | --- |
| Fresh/current topics, community feedback, X/Twitter, controversy, release reaction | %s or %s | Grok search with configured policy is the freshness/community-first route and preserves SourceMux fallback tracing. |
| Official docs, SDK/API reference, product docs, pricing pages, low-SEO-noise discovery | %s docs-search "library or API question" --json | Uses the configured source-first docs search path. |
| Exa-specific deep/source discovery, structured output, text snippets, or low-noise source search | %s exa-search "official docs API reference" --type deep --json | Calls Exa directly when Exa-specific controls matter. |
| Known URL page extraction | %s fetch "https://example.com" --profile auto --json | Uses SourceMux policy-first fetch: GitHub-aware for repo URLs, Firecrawl-first quality routing for ordinary pages when configured, then provider fallbacks. |
| Cheap or zero-key known URL extraction | %s fetch "https://example.com" --profile cheap --json | Uses the low-cost route: Jina -> Firecrawl -> Exa -> Tavily. |
| Difficult known URL extraction with Firecrawl-specific controls | %s firecrawl-scrape "https://example.com" --json | Use only as an explicit direct command when Firecrawl scrape flags matter; this direct command does not use Firecrawl MCP. |
| Known URL plus Exa contents controls, subpages, or API/documentation subtree discovery | %s exa-contents "https://example.com/docs" --subpages 3 --subpage-target api --json | Uses Exa Contents directly for URL-centered extraction and subpage discovery. |
| Site structure discovery for hard sites, URL inventory, or relevance-filtered sections | %s firecrawl-map "https://example.com" --search "docs" --limit 100 --json | Use for URL inventory and site structure, not ordinary page extraction; existing SourceMux map remains Tavily. |
| Explicit slow heavy or multi-agent Grok search | %s | Lets Grok try first, then preserves fallback results for the user's actual task. |
| Grok/profile diagnostics | %s search "ping" --profile heavy --grok-pool-timeout 0 --no-fallback --timeout 120s --json | Diagnostics-only path to verify whether the selected Grok profile itself can return. |
| %s where decomposition helps | %s plan "topic" --json --depth deep, then %s research "topic" --depth deep --profile auto --json | The offline structured planner decides SourceMux-capability steps first; research executes with profile=auto and preserved fallback. |
| Multi-source investigation with synthesis | %s research "topic" --depth standard --profile auto --json or %s research "topic" --depth deep --profile auto --json | Runs the composable SourceMux research workflow. Auto uses heavy/multi-agent search when configured and appropriate, while preserving fallback. |
| Planning/decomposition without executing the research | %s plan "topic" --json --depth standard or %s plan "topic" --json --depth deep | Produces a deterministic structured plan before running provider calls. Text output remains available with plan --depth. |

## Diagnostics workflow

Use this only when the user is asking why endpoints/profile/model behavior failed:

1. Inspect redacted profile metadata, never secrets:
   jq '{grokEndpoints: [.grokEndpoints[]? | {name, model, profile}], reasoningEndpoints: [.reasoningEndpoints[]? | {name, model, profile}]}' %s
2. Run a short probe, not the user's broad research query:
   %s search "ping" --profile heavy --grok-pool-timeout 0 --no-fallback --timeout 120s --json
3. Interpret results:
   - probe succeeds, broad query times out: endpoint is reachable; use --fallback-after or research.
   - probe fails: report the grok_error/route_decision and do not hide it with fallback.
   - fallback engine returns content: use it for source discovery unless the user is specifically debugging Grok.

## Evidence policy

- For source-critical claims, do not rely on a search summary alone.
- First discover candidate URLs with search, docs-search, exa-search, or research.
- Then fetch 1-3 key URLs with fetch --profile auto --json before making high-risk or precise claims.
- For known URLs, use fetch --profile auto first. This is SourceMux policy-first: GitHub URLs route through repository-aware enrichment first, ordinary pages prefer quality extraction, and cheap/zero-key requests must explicitly use --profile cheap.
- Do not call Jina directly unless the user asks for cheap, zero-key, or diagnostic mode.
- Use firecrawl-scrape only when the user needs explicit Firecrawl scrape controls.
- For site URL inventory or section discovery, use firecrawl-map explicitly.
- In final answers, cite fetched or source URL evidence and mention the engine/provider when it matters.
- A fetch result may show a provider such as Firecrawl, GitHub Provider, Jina Reader, Exa, or Tavily; that verifies URL content and does not mean that provider performed the original search.

## Public user mode vs project development mode

This generated skill was installed for scope: %s (%s).

- Public user mode: bootstrap --scope user defaults the generated config path to ~/.config/sourcemux/sourcemux.json. User-scope skills should point at an installed SourceMux binary and a user config file, not at a maintainer's source checkout.
- Project development mode: bootstrap --scope project defaults the generated config path to ./sourcemux.json. Project-scope skills may intentionally point at a source checkout binary/config for local development.
- Explicit --config always wins. If the user intentionally installed a different config path, keep using the configured path shown below.

The binary and config paths below are authoritative for this installed copy. If either path is missing, stale, points at a temporary Go build artifact, or does not match the intended scope, do not invent replacement paths and do not silently switch user-scope work to a maintainer-local project path. Run the equivalent of:

%s bootstrap status --scope %s --config-status
%s bootstrap update <target> --scope %s --binary /absolute/path/to/sourcemux

If the binary path itself is missing, replace only the command binary with a known working sourcemux executable; keep the configured --config path shown here.

## Local installation

- Binary: %s
- Config: %s

## CLI examples

%s
%s
%s
%s fetch "https://example.com" --profile auto --json
%s fetch "https://example.com" --profile cheap --json
%s firecrawl-scrape "https://example.com" --json
%s firecrawl-map "https://example.com" --search "docs" --limit 100 --json
%s docs-search "library or API question" --json
%s exa-search "official docs API reference" --type deep --json
%s exa-contents "https://example.com/docs" --subpages 3 --subpage-target api --json
%s plan "research question" --depth standard
%s plan "deep research question" --json --depth deep
%s research "topic" --depth standard --profile auto --json
%s research "topic" --depth deep --profile auto --json
%s smart-answer "complex research question" --profile auto --json

Diagnostics only; do not use for user-facing research answers:

%s search "ping" --profile heavy --grok-pool-timeout 0 --no-fallback --timeout 120s --json
`, cliPolicy,
		policySummary,
		quickSearch, commandPrefix, deepIntentLabels, commandPrefix, commandPrefix, heavySearch, commandPrefix, commandPrefix,
		twitterSearch, quickSearch, commandPrefix, commandPrefix, commandPrefix, commandPrefix, commandPrefix, commandPrefix, commandPrefix, heavySearch, commandPrefix, deepIntentLabels, commandPrefix, commandPrefix, commandPrefix, commandPrefix, commandPrefix, commandPrefix,
		shellQuote(configPath), commandPrefix,
		scope, modeLabel, commandPrefix, scope, commandPrefix, scope, binary, configPath,
		quickSearch, twitterSearch, complexHeavySearch, commandPrefix, commandPrefix, commandPrefix, commandPrefix, commandPrefix, commandPrefix, commandPrefix, commandPrefix, commandPrefix, commandPrefix, commandPrefix, commandPrefix, commandPrefix)
}

func routingSearchPolicy(configPath string) (config.SearchPolicy, string) {
	policy := config.DefaultSearchPolicy()
	if strings.TrimSpace(configPath) == "" {
		return policy, "No config path was provided; generated guidance is using public-safe searchPolicy defaults."
	}
	cfg, err := config.LoadFile(expandTilde(configPath))
	if err != nil {
		return policy, fmt.Sprintf("Could not load %s while generating this skill (%v); generated guidance is using public-safe searchPolicy defaults.", configPath, err)
	}
	return cfg.SearchPolicy, fmt.Sprintf("Loaded searchPolicy from %s at generation time.", configPath)
}

func expandTilde(path string) string {
	path = strings.TrimSpace(path)
	if path == "~" {
		if home, err := os.UserHomeDir(); err == nil && home != "" {
			return home
		}
		return path
	}
	if strings.HasPrefix(path, "~/") {
		if home, err := os.UserHomeDir(); err == nil && home != "" {
			return filepath.Join(home, strings.TrimPrefix(path, "~/"))
		}
	}
	return path
}

func writeGeneratedSkill(path string, content []byte, force bool, manifest installManifest) (string, string, error) {
	status := "created"
	if existing, err := os.ReadFile(path); err == nil {
		if string(existing) == string(content) {
			status = "unchanged"
			if err := writeManifest(manifestPath(path), manifest); err != nil {
				return "", "", err
			}
			return status, "", nil
		}
		oldManifest, manifestErr := readManifest(manifestPath(path))
		if manifestErr == nil && oldManifest.Target == manifest.Target && contentSHA256(existing) == oldManifest.ContentSHA256 {
			if err := os.WriteFile(path, content, 0o644); err != nil {
				return "", "", err
			}
			if err := writeManifest(manifestPath(path), manifest); err != nil {
				return "", "", err
			}
			return "updated", "", nil
		}
		if !force {
			return "", "", fmt.Errorf("%s already exists with different content; re-run with --force to back up and replace", path)
		}
		backup := plannedBackupPath(path)
		if err := os.Rename(path, backup); err != nil {
			return "", "", fmt.Errorf("backup %s: %w", path, err)
		}
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			return "", "", err
		}
		if err := os.WriteFile(path, content, 0o644); err != nil {
			return "", "", err
		}
		if err := writeManifest(manifestPath(path), manifest); err != nil {
			return "", "", err
		}
		return "replaced", backup, nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return "", "", err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return "", "", err
	}
	if err := os.WriteFile(path, content, 0o644); err != nil {
		return "", "", err
	}
	if err := writeManifest(manifestPath(path), manifest); err != nil {
		return "", "", err
	}
	return status, "", nil
}

func removeGeneratedSkill(path, target string, force bool) (string, string, error) {
	manifest, err := readManifest(manifestPath(path))
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			if _, statErr := os.Stat(path); errors.Is(statErr, os.ErrNotExist) {
				return "missing", "", nil
			}
			if force {
				backup := plannedBackupPath(path)
				if err := os.Rename(path, backup); err != nil {
					return "", "", fmt.Errorf("backup %s: %w", path, err)
				}
				_ = os.Remove(filepath.Dir(path))
				return "removed-with-backup", backup, nil
			}
			return "", "", fmt.Errorf("refusing to remove %s without SourceMux manifest", path)
		}
		return "", "", err
	}
	if manifest.Target != target {
		return "", "", fmt.Errorf("refusing to remove %s: manifest target %q does not match %q", path, manifest.Target, target)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			if rmErr := os.Remove(manifestPath(path)); rmErr != nil && !errors.Is(rmErr, os.ErrNotExist) {
				return "", "", rmErr
			}
			return "manifest-removed", "", nil
		}
		return "", "", err
	}
	if got := contentSHA256(data); got != manifest.ContentSHA256 {
		if !force {
			return "", "", fmt.Errorf("refusing to remove modified generated skill %s: manifest hash mismatch", path)
		}
		backup := plannedBackupPath(path)
		if err := os.Rename(path, backup); err != nil {
			return "", "", fmt.Errorf("backup %s: %w", path, err)
		}
		if err := os.Remove(manifestPath(path)); err != nil && !errors.Is(err, os.ErrNotExist) {
			return "", "", err
		}
		_ = os.Remove(filepath.Dir(path))
		return "removed-with-backup", backup, nil
	}
	if err := os.Remove(path); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return "missing", "", nil
		}
		return "", "", err
	}
	if err := os.Remove(manifestPath(path)); err != nil && !errors.Is(err, os.ErrNotExist) {
		return "", "", err
	}
	_ = os.Remove(filepath.Dir(path))
	return "removed", "", nil
}

func writeManifest(path string, manifest installManifest) error {
	data, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}

func readManifest(path string) (installManifest, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return installManifest{}, err
	}
	var manifest installManifest
	if err := json.Unmarshal(data, &manifest); err != nil {
		return installManifest{}, fmt.Errorf("read manifest %s: %w", path, err)
	}
	return manifest, nil
}

func manifestPath(skillPath string) string {
	return filepath.Join(filepath.Dir(skillPath), manifestName)
}

func contentSHA256(content []byte) string {
	sum := sha256.Sum256(content)
	return fmt.Sprintf("%x", sum[:])
}

func printPlan(plan Plan, asJSON bool) {
	if asJSON {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		_ = enc.Encode(plan)
		return
	}
	fmt.Fprintf(os.Stdout, "%s plan (scope=%s config=%s)\n", plan.Mode, plan.Scope, plan.ConfigFile)
	for _, action := range plan.Actions {
		status := action.Status
		if plan.DryRun && status == "planned" {
			status = "would-change"
		}
		switch action.Type {
		case "write_file", "remove_file":
			fmt.Fprintf(os.Stdout, "- %s %s %s: %s\n", action.Target, action.Type, status, action.Path)
			if action.Manifest != "" {
				fmt.Fprintf(os.Stdout, "  manifest: %s\n", action.Manifest)
			}
			if action.Backup != "" {
				fmt.Fprintf(os.Stdout, "  backup: %s\n", action.Backup)
			}
		case "merge_config", "remove_config":
			fmt.Fprintf(os.Stdout, "- %s %s %s: %s\n", action.Target, action.Type, status, action.Path)
			if action.Backup != "" {
				fmt.Fprintf(os.Stdout, "  backup: %s\n", action.Backup)
			}
			if action.Message != "" {
				fmt.Fprintf(os.Stdout, "  %s\n", action.Message)
			}
		case "mcp_json":
			fmt.Fprintf(os.Stdout, "- %s MCP JSON (%s):\n%s\n", action.Target, status, action.MCPJSON)
		case "shell_command":
			fmt.Fprintf(os.Stdout, "- %s command (%s): %s\n", action.Target, status, shellJoin(action.Command))
		case "config_snippet":
			fmt.Fprintf(os.Stdout, "- %s config snippet for %s (%s):\n%s\n", action.Target, action.Path, status, action.Snippet)
		case "stdio":
			fmt.Fprintf(os.Stdout, "- %s stdio command (%s): %s\n", action.Target, status, shellJoin(action.Command))
		default:
			fmt.Fprintf(os.Stdout, "- %s %s: %s\n", action.Target, status, action.Message)
		}
	}
	if len(plan.Warnings) > 0 {
		fmt.Fprintln(os.Stdout, "Warnings:")
		for _, warning := range uniqueSorted(plan.Warnings) {
			fmt.Fprintf(os.Stdout, "- %s\n", warning)
		}
	}
}

func printPreApplyBackupNotice(plan Plan) {
	for _, action := range plan.Actions {
		if action.Backup == "" || (action.Type != "merge_config" && action.Type != "remove_config") {
			continue
		}
		fmt.Fprintf(os.Stderr, "%s %s will modify %s and create backup %s\n", action.Target, action.Type, action.Path, action.Backup)
		if action.Message != "" {
			fmt.Fprintf(os.Stderr, "%s\n", action.Message)
		}
	}
}

func uniqueSorted(values []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, value := range values {
		if seen[value] {
			continue
		}
		seen[value] = true
		out = append(out, value)
	}
	sort.Strings(out)
	return out
}

func shellJoin(args []string) string {
	quoted := make([]string, 0, len(args))
	for _, arg := range args {
		quoted = append(quoted, shellQuote(arg))
	}
	return strings.Join(quoted, " ")
}

func shellQuote(arg string) string {
	if arg == "" {
		return "''"
	}
	if strings.IndexFunc(arg, func(r rune) bool {
		return !(r == '-' || r == '_' || r == '.' || r == '/' || r == ':' || r == '=' ||
			(r >= '0' && r <= '9') || (r >= 'A' && r <= 'Z') || (r >= 'a' && r <= 'z'))
	}) == -1 {
		return arg
	}
	return "'" + strings.ReplaceAll(arg, "'", "'\"'\"'") + "'"
}
