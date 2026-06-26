package cli

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/kaufmann-dev/git-wrangler/internal/config"
	"github.com/kaufmann-dev/git-wrangler/internal/credentials"
)

func TestInvalidAIAuthFailsBeforeCommandWork(t *testing.T) {
	for _, command := range []string{"commit", "rewrite-commits"} {
		t.Run(command, func(t *testing.T) {
			t.Setenv("NO_COLOR", "1")
			t.Setenv("XDG_CONFIG_HOME", t.TempDir())
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusUnauthorized)
				_, _ = w.Write([]byte("invalid API key"))
			}))
			defer server.Close()

			cfg := config.Defaults()
			cfg.AI.BaseURL = server.URL
			cfg.AI.Model = "gpt-test"
			if err := config.Save(cfg); err != nil {
				t.Fatal(err)
			}

			lookedUp := false
			runner := fakeRunner{
				lookPath: func(name string) (string, error) {
					lookedUp = true
					return "", errors.New("command lookup should not run")
				},
			}
			var stdout, stderr bytes.Buffer
			a := newApp(context.Background(), runner, strings.NewReader(""), &stdout, &stderr)
			a.creds = &fakeCredentialStore{values: map[string]string{credentials.AIAccount("openai"): "bad-key"}}
			cmd := newRootCommand(a)
			cmd.SetArgs([]string{command})
			cmd.SetIn(a.stdin)
			cmd.SetOut(&stdout)
			cmd.SetErr(&stderr)

			if err := cmd.Execute(); err == nil {
				t.Fatal("expected AI authentication failure")
			}
			if lookedUp {
				t.Fatal("command work began before AI authentication failed")
			}
			if stdout.Len() != 0 {
				t.Fatalf("unexpected stdout before failure:\n%s", stdout.String())
			}
			if !strings.Contains(stderr.String(), "AI API validation failed: HTTP 401: invalid API key") {
				t.Fatalf("unexpected stderr:\n%s", stderr.String())
			}
		})
	}
}
