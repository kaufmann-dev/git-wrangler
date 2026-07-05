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
	if isBareRepository(path) {
		return Repository{}, fmt.Errorf("%s is a bare Git repository; --repo requires a working tree", path)
	}
	gitPath := filepath.Join(path, ".git")
	if info, err := os.Stat(gitPath); err == nil && info.IsDir() {
		return Repository{GitDir: gitPath, Dir: path, Display: DisplayName(path)}, nil
	}
	if validGitFile(gitPath) {
		return Repository{GitDir: gitPath, Dir: path, Display: DisplayName(path)}, nil
	}
	return Repository{}, fmt.Errorf("%s is not a Git repository", path)
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

func isBareRepository(path string) bool {
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
