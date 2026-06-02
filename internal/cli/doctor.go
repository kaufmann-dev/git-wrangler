package cli

import (
	"fmt"
	"os"
	"runtime"
	"strings"

	"github.com/kaufmann-dev/git-wrangler/internal/version"
	"github.com/spf13/cobra"
)

func runDoctor(a *app, cmd *cobra.Command, args []string) int {
	fmt.Fprintln(a.stdout, "Git Wrangler Doctor")
	fmt.Fprintln(a.stdout)
	fmt.Fprintf(a.stdout, "Version:    %s\n", firstLine(version.Full()))
	fmt.Fprintf(a.stdout, "Platform:   %s/%s\n", runtime.GOOS, runtime.GOARCH)
	if executable, err := os.Executable(); err == nil {
		fmt.Fprintf(a.stdout, "Executable: %s\n", executable)
	} else {
		fmt.Fprintln(a.stdout, "Executable: unknown")
	}
	fmt.Fprintln(a.stdout)
	fmt.Fprintln(a.stdout, "Dependencies:")

	status := 0
	missing := false
	if !doctorCommand(a, "git", "most Git Wrangler commands", true, "--version") {
		status = 1
		missing = true
	}
	if !doctorCommand(a, "gh", "clone and rename-repo", false, "--version") {
		missing = true
	}
	if !doctorFilterRepo(a) {
		missing = true
	}
	if missing {
		fmt.Fprintln(a.stdout)
		fmt.Fprintln(a.stdout, "Source installs do not include runtime dependencies. Install missing tools yourself or use an official bundled install.")
	}
	return status
}

func doctorCommand(a *app, name, neededFor string, critical bool, args ...string) bool {
	path, err := a.runner.LookPath(name)
	if err != nil {
		doctorMissing(a, name, neededFor, critical)
		return false
	}
	versionText := doctorVersion(a, path, args...)
	doctorOK(a, name, path, versionText)
	return true
}

func doctorFilterRepo(a *app) bool {
	filterRepo, ok := a.git.FilterRepoCommand(a.ctx)
	if !ok {
		doctorMissing(a, "git-filter-repo", "history rewrite commands", false)
		return false
	}
	versionText := doctorVersion(a, filterRepo[0], append(filterRepo[1:], "--version")...)
	doctorOK(a, "git-filter-repo", strings.Join(filterRepo, " "), versionText)
	return true
}

func doctorVersion(a *app, name string, args ...string) string {
	stdout, stderr, err := a.runner.Run(a.ctx, "", nil, name, args...)
	if err != nil {
		return ""
	}
	return firstLine(strings.TrimSpace(stdout + stderr))
}

func doctorOK(a *app, name, detail, versionText string) {
	if versionText != "" {
		detail = fmt.Sprintf("%s (%s)", detail, versionText)
	}
	fmt.Fprintf(a.stdout, "  OK    %-16s %s\n", name, detail)
}

func doctorMissing(a *app, name, neededFor string, critical bool) {
	label := "WARN"
	if critical {
		label = "ERROR"
	}
	fmt.Fprintf(a.stdout, "  %-5s %-16s not found; needed for %s\n", label, name, neededFor)
}
