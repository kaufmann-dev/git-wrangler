package cli

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLicenseMissingTypeFailsBeforeWriting(t *testing.T) {
	t.Setenv("NO_COLOR", "1")
	root := tempGitRepos(t, "repo")
	t.Chdir(root)

	var stdout, stderr bytes.Buffer
	err := ExecuteWithRunner(context.Background(), fakeRunner{lookPath: fakeGitLookPath}, []string{"license", "--name", "Ada"}, strings.NewReader(""), &stdout, &stderr)
	if err == nil || !strings.Contains(stderr.String(), "--type is required") {
		t.Fatalf("license missing type error = %v\nstdout: %s\nstderr: %s", err, stdout.String(), stderr.String())
	}
	if _, statErr := os.Stat(filepath.Join(root, "repo", "LICENSE")); !os.IsNotExist(statErr) {
		t.Fatalf("LICENSE was written despite missing type: %v", statErr)
	}
}

func TestLicenseEverySupportedTypeWritesLicense(t *testing.T) {
	t.Setenv("NO_COLOR", "1")
	for _, template := range licenseTemplates {
		t.Run(template.id, func(t *testing.T) {
			root := tempGitRepos(t, "repo")
			repoDir := filepath.Join(root, "repo")
			args := []string{"license", "--repo", repoDir, "--type", template.id, "--year", "2030", "--name", "Ada Lovelace", "--yes"}
			var stdout, stderr bytes.Buffer
			err := ExecuteWithRunner(context.Background(), fakeRunner{lookPath: fakeGitLookPath}, args, strings.NewReader(""), &stdout, &stderr)
			if err != nil {
				t.Fatalf("license %s returned error: %v\nstdout: %s\nstderr: %s", template.id, err, stdout.String(), stderr.String())
			}
			content, readErr := os.ReadFile(filepath.Join(repoDir, "LICENSE"))
			if readErr != nil {
				t.Fatal(readErr)
			}
			text := string(content)
			if firstLine := strings.SplitN(strings.TrimSpace(text), "\n", 2)[0]; firstLine == "" {
				t.Fatalf("license %s has empty title:\n%s", template.id, text)
			}
			if template.requiresHolder {
				for _, want := range []string{"2030", "Ada Lovelace"} {
					if !strings.Contains(text, want) {
						t.Fatalf("license %s missing %q:\n%s", template.id, want, text)
					}
				}
			} else if strings.Contains(text, "Ada Lovelace") {
				t.Fatalf("holder-free license %s used --name:\n%s", template.id, text)
			}
			if !strings.Contains(stdout.String(), "Summary: 1 created, 0 overwritten, 0 skipped, 0 failed") {
				t.Fatalf("missing summary:\nstdout: %s\nstderr: %s", stdout.String(), stderr.String())
			}
		})
	}
}

func TestLicenseZeroClauseBSDUsesOfficialTextAndRequiresHolder(t *testing.T) {
	t.Setenv("NO_COLOR", "1")
	root := tempGitRepos(t, "repo")
	repoDir := filepath.Join(root, "repo")

	var stdout, stderr bytes.Buffer
	err := ExecuteWithRunner(context.Background(), fakeRunner{lookPath: fakeGitLookPath}, []string{"license", "--repo", repoDir, "--type", "0bsd", "--year", "2030", "--name", "Ada Lovelace"}, strings.NewReader(""), &stdout, &stderr)
	if err != nil {
		t.Fatalf("0bsd license returned error: %v\nstdout: %s\nstderr: %s", err, stdout.String(), stderr.String())
	}
	content, readErr := os.ReadFile(filepath.Join(repoDir, "LICENSE"))
	if readErr != nil {
		t.Fatal(readErr)
	}
	text := string(content)
	for _, want := range []string{
		"BSD Zero Clause License",
		"Copyright (C) 2030 by Ada Lovelace",
		"Permission to use, copy, modify, and/or distribute this software",
		`THE SOFTWARE IS PROVIDED "AS IS"`,
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("0bsd license missing %q:\n%s", want, text)
		}
	}

	missingRoot := tempGitRepos(t, "missing-holder")
	stdout.Reset()
	stderr.Reset()
	err = ExecuteWithRunner(context.Background(), fakeRunner{lookPath: fakeGitLookPath}, []string{"license", "--repo", filepath.Join(missingRoot, "missing-holder"), "--type", "0bsd", "--year", "2030"}, strings.NewReader(""), &stdout, &stderr)
	if err == nil || !strings.Contains(stderr.String(), "--name is required") {
		t.Fatalf("0bsd missing holder error = %v\nstdout: %s\nstderr: %s", err, stdout.String(), stderr.String())
	}
}

func TestLicenseCopyrightBearingTypesRequireName(t *testing.T) {
	t.Setenv("NO_COLOR", "1")
	root := tempGitRepos(t, "repo")
	t.Chdir(root)

	var stdout, stderr bytes.Buffer
	err := ExecuteWithRunner(context.Background(), fakeRunner{lookPath: fakeGitLookPath}, []string{"license", "--type", "mit", "--year", "2030", "--yes"}, strings.NewReader(""), &stdout, &stderr)
	if err == nil || !strings.Contains(stderr.String(), "--name is required") {
		t.Fatalf("license missing name error = %v\nstdout: %s\nstderr: %s", err, stdout.String(), stderr.String())
	}
}

func TestLicenseHolderFreeTypesDoNotRequireName(t *testing.T) {
	t.Setenv("NO_COLOR", "1")
	root := tempGitRepos(t, "repo")
	t.Chdir(root)

	var stdout, stderr bytes.Buffer
	err := ExecuteWithRunner(context.Background(), fakeRunner{lookPath: fakeGitLookPath}, []string{"license", "--type", "unlicense"}, strings.NewReader(""), &stdout, &stderr)
	if err != nil {
		t.Fatalf("holder-free license returned error: %v\nstdout: %s\nstderr: %s", err, stdout.String(), stderr.String())
	}
	content, readErr := os.ReadFile(filepath.Join(root, "repo", "LICENSE"))
	if readErr != nil {
		t.Fatal(readErr)
	}
	if !strings.Contains(string(content), "The Unlicense") {
		t.Fatalf("unexpected license content:\n%s", string(content))
	}
}

func TestLicenseInvalidTypeListsSupportedIDs(t *testing.T) {
	t.Setenv("NO_COLOR", "1")
	var stdout, stderr bytes.Buffer
	err := ExecuteWithRunner(context.Background(), fakeRunner{lookPath: fakeGitLookPath}, []string{"license", "--type", "bad", "--name", "Ada"}, strings.NewReader(""), &stdout, &stderr)
	if err == nil || !strings.Contains(stderr.String(), "Supported types: apache-2.0, gpl-3.0, mit") || !strings.Contains(stderr.String(), "unlicense") {
		t.Fatalf("invalid type error = %v\nstdout: %s\nstderr: %s", err, stdout.String(), stderr.String())
	}
}

func TestLicenseOverwriteConfirmationIsAggregate(t *testing.T) {
	t.Setenv("NO_COLOR", "1")
	root := tempGitRepos(t, "one", "two")
	t.Chdir(root)
	if err := os.WriteFile(filepath.Join(root, "one", "LICENSE"), []byte("old one\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "two", "LICENSE"), []byte("old two\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	var stdout, stderr bytes.Buffer
	err := executeInteractive(t, context.Background(), fakeRunner{lookPath: fakeGitLookPath}, []string{"license", "--type", "mit", "--name", "Ada", "--year", "2030", "--overwrite"}, strings.NewReader("n\n"), &stdout, &stderr)
	if err != nil {
		t.Fatalf("declined overwrite returned error: %v\nstdout: %s\nstderr: %s", err, stdout.String(), stderr.String())
	}
	if strings.Count(stderr.String(), "Overwrite existing LICENSE files in 2 repositories?") != 1 {
		t.Fatalf("expected one aggregate confirmation:\n%s", stderr.String())
	}
	if !strings.Contains(stdout.String(), "Summary: 0 created, 0 overwritten, 2 skipped, 0 failed") {
		t.Fatalf("missing declined summary:\n%s", stdout.String())
	}

	stdout.Reset()
	stderr.Reset()
	err = ExecuteWithRunner(context.Background(), fakeRunner{lookPath: fakeGitLookPath}, []string{"license", "--type", "mit", "--name", "Ada", "--year", "2030", "--overwrite", "--yes"}, strings.NewReader(""), &stdout, &stderr)
	if err != nil {
		t.Fatalf("overwrite --yes returned error: %v\nstdout: %s\nstderr: %s", err, stdout.String(), stderr.String())
	}
	if !strings.Contains(stdout.String(), "Summary: 0 created, 2 overwritten, 0 skipped, 0 failed") {
		t.Fatalf("missing overwrite summary:\n%s", stdout.String())
	}
}

func TestLicenseRemoveDeletesOnlyExactLicense(t *testing.T) {
	t.Setenv("NO_COLOR", "1")
	root := tempGitRepos(t, "repo")
	repoDir := filepath.Join(root, "repo")
	for name, content := range map[string]string{
		"LICENSE":     "main license\n",
		"LICENSE.md":  "markdown license\n",
		"LICENSE.txt": "text license\n",
		"COPYING":     "copying license\n",
	} {
		if err := os.WriteFile(filepath.Join(repoDir, name), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	var stdout, stderr bytes.Buffer
	err := ExecuteWithRunner(context.Background(), fakeRunner{lookPath: fakeGitLookPath}, []string{"license", "remove", "--repo", repoDir, "--yes"}, strings.NewReader(""), &stdout, &stderr)
	if err != nil {
		t.Fatalf("license remove returned error: %v\nstdout: %s\nstderr: %s", err, stdout.String(), stderr.String())
	}
	if _, statErr := os.Lstat(filepath.Join(repoDir, "LICENSE")); !os.IsNotExist(statErr) {
		t.Fatalf("LICENSE still exists after removal: %v", statErr)
	}
	for _, name := range []string{"LICENSE.md", "LICENSE.txt", "COPYING"} {
		if _, statErr := os.Stat(filepath.Join(repoDir, name)); statErr != nil {
			t.Fatalf("%s was removed: %v", name, statErr)
		}
	}
	if !strings.Contains(stdout.String(), "Summary: 1 removed, 0 skipped, 0 failed") {
		t.Fatalf("unexpected removal summary:\nstdout: %s\nstderr: %s", stdout.String(), stderr.String())
	}
}

func TestLicenseRemoveConfirmationIsAggregate(t *testing.T) {
	t.Setenv("NO_COLOR", "1")
	root := tempGitRepos(t, "one", "two")
	t.Chdir(root)
	for _, name := range []string{"one", "two"} {
		if err := os.WriteFile(filepath.Join(root, name, "LICENSE"), []byte(name+" license\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	var stdout, stderr bytes.Buffer
	err := executeInteractive(t, context.Background(), fakeRunner{lookPath: fakeGitLookPath}, []string{"license", "remove"}, strings.NewReader("n\n"), &stdout, &stderr)
	if err != nil {
		t.Fatalf("declined removal returned error: %v\nstdout: %s\nstderr: %s", err, stdout.String(), stderr.String())
	}
	if strings.Count(stderr.String(), "Remove LICENSE files from 2 repositories?") != 1 {
		t.Fatalf("expected one aggregate confirmation:\n%s", stderr.String())
	}
	if !strings.Contains(stdout.String(), "Summary: 0 removed, 2 skipped, 0 failed") {
		t.Fatalf("unexpected declined summary:\n%s", stdout.String())
	}
	for _, name := range []string{"one", "two"} {
		if _, statErr := os.Stat(filepath.Join(root, name, "LICENSE")); statErr != nil {
			t.Fatalf("%s LICENSE changed after decline: %v", name, statErr)
		}
	}

	stdout.Reset()
	stderr.Reset()
	err = ExecuteWithRunner(context.Background(), fakeRunner{lookPath: fakeGitLookPath}, []string{"license", "remove", "--yes"}, strings.NewReader(""), &stdout, &stderr)
	if err != nil {
		t.Fatalf("confirmed removal returned error: %v\nstdout: %s\nstderr: %s", err, stdout.String(), stderr.String())
	}
	if !strings.Contains(stdout.String(), "Summary: 2 removed, 0 skipped, 0 failed") {
		t.Fatalf("unexpected confirmed summary:\n%s", stdout.String())
	}
}

func TestLicenseRemoveMissingFileSkipsWithoutConfirmation(t *testing.T) {
	t.Setenv("NO_COLOR", "1")
	root := tempGitRepos(t, "repo")
	repoDir := filepath.Join(root, "repo")

	var stdout, stderr bytes.Buffer
	err := ExecuteWithRunner(context.Background(), fakeRunner{lookPath: fakeGitLookPath}, []string{"license", "remove", "--repo", repoDir}, strings.NewReader(""), &stdout, &stderr)
	if err != nil {
		t.Fatalf("missing LICENSE removal returned error: %v\nstdout: %s\nstderr: %s", err, stdout.String(), stderr.String())
	}
	if strings.Contains(stderr.String(), "Remove LICENSE files") {
		t.Fatalf("missing LICENSE prompted for confirmation:\n%s", stderr.String())
	}
	if !strings.Contains(stdout.String(), "SKIP repo: LICENSE does not exist") || !strings.Contains(stdout.String(), "Summary: 0 removed, 1 skipped, 0 failed") {
		t.Fatalf("unexpected missing LICENSE output:\nstdout: %s\nstderr: %s", stdout.String(), stderr.String())
	}
}

func TestLicenseRemoveGuidedPromptsForRepository(t *testing.T) {
	t.Setenv("NO_COLOR", "1")
	root := tempGitRepos(t, "repo")
	repoDir := filepath.Join(root, "repo")
	if err := os.WriteFile(filepath.Join(repoDir, "LICENSE"), []byte("license\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	var stdout, stderr bytes.Buffer
	err := executeInteractive(t, context.Background(), fakeRunner{lookPath: fakeGitLookPath}, []string{"license", "remove", "--guided", "--yes"}, strings.NewReader(repoDir+"\n"), &stdout, &stderr)
	if err != nil {
		t.Fatalf("guided license removal returned error: %v\nstdout: %s\nstderr: %s", err, stdout.String(), stderr.String())
	}
	if !strings.Contains(stderr.String(), "Repository:") || !strings.Contains(stderr.String(), "Selected configuration") {
		t.Fatalf("guided removal did not prompt and summarize repository:\n%s", stderr.String())
	}
	if _, statErr := os.Stat(filepath.Join(repoDir, "LICENSE")); !os.IsNotExist(statErr) {
		t.Fatalf("guided removal left LICENSE behind: %v", statErr)
	}
}

func TestLicenseRemoveRequiresConfirmationAndRejectsDirectory(t *testing.T) {
	t.Setenv("NO_COLOR", "1")

	t.Run("confirmation", func(t *testing.T) {
		root := tempGitRepos(t, "repo")
		repoDir := filepath.Join(root, "repo")
		if err := os.WriteFile(filepath.Join(repoDir, "LICENSE"), []byte("license\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		var stdout, stderr bytes.Buffer
		err := ExecuteWithRunner(context.Background(), fakeRunner{lookPath: fakeGitLookPath}, []string{"license", "remove", "--repo", repoDir}, strings.NewReader(""), &stdout, &stderr)
		if err == nil || !strings.Contains(stderr.String(), "pass --yes") {
			t.Fatalf("noninteractive removal error = %v\nstdout: %s\nstderr: %s", err, stdout.String(), stderr.String())
		}
		if _, statErr := os.Stat(filepath.Join(repoDir, "LICENSE")); statErr != nil {
			t.Fatalf("LICENSE changed without confirmation: %v", statErr)
		}
	})

	t.Run("directory", func(t *testing.T) {
		root := tempGitRepos(t, "repo")
		repoDir := filepath.Join(root, "repo")
		if err := os.Mkdir(filepath.Join(repoDir, "LICENSE"), 0o755); err != nil {
			t.Fatal(err)
		}
		var stdout, stderr bytes.Buffer
		err := ExecuteWithRunner(context.Background(), fakeRunner{lookPath: fakeGitLookPath}, []string{"license", "remove", "--repo", repoDir, "--yes"}, strings.NewReader(""), &stdout, &stderr)
		if err == nil || !strings.Contains(stderr.String(), "LICENSE is a directory, not a file") {
			t.Fatalf("directory removal error = %v\nstdout: %s\nstderr: %s", err, stdout.String(), stderr.String())
		}
		if !strings.Contains(stdout.String(), "Summary: 0 removed, 0 skipped, 1 failed") {
			t.Fatalf("unexpected directory summary:\n%s", stdout.String())
		}
		if info, statErr := os.Stat(filepath.Join(repoDir, "LICENSE")); statErr != nil || !info.IsDir() {
			t.Fatalf("LICENSE directory was changed: info=%v err=%v", info, statErr)
		}
	})
}
