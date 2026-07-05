package cli

import (
	"bufio"
	"embed"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"

	enry "github.com/go-enry/go-enry/v2"
	"github.com/spf13/cobra"
)

//go:embed gitignore_templates/*.gitignore gitignore_templates/Global/*.gitignore
var gitignoreTemplates embed.FS

type fixGitignoreOptions struct {
	target       targetOptions
	confirmation confirmationOptions
}

func fixGitignoreOptionsFromCommand(cmd *cobra.Command) fixGitignoreOptions {
	return fixGitignoreOptions{
		target:       targetOptionsFromCommand(cmd),
		confirmation: confirmationOptionsFromCommand(cmd),
	}
}

func runFixGitignore(a *app, cmd *cobra.Command, args []string) int {
	opts := fixGitignoreOptionsFromCommand(cmd)
	if !requireGit(a, "fix-gitignore") {
		return 1
	}
	repos, err := opts.target.repositories()
	if err != nil {
		a.error(err.Error())
		return 1
	}
	if len(repos) == 0 {
		return noRepos(a)
	}
	type gitignoreScan struct {
		repo  repo
		added []string
		err   error
	}
	scans := parallelReposProgress(a.ctx, repos, newProgress(a, "Scanning .gitignore candidates", len(repos)), func(r repo) gitignoreScan {
		added, err := proposedGitignoreEntries(a, r.dir)
		if err != nil {
			return gitignoreScan{repo: r, err: err}
		}
		return gitignoreScan{repo: r, added: added}
	})
	if interrupted(a) {
		return 1
	}
	status := 0
	applies := []gitignoreScan{}
	unchanged := 0
	scanFailed := 0
	for _, scan := range scans {
		r := scan.repo
		if scan.err != nil {
			renderErrorBlock(a, r.display+": could not scan .gitignore candidates", scan.err.Error())
			status = 1
			scanFailed++
			continue
		}
		added := scan.added
		if len(added) > 0 {
			renderRepoHeader(a, r.display)
			fmt.Fprintf(a.stdout, "  %sWill add:%s %s\n", a.ui.Yellow, a.ui.Reset, strings.Join(added, ", "))
			fmt.Fprintln(a.stdout)
			applies = append(applies, scan)
		} else {
			unchanged++
		}
	}
	renderSummary(a,
		summaryCount{label: "with changes", value: len(applies), color: a.ui.Yellow},
		summaryCount{label: "unchanged", value: unchanged, color: a.ui.Green},
		summaryCount{label: "failed", value: scanFailed, color: a.ui.Red},
	)
	if len(applies) == 0 {
		return status
	}
	renderWarning(a, fmt.Sprintf("This operation will modify .gitignore in %d repositories.", len(applies)))
	confirmation := confirmOrSkip(a, opts.confirmation.yes, fmt.Sprintf("Apply .gitignore updates for %d repositories?", len(applies)))
	if confirmation == confirmationUnavailable || confirmation == confirmationCancelled {
		return 1
	}
	if confirmation == confirmationDeclined {
		renderSummary(a,
			summaryCount{label: "updated", value: 0, color: a.ui.Green},
			summaryCount{label: "skipped", value: len(applies), color: a.ui.Yellow},
			summaryCount{label: "failed", value: scanFailed, color: a.ui.Red},
		)
		return status
	}
	updated := 0
	applyFailed := 0
	progress := newProgress(a, "Applying .gitignore updates", len(applies))
	type applyError struct {
		subject string
		output  string
	}
	applyErrors := []applyError{}
	for _, apply := range applies {
		r := apply.repo
		progress.start(r.display)
		if err := appendGitignoreEntries(filepath.Join(r.dir, ".gitignore"), apply.added); err != nil {
			progress.advance(r.display)
			applyErrors = append(applyErrors, applyError{subject: r.display + ": could not update .gitignore", output: err.Error()})
			status = 1
			applyFailed++
			continue
		}
		updated++
		progress.advance(r.display)
	}
	finishProgressBeforeOutput(progress)
	for _, err := range applyErrors {
		renderErrorBlock(a, err.subject, err.output)
	}
	renderSummary(a,
		summaryCount{label: "updated", value: updated, color: a.ui.Green},
		summaryCount{label: "skipped", value: 0, color: a.ui.Yellow},
		summaryCount{label: "failed", value: scanFailed + applyFailed, color: a.ui.Red},
	)
	return status
}

type gitignoreInventoryEntry struct {
	path string
	dir  bool
}

func proposedGitignoreEntries(a *app, root string) ([]string, error) {
	inventory, err := gitignoreInventory(root)
	if err != nil {
		return nil, err
	}
	templates := selectedGitignoreTemplates(root, inventory)
	rules, err := gitignoreTemplateRules(templates)
	if err != nil {
		return nil, err
	}
	added := []string{}
	for _, rule := range rules {
		match := findGitignoreRuleMatch(inventory, rule)
		if match == "" {
			continue
		}
		if _, err := a.git.Capture(a.ctx, root, nil, "check-ignore", "-q", match); err == nil {
			continue
		}
		if fileContainsLine(filepath.Join(root, ".gitignore"), rule) {
			continue
		}
		added = append(added, rule)
	}
	return added, nil
}

func gitignoreInventory(root string) ([]gitignoreInventoryEntry, error) {
	entries := []gitignoreInventoryEntry{}
	err := filepath.WalkDir(root, func(filePath string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(root, filePath)
		if err != nil {
			return err
		}
		if rel == "." {
			return nil
		}
		if rel == ".git" || strings.HasPrefix(rel, ".git"+string(filepath.Separator)) {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if d.Type()&os.ModeSymlink != 0 {
			return nil
		}
		entries = append(entries, gitignoreInventoryEntry{path: filepath.ToSlash(rel), dir: d.IsDir()})
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].path < entries[j].path
	})
	return entries, nil
}

func selectedGitignoreTemplates(root string, inventory []gitignoreInventoryEntry) map[string]bool {
	selected := map[string]bool{}
	for _, entry := range inventory {
		if entry.dir {
			continue
		}
		for _, template := range gitignoreProjectMarkerTemplates(entry.path) {
			selected[normalizeGitignoreTemplateName(template)] = true
		}
		data, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(entry.path)))
		if err != nil {
			continue
		}
		language := enry.GetLanguage(entry.path, data)
		for _, template := range gitignoreLanguageTemplates(language) {
			selected[normalizeGitignoreTemplateName(template)] = true
		}
	}
	return selected
}

func gitignoreProjectMarkerTemplates(filePath string) []string {
	switch path.Base(filePath) {
	case "package.json":
		return []string{"Node"}
	default:
		return nil
	}
}

func gitignoreLanguageTemplates(language string) []string {
	if language == "" {
		return nil
	}
	switch language {
	case "JavaScript", "TypeScript", "JSX", "TSX":
		return []string{"Node"}
	case "C#", "F#", "Visual Basic .NET":
		return []string{"VisualStudio"}
	default:
		return []string{language}
	}
}

func gitignoreTemplateRules(selected map[string]bool) ([]string, error) {
	templatePaths := []string{}
	err := fs.WalkDir(gitignoreTemplates, "gitignore_templates", func(templatePath string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || filepath.Ext(templatePath) != ".gitignore" {
			return nil
		}
		if strings.HasPrefix(templatePath, "gitignore_templates/Global/") {
			templatePaths = append(templatePaths, templatePath)
			return nil
		}
		name := strings.TrimSuffix(path.Base(templatePath), ".gitignore")
		if selected[normalizeGitignoreTemplateName(name)] {
			templatePaths = append(templatePaths, templatePath)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.Strings(templatePaths)
	seen := map[string]bool{}
	rules := []string{}
	for _, templatePath := range templatePaths {
		data, err := gitignoreTemplates.ReadFile(templatePath)
		if err != nil {
			return nil, err
		}
		for _, line := range strings.Split(string(data), "\n") {
			rule := strings.TrimSpace(strings.TrimRight(line, "\r"))
			if rule == "" || strings.HasPrefix(rule, "#") || strings.HasPrefix(rule, "!") {
				continue
			}
			if seen[rule] {
				continue
			}
			seen[rule] = true
			rules = append(rules, rule)
		}
	}
	return rules, nil
}

func normalizeGitignoreTemplateName(name string) string {
	name = strings.ToLower(name)
	name = strings.ReplaceAll(name, " ", "")
	name = strings.ReplaceAll(name, "-", "")
	name = strings.ReplaceAll(name, "_", "")
	name = strings.ReplaceAll(name, ".", "")
	return name
}

func findGitignoreRuleMatch(inventory []gitignoreInventoryEntry, rule string) string {
	dirOnly := strings.HasSuffix(rule, "/")
	pattern := strings.TrimSuffix(rule, "/")
	anchored := strings.HasPrefix(pattern, "/")
	pattern = strings.TrimPrefix(pattern, "/")
	if pattern == "" {
		return ""
	}
	for _, entry := range inventory {
		if dirOnly && !entry.dir {
			continue
		}
		if gitignoreRuleMatches(pattern, anchored, entry.path) {
			return "./" + entry.path
		}
	}
	return ""
}

func gitignoreRuleMatches(pattern string, anchored bool, filePath string) bool {
	if pattern == filePath {
		return true
	}
	if anchored {
		return pathMatch(pattern, filePath)
	}
	if !strings.Contains(pattern, "/") {
		return pathMatch(pattern, path.Base(filePath))
	}
	if pathMatch(pattern, filePath) {
		return true
	}
	parts := strings.Split(filePath, "/")
	for i := 1; i < len(parts); i++ {
		if pathMatch(pattern, strings.Join(parts[i:], "/")) {
			return true
		}
	}
	return false
}

func pathMatch(pattern, name string) bool {
	if ok, err := path.Match(pattern, name); err == nil && ok {
		return true
	}
	if strings.HasPrefix(pattern, "**/") {
		return pathMatch(strings.TrimPrefix(pattern, "**/"), name)
	}
	return false
}

func fileContainsLine(path, line string) bool {
	f, err := os.Open(path)
	if err != nil {
		return false
	}
	defer f.Close()
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		if scanner.Text() == line {
			return true
		}
	}
	return false
}

func appendGitignoreEntries(path string, entries []string) error {
	if data, err := os.ReadFile(path); err == nil && len(data) > 0 && data[len(data)-1] != '\n' {
		f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0o644)
		if err != nil {
			return err
		}
		if _, err := f.WriteString("\n"); err != nil {
			_ = f.Close()
			return err
		}
		if err := f.Close(); err != nil {
			return err
		}
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()
	for _, entry := range entries {
		if _, err := fmt.Fprintln(f, entry); err != nil {
			return err
		}
	}
	return nil
}
