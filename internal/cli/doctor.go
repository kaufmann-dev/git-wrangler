package cli

import (
	"fmt"
	"io"
	"runtime"
	"strings"

	"github.com/kaufmann-dev/git-wrangler/internal/run"
	"github.com/spf13/cobra"
)

func runDoctor(a *app, cmd *cobra.Command, args []string) int {
	summaryOnly, _ := cmd.Flags().GetBool("summary")

	printDependencySummary(a)
	if summaryOnly {
		return 0
	}
	printInstallInstructions(a)
	fmt.Fprintln(a.stdout)
	fmt.Fprintln(a.stdout, "Run 'gh auth login' before private or all GitHub repository operations.")
	return 0
}

func printDependencySummary(a *app) {
	fmt.Fprintf(a.stdout, "\n%sDependencies%s\n\n", a.ui.Bold, a.ui.Reset)
	printDependency(a, "git", "all commands")
	printDependency(a, "gh", "clone, rename-repo")
	printFilterRepoDependency(a)
}

func printDependency(a *app, name, requiredFor string) {
	if path, err := run.LookPath(name); err == nil {
		fmt.Fprintf(a.stdout, "  %sfound%s   %-16s %s\n", a.ui.Green, a.ui.Reset, name, path)
	} else {
		fmt.Fprintf(a.stdout, "  %smissing%s %-16s required for %s\n", a.ui.Red, a.ui.Reset, name, requiredFor)
	}
}

func printFilterRepoDependency(a *app) {
	if path, err := run.LookPath("git-filter-repo"); err == nil {
		fmt.Fprintf(a.stdout, "  %sfound%s   %-16s %s\n", a.ui.Green, a.ui.Reset, "git-filter-repo", path)
	} else if _, ok := filterRepoCommand(&app{stderr: io.Discard, ui: a.ui}, "doctor"); ok {
		fmt.Fprintf(a.stdout, "  %sfound%s   %-16s %s\n", a.ui.Green, a.ui.Reset, "git-filter-repo", "git filter-repo")
	} else {
		fmt.Fprintf(a.stdout, "  %smissing%s %-16s required for %s\n", a.ui.Red, a.ui.Reset, "git-filter-repo", "rewrite-authors, rewrite-commits, rewrite-commits-ai, rewrite-dates, remove-secrets")
	}
}

func installed(name string) bool {
	_, err := run.LookPath(name)
	return err == nil
}

func filterRepoInstalled() bool {
	if installed("git-filter-repo") {
		return true
	}
	_, err := runCapture("", nil, "git", "filter-repo", "--version")
	return err == nil
}

func missingDependencyCount() int {
	count := 0
	for _, name := range []string{"git", "gh"} {
		if !installed(name) {
			count++
		}
	}
	if !filterRepoInstalled() {
		count++
	}
	return count
}

func printInstallInstructions(a *app) {
	if missingDependencyCount() == 0 {
		fmt.Fprintf(a.stdout, "\n%sAll command dependencies are installed.%s\n", a.ui.Green, a.ui.Reset)
		return
	}

	manager := detectPackageManager()
	fmt.Fprintf(a.stdout, "\n%sInstall Instructions%s\n\n", a.ui.Bold, a.ui.Reset)
	fmt.Fprintf(a.stdout, "  Detected: %s", detectOS())
	if manager != "unknown" {
		fmt.Fprintf(a.stdout, " with %s", manager)
	}
	fmt.Fprintln(a.stdout)
	fmt.Fprintln(a.stdout)

	packages := missingPackages(manager)
	printCommand := func(s string) { fmt.Fprintf(a.stdout, "    %s\n", s) }
	switch manager {
	case "brew":
		printCommand("brew install " + strings.Join(packages, " "))
	case "apt":
		if !installed("gh") {
			fmt.Fprintln(a.stdout, "  Add the GitHub CLI apt repository for gh:")
			printCommand("type -p wget >/dev/null || (sudo apt update && sudo apt install wget -y)")
			printCommand("sudo mkdir -p -m 755 /etc/apt/keyrings")
			printCommand("out=$(mktemp) && wget -nv -O\"$out\" https://cli.github.com/packages/githubcli-archive-keyring.gpg")
			printCommand("cat \"$out\" | sudo tee /etc/apt/keyrings/githubcli-archive-keyring.gpg >/dev/null")
			printCommand("sudo chmod go+r /etc/apt/keyrings/githubcli-archive-keyring.gpg")
			printCommand("sudo mkdir -p -m 755 /etc/apt/sources.list.d")
			printCommand("echo \"deb [arch=$(dpkg --print-architecture) signed-by=/etc/apt/keyrings/githubcli-archive-keyring.gpg] https://cli.github.com/packages stable main\" | sudo tee /etc/apt/sources.list.d/github-cli.list >/dev/null")
		}
		printCommand("sudo apt update")
		printCommand("sudo apt install " + strings.Join(packages, " "))
	case "dnf", "yum", "zypper", "pacman", "apk", "xbps":
		prefix := map[string]string{
			"dnf":    "sudo dnf install ",
			"yum":    "sudo yum install ",
			"zypper": "sudo zypper install ",
			"pacman": "sudo pacman -S ",
			"apk":    "sudo apk add ",
			"xbps":   "sudo xbps-install ",
		}[manager]
		printCommand(prefix + strings.Join(packages, " "))
	default:
		fmt.Fprintln(a.stdout, "  No supported package manager was detected. Install manually:")
		if !installed("git") {
			printCommand("git: https://git-scm.com/downloads")
		}
		if !installed("gh") {
			printCommand("gh: https://cli.github.com/")
		}
		if !filterRepoInstalled() {
			printCommand("git-filter-repo: https://github.com/newren/git-filter-repo/blob/main/INSTALL.md")
		}
	}
}

func missingPackages(manager string) []string {
	var packages []string
	add := func(commandName, packageName string) {
		if !installed(commandName) {
			packages = append(packages, packageName)
		}
	}
	add("git", "git")
	switch manager {
	case "pacman", "apk", "xbps":
		add("gh", "github-cli")
	default:
		add("gh", "gh")
	}
	if !filterRepoInstalled() {
		packages = append(packages, "git-filter-repo")
	}
	return packages
}

func detectOS() string {
	switch runtime.GOOS {
	case "darwin":
		return "macOS"
	case "linux":
		return "Linux"
	case "windows":
		return "Windows"
	default:
		return runtime.GOOS
	}
}

func detectPackageManager() string {
	if runtime.GOOS == "windows" {
		for _, name := range []string{"winget", "scoop", "choco"} {
			if installed(name) {
				if name == "choco" {
					return "chocolatey"
				}
				return name
			}
		}
		return "unknown"
	}
	if runtime.GOOS == "darwin" {
		if installed("brew") {
			return "brew"
		}
		return "unknown"
	}
	for _, name := range []string{"apt", "dnf", "yum", "zypper", "pacman", "apk", "xbps-install", "brew"} {
		if installed(name) {
			if name == "xbps-install" {
				return "xbps"
			}
			return name
		}
	}
	return "unknown"
}
