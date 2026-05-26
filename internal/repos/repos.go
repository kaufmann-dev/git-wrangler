package repos

import (
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
			return nil
		}
		if d.Name() == ".git" {
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

func DirFromGitDir(gitDir string) string {
	if gitDir == ".git" || gitDir == "./.git" {
		return "."
	}
	return strings.TrimSuffix(gitDir, string(filepath.Separator)+".git")
}

func DisplayName(repoDir string) string {
	trimmed := strings.TrimRight(repoDir, string(filepath.Separator)+"/")
	if trimmed == "." {
		if cwd, err := os.Getwd(); err == nil {
			return filepath.Base(cwd)
		}
		return "."
	}
	return filepath.Base(trimmed)
}
