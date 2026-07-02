package cli

import (
	"fmt"
	"os"
	"runtime"
	"strings"

	"github.com/kaufmann-dev/git-wrangler/internal/config"
	"github.com/kaufmann-dev/git-wrangler/internal/credentials"
	"github.com/kaufmann-dev/git-wrangler/internal/version"
	"github.com/spf13/cobra"
)

func runDoctor(a *app, cmd *cobra.Command, args []string) int {
	opts := doctorOptionsFromCommand(cmd)
	if opts.json.enabled {
		return runDoctorJSON(a)
	}
	fmt.Fprintln(a.stdout, "Git Wrangler Doctor")
	fmt.Fprintln(a.stdout)
	fmt.Fprintln(a.stdout, "Runtime")
	runtimeRows := []keyValueRow{
		{key: "Version", value: firstLine(version.Full())},
		{key: "Platform", value: runtime.GOOS + "/" + runtime.GOARCH},
	}
	if executable, err := os.Executable(); err == nil {
		runtimeRows = append(runtimeRows, keyValueRow{key: "Executable", value: executable})
	} else {
		runtimeRows = append(runtimeRows, keyValueRow{key: "Executable", value: a.ui.Muted + "unknown" + a.ui.Reset})
	}
	renderKeyValues(a, runtimeRows)
	fmt.Fprintln(a.stdout)
	fmt.Fprintln(a.stdout, "Dependencies")

	status := 0
	missing := false
	dependencyRows := [][]string{}
	if !doctorCommand(a, &dependencyRows, "git", "most Git Wrangler commands", true, "--version") {
		status = 1
		missing = true
	}
	if !doctorCommand(a, &dependencyRows, "gh", "clone and rename-repo", false, "--version") {
		missing = true
	}
	if !doctorFilterRepo(a, &dependencyRows) {
		missing = true
	}
	renderTable(a, []tableColumn{{header: "Check"}, {header: "State"}, {header: "Detail"}}, dependencyRows)
	if missing {
		fmt.Fprintln(a.stdout)
		fmt.Fprintln(a.stdout, "Source installs do not include runtime dependencies. Install missing tools yourself or use an official bundled install.")
	}
	fmt.Fprintln(a.stdout)
	if !doctorConfig(a) {
		status = 1
	}
	return status
}

func runDoctorJSON(a *app) int {
	type dependency struct {
		Name     string `json:"name"`
		OK       bool   `json:"ok"`
		Critical bool   `json:"critical"`
		Path     string `json:"path,omitempty"`
		Version  string `json:"version,omitempty"`
		Message  string `json:"message,omitempty"`
	}
	dependencies := []dependency{}
	status := 0
	missing := 0
	addCommand := func(name string, critical bool, args ...string) {
		path, err := a.runner.LookPath(name)
		if err != nil {
			dependencies = append(dependencies, dependency{Name: name, Critical: critical, Message: "not found"})
			missing++
			if critical {
				status = 1
			}
			return
		}
		dependencies = append(dependencies, dependency{Name: name, OK: true, Critical: critical, Path: path, Version: doctorVersion(a, path, args...)})
	}
	addCommand("git", true, "--version")
	addCommand("gh", false, "--version")
	if filterRepo, ok := a.git.FilterRepoCommand(a.ctx); ok {
		dependencies = append(dependencies, dependency{Name: "git-filter-repo", OK: true, Path: strings.Join(filterRepo, " "), Version: doctorVersion(a, filterRepo[0], append(filterRepo[1:], "--version")...)})
	} else {
		dependencies = append(dependencies, dependency{Name: "git-filter-repo", Message: "not found"})
		missing++
	}

	configInfo := map[string]any{}
	path, err := config.Path()
	if err != nil {
		status = 1
		configInfo["error"] = jsonError{Message: err.Error()}
	} else {
		configInfo["path"] = path
		cfg, err := config.Load()
		if err != nil {
			status = 1
			configInfo["error"] = jsonError{Message: err.Error()}
		} else {
			github := credentials.ResolveGitHubToken(a.creds, cfg.GitHub.Host)
			aiKey := credentials.ResolveAIKey(a.creds, cfg.AI.Provider)
			configInfo["github"] = map[string]any{
				"host":       cfg.GitHub.Host,
				"username":   cfg.GitHub.Username,
				"authSource": string(github.Source),
				"authSet":    github.Value != "",
			}
			configInfo["ai"] = map[string]any{
				"provider":     cfg.AI.Provider,
				"baseURL":      cfg.AI.BaseURL,
				"model":        cfg.AI.Model,
				"apiKeySource": string(aiKey.Source),
				"apiKeySet":    aiKey.Value != "",
			}
			configInfo["keyringAvailable"] = credentials.KeyringAvailable(a.creds)
		}
	}
	_ = writeJSON(a, map[string]any{
		"ok":           status == 0,
		"summary":      map[string]int{"dependencies": len(dependencies), "missing": missing},
		"dependencies": dependencies,
		"config":       configInfo,
	})
	return status
}

func doctorCommand(a *app, rows *[][]string, name, neededFor string, critical bool, args ...string) bool {
	path, err := a.runner.LookPath(name)
	if err != nil {
		*rows = append(*rows, []string{name, doctorState(a, critical), "not found; needed for " + neededFor})
		return false
	}
	versionText := doctorVersion(a, path, args...)
	*rows = append(*rows, []string{name, a.ui.Green + "OK" + a.ui.Reset, doctorDetail(path, versionText)})
	return true
}

func doctorFilterRepo(a *app, rows *[][]string) bool {
	filterRepo, ok := a.git.FilterRepoCommand(a.ctx)
	if !ok {
		*rows = append(*rows, []string{"git-filter-repo", a.ui.Yellow + "WARN" + a.ui.Reset, "not found; needed for history rewrite commands"})
		return false
	}
	versionText := doctorVersion(a, filterRepo[0], append(filterRepo[1:], "--version")...)
	*rows = append(*rows, []string{"git-filter-repo", a.ui.Green + "OK" + a.ui.Reset, doctorDetail(strings.Join(filterRepo, " "), versionText)})
	return true
}

func doctorVersion(a *app, name string, args ...string) string {
	stdout, stderr, err := a.runner.Run(a.ctx, "", nil, name, args...)
	if err != nil {
		return ""
	}
	return firstLine(strings.TrimSpace(stdout + stderr))
}

func doctorDetail(detail, versionText string) string {
	if versionText == "" {
		return detail
	}
	return fmt.Sprintf("%s (%s)", detail, versionText)
}

func doctorState(a *app, critical bool) string {
	if critical {
		return a.ui.Red + "ERROR" + a.ui.Reset
	}
	return a.ui.Yellow + "WARN" + a.ui.Reset
}

func doctorConfig(a *app) bool {
	fmt.Fprintln(a.stdout, "Config And Auth")
	path, err := config.Path()
	if err != nil {
		renderTable(a, []tableColumn{{header: "Check"}, {header: "State"}, {header: "Detail"}}, [][]string{{"config", a.ui.Red + "ERROR" + a.ui.Reset, err.Error()}})
		return false
	}
	cfg, err := config.Load()
	if err != nil {
		renderTable(a, []tableColumn{{header: "Check"}, {header: "State"}, {header: "Detail"}}, [][]string{{"config", a.ui.Red + "ERROR" + a.ui.Reset, path + ": " + err.Error()}})
		return false
	}
	rows := [][]string{{"config", a.ui.Green + "OK" + a.ui.Reset, path}}
	if credentials.KeyringAvailable(a.creds) {
		rows = append(rows, []string{"keyring", a.ui.Green + "OK" + a.ui.Reset, "available"})
	} else {
		rows = append(rows, []string{"keyring", a.ui.Yellow + "WARN" + a.ui.Reset, "unavailable"})
	}
	github := credentials.ResolveGitHubToken(a.creds, cfg.GitHub.Host)
	rows = append(rows, []string{"github.auth", doctorCredentialState(a, github), string(github.Source)})
	rows = append(rows, []string{"github.host", a.ui.Green + "OK" + a.ui.Reset, cfg.GitHub.Host})
	if cfg.GitHub.Username != "" {
		rows = append(rows, []string{"github.username", a.ui.Green + "OK" + a.ui.Reset, cfg.GitHub.Username})
	}
	aiKey := credentials.ResolveAIKey(a.creds, cfg.AI.Provider)
	rows = append(rows, []string{"ai.api-key", doctorCredentialState(a, aiKey), string(aiKey.Source)})
	rows = append(rows, doctorConfigRow(a, "ai.provider", cfg.AI.Provider))
	rows = append(rows, doctorConfigRow(a, "ai.base-url", cfg.AI.BaseURL))
	rows = append(rows, doctorConfigRow(a, "ai.model", cfg.AI.Model))
	renderTable(a, []tableColumn{{header: "Check"}, {header: "State"}, {header: "Detail"}}, rows)
	return true
}

func doctorCredentialState(a *app, resolved credentials.Resolved) string {
	if resolved.Value == "" {
		return a.ui.Yellow + "WARN" + a.ui.Reset
	}
	return a.ui.Green + "OK" + a.ui.Reset
}

func doctorConfigRow(a *app, name, value string) []string {
	if value == "" {
		return []string{name, a.ui.Yellow + "WARN" + a.ui.Reset, "missing"}
	}
	return []string{name, a.ui.Green + "OK" + a.ui.Reset, value}
}
