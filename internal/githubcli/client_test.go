package githubcli

import (
	"context"
	"errors"
	"io"
	"reflect"
	"strings"
	"testing"

	"github.com/kaufmann-dev/git-wrangler/internal/run"
)

type fakeRunner struct {
	run      func(ctx context.Context, dir string, env []string, name string, args ...string) (string, string, error)
	lookPath func(string) (string, error)
	pipe     func(context.Context, string, []string, run.Command, run.Command, func(io.Reader) error) error
}

func (f fakeRunner) Run(ctx context.Context, dir string, env []string, name string, args ...string) (string, string, error) {
	if f.run == nil {
		return "", "", errors.New("unexpected command")
	}
	return f.run(ctx, dir, env, name, args...)
}

func (f fakeRunner) LookPath(name string) (string, error) {
	if f.lookPath == nil {
		return "", errors.New("unexpected lookpath")
	}
	return f.lookPath(name)
}

func (f fakeRunner) Pipe(ctx context.Context, dir string, env []string, left run.Command, right run.Command, consume func(io.Reader) error) error {
	if f.pipe == nil {
		return errors.New("unexpected pipe")
	}
	return f.pipe(ctx, dir, env, left, right, consume)
}

func TestCaptureAndStdoutInvokeGh(t *testing.T) {
	var gotName, gotDir string
	var gotEnv, gotArgs []string
	client := New(fakeRunner{run: func(ctx context.Context, dir string, env []string, name string, args ...string) (string, string, error) {
		gotName, gotDir, gotEnv, gotArgs = name, dir, env, args
		return "output\n", "", nil
	}})

	out, err := client.CaptureEnv(context.Background(), "/repo", []string{"GH_TOKEN=tok"}, "repo", "view")
	if err != nil {
		t.Fatal(err)
	}
	if out != "output\n" || gotName != "gh" || gotDir != "/repo" {
		t.Fatalf("name=%q dir=%q out=%q", gotName, gotDir, out)
	}
	if !reflect.DeepEqual(gotArgs, []string{"repo", "view"}) || !reflect.DeepEqual(gotEnv, []string{"GH_TOKEN=tok"}) {
		t.Fatalf("args=%#v env=%#v", gotArgs, gotEnv)
	}

	out, err = client.StdoutEnv(context.Background(), "", nil, "api", "user")
	if err != nil || out != "output\n" || gotName != "gh" {
		t.Fatalf("StdoutEnv name=%q out=%q err=%v", gotName, out, err)
	}
}

func TestValidateAuthSendsUserQueryAndSucceeds(t *testing.T) {
	var gotArgs, gotEnv []string
	client := New(fakeRunner{run: func(ctx context.Context, dir string, env []string, name string, args ...string) (string, string, error) {
		gotArgs, gotEnv = args, env
		return "octocat\n", "", nil
	}})
	if err := client.ValidateAuth(context.Background(), []string{"GH_TOKEN=tok"}); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(gotArgs, []string{"api", "user", "-q", ".login"}) {
		t.Fatalf("args = %#v", gotArgs)
	}
	if !reflect.DeepEqual(gotEnv, []string{"GH_TOKEN=tok"}) {
		t.Fatalf("env = %#v", gotEnv)
	}
}

func TestValidateAuthPropagatesFailure(t *testing.T) {
	client := New(fakeRunner{run: func(ctx context.Context, dir string, env []string, name string, args ...string) (string, string, error) {
		return "", "gh: not logged in", errors.New("exit status 1")
	}})
	err := client.ValidateAuth(context.Background(), nil)
	if err == nil {
		t.Fatal("expected auth failure")
	}
	if !strings.Contains(err.Error(), "not logged in") {
		t.Fatalf("error = %v, want stderr detail", err)
	}
}

func TestEnvBuildsTokenAndHost(t *testing.T) {
	if got := Env("", ""); got != nil {
		t.Fatalf("empty token should yield nil, got %#v", got)
	}
	if got := Env("tok", ""); !reflect.DeepEqual(got, []string{"GH_TOKEN=tok"}) {
		t.Fatalf("token only = %#v", got)
	}
	if got := Env("tok", "git.example.com"); !reflect.DeepEqual(got, []string{"GH_TOKEN=tok", "GH_HOST=git.example.com"}) {
		t.Fatalf("token+host = %#v", got)
	}
}

func TestUnauthenticatedEnvBlanksInheritedTokens(t *testing.T) {
	want := []string{"GH_TOKEN=", "GITHUB_TOKEN=", "GH_ENTERPRISE_TOKEN=", "GITHUB_ENTERPRISE_TOKEN="}
	if got := UnauthenticatedEnv(); !reflect.DeepEqual(got, want) {
		t.Fatalf("UnauthenticatedEnv = %#v", got)
	}
}
