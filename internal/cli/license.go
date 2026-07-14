package cli

import (
	"embed"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/spf13/cobra"
)

type licenseTemplate struct {
	id             string
	displayName    string
	requiresHolder bool
	gnuNotice      bool
	noticeInBody   bool
	textFile       string
}

type licenseOptions struct {
	target       targetOptions
	confirmation confirmationOptions
	template     licenseTemplate
	holder       string
	year         int
	overwrite    bool
}

type licenseRemoveOptions struct {
	target       targetOptions
	confirmation confirmationOptions
}

func licenseOptionsFromCommand(a *app, cmd *cobra.Command) (licenseOptions, bool) {
	licenseType, ok := requiredStringFlag(a, cmd, "type", "License type: ")
	if !ok {
		return licenseOptions{}, false
	}
	template, ok := licenseTemplateByID(licenseType)
	if !ok {
		a.plainErrorf("unsupported license type %q. Supported types: %s.", licenseType, strings.Join(supportedLicenseIDs(), ", "))
		return licenseOptions{}, false
	}
	holder := stringFlagValue(cmd, "name")
	if template.requiresHolder {
		var holderOK bool
		holder, holderOK = requiredStringFlag(a, cmd, "name", "Copyright holder name: ")
		if !holderOK {
			return licenseOptions{}, false
		}
	}
	year := intFlagValue(cmd, "year")
	if year <= 0 {
		a.plainErrorf("--year must be a positive integer.")
		return licenseOptions{}, false
	}
	return licenseOptions{
		target:       targetOptionsFromCommand(cmd),
		confirmation: confirmationOptionsFromCommand(cmd),
		template:     template,
		holder:       holder,
		year:         year,
		overwrite:    boolFlagValue(cmd, "overwrite"),
	}, true
}

func runLicense(a *app, cmd *cobra.Command, args []string) int {
	opts, ok := licenseOptionsFromCommand(a, cmd)
	if !ok {
		return 1
	}
	if !requireGit(a, "license") {
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
	content, err := renderLicense(opts.template, opts.year, opts.holder)
	if err != nil {
		a.error(err.Error())
		return 1
	}
	overwriteCount := 0
	if opts.overwrite {
		for _, r := range repos {
			if fileExists(filepath.Join(r.dir, "LICENSE")) {
				overwriteCount++
			}
		}
	}
	overwriteConfirmed := true
	if overwriteCount > 0 {
		confirmation := confirmOrSkip(a, opts.confirmation.yes, fmt.Sprintf("Overwrite existing LICENSE files in %d repositories?", overwriteCount))
		if confirmation == confirmationUnavailable || confirmation == confirmationCancelled {
			return 1
		}
		overwriteConfirmed = confirmation == confirmationAccepted
	}
	status := 0
	created := 0
	overwritten := 0
	skipped := 0
	failed := 0
	for _, r := range repos {
		path := filepath.Join(r.dir, "LICENSE")
		if fileExists(path) && !opts.overwrite {
			renderStatusLine(a, a.stdout, statusSkip, r.display, "LICENSE already exists; use --overwrite to replace it")
			skipped++
			continue
		}
		existed := fileExists(path)
		if existed && opts.overwrite && !overwriteConfirmed {
			skipped++
			continue
		}
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			renderErrorBlock(a, r.display+": could not write LICENSE", err.Error())
			status = 1
			failed++
			continue
		}
		if existed && opts.overwrite {
			overwritten++
		} else {
			created++
		}
	}
	renderSummary(a,
		summaryCount{label: "created", value: created, color: a.ui.Green},
		summaryCount{label: "overwritten", value: overwritten, color: a.ui.Green},
		summaryCount{label: "skipped", value: skipped, color: a.ui.Yellow},
		summaryCount{label: "failed", value: failed, color: a.ui.Red},
	)
	return status
}

func licenseRemoveOptionsFromCommand(cmd *cobra.Command) licenseRemoveOptions {
	return licenseRemoveOptions{
		target:       targetOptionsFromCommand(cmd),
		confirmation: confirmationOptionsFromCommand(cmd),
	}
}

func runLicenseRemove(a *app, cmd *cobra.Command, args []string) int {
	opts := licenseRemoveOptionsFromCommand(cmd)
	if !requireGit(a, "license remove") {
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

	candidates := make([]repo, 0, len(repos))
	skipped := 0
	failed := 0
	status := 0
	for _, r := range repos {
		path := filepath.Join(r.dir, "LICENSE")
		info, err := os.Lstat(path)
		switch {
		case os.IsNotExist(err):
			renderStatusLine(a, a.stdout, statusSkip, r.display, "LICENSE does not exist")
			skipped++
		case err != nil:
			renderErrorBlock(a, r.display+": could not inspect LICENSE", err.Error())
			failed++
			status = 1
		case info.IsDir():
			renderErrorBlock(a, r.display+": could not remove LICENSE", "LICENSE is a directory, not a file")
			failed++
			status = 1
		case !info.Mode().IsRegular() && info.Mode()&os.ModeSymlink == 0:
			renderErrorBlock(a, r.display+": could not remove LICENSE", "LICENSE is not a regular file or symbolic link")
			failed++
			status = 1
		default:
			candidates = append(candidates, r)
		}
	}

	if len(candidates) == 0 {
		renderLicenseRemovalSummary(a, 0, skipped, failed)
		return status
	}

	renderWarning(a, fmt.Sprintf("This operation will permanently remove LICENSE files from %d repositories.", len(candidates)))
	confirmation := confirmOrSkip(a, opts.confirmation.yes, fmt.Sprintf("Remove LICENSE files from %d repositories?", len(candidates)))
	if confirmation == confirmationUnavailable || confirmation == confirmationCancelled {
		return 1
	}
	if confirmation == confirmationDeclined {
		renderLicenseRemovalSummary(a, 0, skipped+len(candidates), failed)
		return status
	}

	removed := 0
	for _, r := range candidates {
		path := filepath.Join(r.dir, "LICENSE")
		if err := os.Remove(path); err != nil {
			if os.IsNotExist(err) {
				renderStatusLine(a, a.stdout, statusSkip, r.display, "LICENSE does not exist")
				skipped++
				continue
			}
			renderErrorBlock(a, r.display+": could not remove LICENSE", err.Error())
			failed++
			status = 1
			continue
		}
		removed++
	}
	renderLicenseRemovalSummary(a, removed, skipped, failed)
	return status
}

func renderLicenseRemovalSummary(a *app, removed, skipped, failed int) {
	renderSummary(a,
		summaryCount{label: "removed", value: removed, color: a.ui.Green},
		summaryCount{label: "skipped", value: skipped, color: a.ui.Yellow},
		summaryCount{label: "failed", value: failed, color: a.ui.Red},
	)
}

func supportedLicenseIDs() []string {
	ids := make([]string, 0, len(licenseTemplates))
	for _, template := range licenseTemplates {
		ids = append(ids, template.id)
	}
	return ids
}

func licenseTemplateByID(id string) (licenseTemplate, bool) {
	id = strings.ToLower(strings.TrimSpace(id))
	for _, template := range licenseTemplates {
		if template.id == id {
			return template, true
		}
	}
	return licenseTemplate{}, false
}

var licenseTemplates = []licenseTemplate{
	{id: "apache-2.0", displayName: "Apache License 2.0", requiresHolder: true, textFile: "Apache-2.0.txt"},
	{id: "gpl-3.0", displayName: "GNU General Public License v3.0", requiresHolder: true, gnuNotice: true, textFile: "GPL-3.0-only.txt"},
	{id: "mit", displayName: "MIT License", requiresHolder: true, noticeInBody: true, textFile: "MIT.txt"},
	{id: "0bsd", displayName: "BSD Zero Clause License", requiresHolder: true, noticeInBody: true, textFile: "0BSD.txt"},
	{id: "bsd-2-clause", displayName: "BSD 2-Clause License", requiresHolder: true, noticeInBody: true, textFile: "BSD-2-Clause.txt"},
	{id: "bsd-3-clause", displayName: "BSD 3-Clause License", requiresHolder: true, noticeInBody: true, textFile: "BSD-3-Clause.txt"},
	{id: "bsl-1.0", displayName: "Boost Software License 1.0", textFile: "BSL-1.0.txt"},
	{id: "cc0-1.0", displayName: "Creative Commons Zero v1.0 Universal", textFile: "CC0-1.0.txt"},
	{id: "epl-2.0", displayName: "Eclipse Public License 2.0", requiresHolder: true, textFile: "EPL-2.0.txt"},
	{id: "agpl-3.0", displayName: "GNU Affero General Public License v3.0", requiresHolder: true, gnuNotice: true, textFile: "AGPL-3.0-only.txt"},
	{id: "gpl-2.0", displayName: "GNU General Public License v2.0", requiresHolder: true, gnuNotice: true, textFile: "GPL-2.0-only.txt"},
	{id: "lgpl-2.1", displayName: "GNU Lesser General Public License v2.1", requiresHolder: true, gnuNotice: true, textFile: "LGPL-2.1-only.txt"},
	{id: "mpl-2.0", displayName: "Mozilla Public License 2.0", requiresHolder: true, textFile: "MPL-2.0.txt"},
	{id: "unlicense", displayName: "The Unlicense", textFile: "Unlicense.txt"},
}

//go:embed license_templates/*.txt
var licenseTextFS embed.FS

func renderLicense(template licenseTemplate, year int, holder string) (string, error) {
	body, err := embeddedLicenseText(template.textFile)
	if err != nil {
		return "", err
	}
	if template.requiresHolder {
		body = replaceLicensePlaceholders(body, year, holder)
		if !template.noticeInBody {
			body = licenseCopyrightNotice(template, year, holder) + "\n\n" + body
		}
	}
	return strings.TrimRight(body, "\n") + "\n", nil
}

func embeddedLicenseText(name string) (string, error) {
	data, err := licenseTextFS.ReadFile("license_templates/" + name)
	if err != nil {
		return "", fmt.Errorf("embedded license text missing: %s", name)
	}
	return string(data), nil
}

func replaceLicensePlaceholders(body string, year int, holder string) string {
	yearText := strconv.Itoa(year)
	replacements := []struct {
		old string
		new string
	}{
		{"Copyright (C) <year>  <name of author>", fmt.Sprintf("Copyright (C) %d %s", year, holder)},
		{"Copyright (C) year  name of author", fmt.Sprintf("Copyright (C) %d %s", year, holder)},
		{"Copyright (C) yyyy name of author", fmt.Sprintf("Copyright (C) %d %s", year, holder)},
		{"<year>", yearText},
		{"[yyyy]", yearText},
		{"<owner>", holder},
		{"<copyright holders>", holder},
		{"[name of copyright owner]", holder},
		{"<name of author>", holder},
		{"{name license(s), version(s), and exceptions or additional permissions here}", "GNU General Public License, version 2.0"},
	}
	for _, replacement := range replacements {
		body = strings.ReplaceAll(body, replacement.old, replacement.new)
	}
	return body
}

func licenseCopyrightNotice(template licenseTemplate, year int, holder string) string {
	if template.gnuNotice {
		return fmt.Sprintf("Copyright (C) %d %s", year, holder)
	}
	return fmt.Sprintf("Copyright (c) %d %s", year, holder)
}
