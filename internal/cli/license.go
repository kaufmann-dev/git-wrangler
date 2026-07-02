package cli

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
)

type licenseOptions struct {
	target       targetOptions
	confirmation confirmationOptions
	holder       string
	overwrite    bool
}

func licenseOptionsFromCommand(a *app, cmd *cobra.Command) (licenseOptions, bool) {
	holder, ok := requiredStringFlag(a, cmd, "name", "Copyright holder name: ")
	if !ok {
		return licenseOptions{}, false
	}
	return licenseOptions{
		target:       targetOptionsFromCommand(cmd),
		confirmation: confirmationOptionsFromCommand(cmd),
		holder:       holder,
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
		if err := os.WriteFile(path, []byte(mitLicense(opts.holder)), 0o644); err != nil {
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

func mitLicense(holder string) string {
	return "MIT License\n\nCopyright (c) " + holder + "\n\n" + `Permission is hereby granted, free of charge, to any person obtaining a copy
of this software and associated documentation files (the "Software"), to deal
in the Software without restriction, including without limitation the rights
to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
copies of the Software, and to permit persons to whom the Software is
furnished to do so, subject to the following conditions:

The above copyright notice and this permission notice shall be included in all
copies or substantial portions of the Software.

THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
SOFTWARE.
`
}
