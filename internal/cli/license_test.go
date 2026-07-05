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
