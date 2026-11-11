package git

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/kaufmann-dev/git-wrangler/internal/run"
)

type Comparison struct {
	Branch string `json:"branch"`
	Target string `json:"target"`
	State  string `json:"state"`
	Method string `json:"method"`
	Ahead  *int   `json:"ahead"`
	Behind *int   `json:"behind"`
	Base   string `json:"-"`
}

func number(n int) *int { return &n }

func (c Client) CompareOrigin(ctx context.Context, dir string) (Comparison, error) {
	result := Comparison{State: "unavailable"}
	branch, stderr, err := c.runner.Run(ctx, dir, nil, "git", "symbolic-ref", "--quiet", "--short", "HEAD")
	if err != nil {
		if strings.TrimSpace(stderr) == "" && exitCode(err) == 1 {
			result.State = "detached"
			return result, nil
		}
		return result, fmt.Errorf("inspect branch: %s: %w", stderr, err)
	}
	result.Branch = strings.TrimSpace(branch)
	if result.Branch == "" {
		return result, fmt.Errorf("empty branch name")
	}
	result.Target = "origin/" + result.Branch
	ref := "refs/remotes/" + result.Target
	_, err = c.Stdout(ctx, dir, nil, "rev-parse", "--verify", "--quiet", "HEAD")
	if err != nil {
		if exitCode(err) != 1 {
			return result, err
		}
		result.State = "unborn"
		result.Ahead = number(0)
		result.Behind = number(0)
		return result, nil
	}
	_, stderr, err = c.runner.Run(ctx, dir, nil, "git", "show-ref", "--verify", "--quiet", ref)
	if err != nil {
		if exitCode(err) != 1 {
			return result, fmt.Errorf("inspect origin ref: %s: %w", stderr, err)
		}
		count, e := c.Stdout(ctx, dir, nil, "rev-list", "--count", "HEAD")
		if e != nil {
			return result, e
		}
		n, e := strconv.Atoi(strings.TrimSpace(count))
		if e != nil {
			return result, e
		}
		result.State = "unpublished"
		result.Method = "empty tree"
		result.Ahead = number(n)
		result.Behind = number(0)
		result.Base, e = c.EmptyTree(ctx, dir)
		return result, e
	}
	counts, err := c.Stdout(ctx, dir, nil, "rev-list", "--left-right", "--count", "HEAD..."+ref)
	if err != nil {
		return result, err
	}
	fields := strings.Fields(counts)
	if len(fields) != 2 {
		return result, fmt.Errorf("invalid ahead/behind counts")
	}
	a, e := strconv.Atoi(fields[0])
	if e != nil {
		return result, e
	}
	b, e := strconv.Atoi(fields[1])
	if e != nil {
		return result, e
	}
	result.Ahead = number(a)
	result.Behind = number(b)
	result.State = "compared"
	result.Method = "merge base"
	base, stderr, err := c.runner.Run(ctx, dir, nil, "git", "merge-base", ref, "HEAD")
	if err != nil {
		if exitCode(err) != 1 {
			return result, fmt.Errorf("find merge base: %s: %w", stderr, err)
		}
		result.Method = "tree comparison; no common ancestor"
		base = ref
	}
	result.Base = strings.TrimSpace(base)
	return result, nil
}

type exitCoder interface{ ExitCode() int }

func exitCode(err error) int {
	if e, ok := err.(exitCoder); ok {
		return e.ExitCode()
	}
	return -1
}
func (c Client) EmptyTree(ctx context.Context, dir string) (string, error) {
	out, err := c.Stdout(run.WithStdin(ctx, ""), dir, nil, "hash-object", "-t", "tree", "--stdin")
	return strings.TrimSpace(out), err
}

type FileChange struct {
	Status       string `json:"status"`
	Path         string `json:"path"`
	PreviousPath string `json:"previousPath,omitempty"`
	Additions    *int   `json:"additions"`
	Deletions    *int   `json:"deletions"`
	Binary       bool   `json:"binary"`
}

// DiffChanges joins NUL-delimited status and numstat records by destination path.
func (c Client) DiffChanges(ctx context.Context, dir string, revisions ...string) ([]FileChange, error) {
	args := []string{"diff", "--no-ext-diff", "--no-textconv", "--find-renames", "--name-status", "-z"}
	args = append(args, revisions...)
	args = append(args, "--")
	names, err := c.Stdout(ctx, dir, nil, args...)
	if err != nil {
		return nil, err
	}
	args[4] = "--numstat"
	stats, err := c.Stdout(ctx, dir, nil, args...)
	if err != nil {
		return nil, err
	}
	return parseChanges(names, stats)
}
func parseChanges(names, stats string) ([]FileChange, error) {
	files := []FileChange{}
	index := map[string]int{}
	parts := strings.Split(strings.TrimSuffix(names, "\x00"), "\x00")
	for i := 0; i < len(parts) && parts[i] != ""; {
		status := parts[i]
		i++
		if i >= len(parts) {
			return nil, fmt.Errorf("truncated file status")
		}
		f := FileChange{Status: status[:1], Path: parts[i], Additions: number(0), Deletions: number(0)}
		i++
		if f.Status == "R" || f.Status == "C" {
			if i >= len(parts) {
				return nil, fmt.Errorf("truncated rename")
			}
			f.PreviousPath = f.Path
			f.Path = parts[i]
			i++
		}
		if f.Status == "U" {
			f.Additions = nil
			f.Deletions = nil
		}
		if pos, ok := index[f.Path]; ok {
			if files[pos].Status != "U" {
				files[pos] = f
			}
			continue
		}
		index[f.Path] = len(files)
		files = append(files, f)
	}
	parts = strings.Split(strings.TrimSuffix(stats, "\x00"), "\x00")
	for i := 0; i < len(parts) && parts[i] != ""; i++ {
		fields := strings.SplitN(parts[i], "\t", 3)
		if len(fields) != 3 {
			return nil, fmt.Errorf("invalid numstat")
		}
		path := fields[2]
		if path == "" {
			if i+2 >= len(parts) {
				return nil, fmt.Errorf("truncated numstat rename")
			}
			path = parts[i+2]
			i += 2
		}
		pos, ok := index[path]
		if !ok {
			return nil, fmt.Errorf("diff changed while inspecting %q", path)
		}
		f := &files[pos]
		if f.Status == "U" {
			continue
		}
		if fields[0] == "-" || fields[1] == "-" {
			f.Binary = true
			f.Additions = nil
			f.Deletions = nil
			continue
		}
		a, e := strconv.Atoi(fields[0])
		if e != nil {
			return nil, e
		}
		d, e := strconv.Atoi(fields[1])
		if e != nil {
			return nil, e
		}
		f.Additions = number(a)
		f.Deletions = number(d)
	}
	return files, nil
}
func (c Client) UntrackedChanges(ctx context.Context, dir string) ([]FileChange, error) {
	out, err := c.Stdout(ctx, dir, nil, "ls-files", "--others", "--exclude-standard", "-z")
	if err != nil {
		return nil, err
	}
	files := []FileChange{}
	for _, path := range strings.Split(out, "\x00") {
		if path == "" {
			continue
		}
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		full := filepath.Join(dir, filepath.FromSlash(path))
		info, err := os.Lstat(full)
		if err != nil {
			return nil, err
		}
		f := FileChange{Status: "A", Path: path, Additions: number(0), Deletions: number(0)}
		var reader io.Reader
		var file *os.File
		if info.Mode()&os.ModeSymlink != 0 {
			target, e := os.Readlink(full)
			if e != nil {
				return nil, e
			}
			reader = strings.NewReader(target)
		} else if info.Mode().IsRegular() {
			file, err = os.Open(full)
			if err != nil {
				return nil, err
			}
			reader = file
		} else {
			f.Additions = nil
			f.Deletions = nil
			files = append(files, f)
			continue
		}
		count, binary, e := countNewLines(ctx, reader)
		if file != nil {
			closeErr := file.Close()
			if e == nil {
				e = closeErr
			}
		}
		if e != nil {
			return nil, e
		}
		if binary {
			f.Binary = true
			f.Additions = nil
			f.Deletions = nil
		} else {
			f.Additions = number(count)
		}

		files = append(files, f)
	}
	return files, nil
}

func countNewLines(ctx context.Context, r io.Reader) (int, bool, error) {
	buffer := make([]byte, 32768)
	count, total := 0, 0
	var last byte
	for {
		if err := ctx.Err(); err != nil {
			return 0, false, err
		}
		n, err := r.Read(buffer)
		if n > 0 {
			sample := n
			if total+sample > 8000 {
				sample = 8000 - total
			}
			if sample > 0 && bytes.IndexByte(buffer[:sample], 0) >= 0 {
				return 0, true, nil
			}
			count += bytes.Count(buffer[:n], []byte{'\n'})
			total += n
			last = buffer[n-1]
		}
		if err == io.EOF {
			break
		}
		if err != nil {
			return 0, false, err
		}
	}
	if total > 0 && last != '\n' {
		count++
	}
	return count, false, nil
}
