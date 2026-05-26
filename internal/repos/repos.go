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
			if d.Name() == ".git" && validGitFile(path) {
				dir := filepath.Dir(path)
				found = append(found, Repository{
					GitDir:  path,
					Dir:     dir,
					Display: DisplayName(dir),
				})
				return filepath.SkipDir
			}
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

func validGitFile(path string) bool {
	data, err := os.ReadFile(path)
	if err != nil {
		return false
	}
	target, ok := strings.CutPrefix(strings.TrimSpace(string(data)), "gitdir:")
	if !ok {
		return false
	}
	target = strings.TrimSpace(target)
	if target == "" {
		return false
	}
	if !filepath.IsAbs(target) {
		target = filepath.Join(filepath.Dir(path), target)
	}
	info, err := os.Stat(target)
	return err == nil && info.IsDir()
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
