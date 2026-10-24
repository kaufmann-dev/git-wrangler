package repos

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

type Repository struct {
	GitDir  string
	Dir     string
	Display string
}

func Discover(root string) ([]Repository, error) {
	if root == "" {
		root = "."
	}
	var found []Repository
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() {
			if d.Name() != ".git" {
				return nil
			}
			target, ok := gitFileTarget(path)
			if !ok {
				return nil
			}
			// Submodule checkouts belong to their superproject and sit
			// detached at its pinned commit; only linked worktrees count.
			if isSubmoduleGitDir(target) {
				return filepath.SkipDir
			}
			dir := filepath.Dir(path)
			found = append(found, Repository{
				GitDir:  path,
				Dir:     dir,
				Display: DisplayName(dir),
			})
			return filepath.SkipDir
		}
		if d.Name() == ".git" {
			// An empty or partial .git directory is not a repository; Git would
			// resolve it to an enclosing repository and duplicate that target.
			if !isGitDir(path) {
				return filepath.SkipDir
			}
			dir := DirFromGitDir(path)
			found = append(found, Repository{
				GitDir:  path,
				Dir:     dir,
				Display: DisplayName(dir),
			})
			return filepath.SkipDir
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.Slice(found, func(i, j int) bool { return found[i].Dir < found[j].Dir })
	return found, nil
}

func ResolveExact(path string) (Repository, error) {
	if path == "" {
		path = "."
	}
	info, err := os.Stat(path)
	if err != nil {
		return Repository{}, err
	}
	if !info.IsDir() {
		return Repository{}, fmt.Errorf("%s is not a Git repository", path)
	}
	if isGitDir(path) {
		return Repository{}, fmt.Errorf("%s is a bare Git repository; --repo requires a working tree", path)
	}
	gitPath := filepath.Join(path, ".git")
	if isGitDir(gitPath) {
		return Repository{GitDir: gitPath, Dir: path, Display: DisplayName(path)}, nil
	}
	if validGitFile(gitPath) {
		return Repository{GitDir: gitPath, Dir: path, Display: DisplayName(path)}, nil
	}
	return Repository{}, fmt.Errorf("%s is not a Git repository", path)
}

func validGitFile(path string) bool {
	_, ok := gitFileTarget(path)
	return ok
}

// gitFileTarget resolves the gitdir: pointer of a .git file and reports
// whether it names an existing directory.
func gitFileTarget(path string) (string, bool) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", false
	}
	target, ok := strings.CutPrefix(strings.TrimSpace(string(data)), "gitdir:")
	if !ok {
		return "", false
	}
	target = strings.TrimSpace(target)
	if target == "" {
		return "", false
	}
	if !filepath.IsAbs(target) {
		target = filepath.Join(filepath.Dir(path), target)
	}
	info, err := os.Stat(target)
	if err != nil || !info.IsDir() {
		return "", false
	}
	return target, true
}

// isSubmoduleGitDir reports whether a resolved gitdir lives below a
// superproject's .git/modules directory, where Git stores every submodule
// repository, nested ones included.
func isSubmoduleGitDir(target string) bool {
	return strings.Contains(filepath.ToSlash(filepath.Clean(target)), "/.git/modules/")
}

// isGitDir reports whether path holds Git's repository metadata: a HEAD file
// plus objects and refs directories.
func isGitDir(path string) bool {
	if info, err := os.Stat(filepath.Join(path, "HEAD")); err != nil || info.IsDir() {
		return false
	}
	for _, dir := range []string{"objects", "refs"} {
		info, err := os.Stat(filepath.Join(path, dir))
		if err != nil || !info.IsDir() {
			return false
		}
	}
	return true
}

func DirFromGitDir(gitDir string) string {
	if gitDir == ".git" || gitDir == "./.git" {
		return "."
	}
	return strings.TrimSuffix(gitDir, string(filepath.Separator)+".git")
}

func DisplayName(repoDir string) string {
	trimmed := strings.TrimRight(repoDir, `/\`)
	if trimmed == "." {
		if cwd, err := os.Getwd(); err == nil {
			return filepath.Base(cwd)
		}
		return "."
	}
	return filepath.Base(trimmed)
}
