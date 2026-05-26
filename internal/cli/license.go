package cli

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
)

func runLicense(a *app, cmd *cobra.Command, args []string) int {
	repoName, _ := cmd.Flags().GetString("repo")
	holder, _ := cmd.Flags().GetString("name")
	overwrite, _ := cmd.Flags().GetBool("overwrite")
	if holder == "" {
		a.error("Copyright holder name is required. Use --name <NAME>.")
		return 1
	}
	if !requireGit(a, "license") {
		return 1
	}
	root := "."
	if repoName != "" {
		root = repoName
	}
	repos, err := findGitRepositories(root)
	if err != nil {
		a.error(err.Error())
		return 1
	}
	if len(repos) == 0 {
		return noRepos(a)
	}
	for _, r := range repos {
		path := filepath.Join(r.dir, "LICENSE")
		if fileExists(path) && !overwrite {
			fmt.Fprintf(a.stdout, "%sLICENSE file already exists in repository: %s (use --overwrite to replace it)%s\n", a.ui.Yellow, r.display, a.ui.Reset)
			continue
		}
		_ = os.WriteFile(path, []byte(mitLicense(holder)), 0o644)
		if overwrite && fileExists(path) {
			fmt.Fprintf(a.stdout, "%sLICENSE file overwritten in repository: %s%s\n", a.ui.Green, r.display, a.ui.Reset)
		} else {
			fmt.Fprintf(a.stdout, "%sLICENSE file created in repository: %s%s\n", a.ui.Green, r.display, a.ui.Reset)
		}
	}
	return 0
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
