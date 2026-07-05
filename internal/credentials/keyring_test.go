package credentials

import (
	"errors"
	"testing"

	"github.com/zalando/go-keyring"
)

func TestKeyringStoreRoundTrip(t *testing.T) {
	keyring.MockInit()
	store := NewKeyringStore()

	if err := store.Set("acct", "s3cret"); err != nil {
		t.Fatal(err)
	}
	got, err := store.Get("acct")
	if err != nil || got != "s3cret" {
		t.Fatalf("Get = %q, %v", got, err)
	}
	if err := store.Delete("acct"); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Get("acct"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("Get after delete = %v, want ErrNotFound", err)
	}
	if err := store.Delete("acct"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("Delete of missing = %v, want ErrNotFound", err)
	}
}

func TestKeyringAvailableWithMock(t *testing.T) {
	keyring.MockInit()
	if !KeyringAvailable(NewKeyringStore()) {
		t.Fatal("mock keyring should report available")
	}
	if KeyringAvailable(nil) {
		t.Fatal("nil store must not be available")
	}
}

func TestAccountBuilders(t *testing.T) {
	if got := GitHubAccount("github.com"); got != "github:github.com" {
		t.Fatalf("GitHubAccount = %q", got)
	}
	if got := AIAccount("openai"); got != "ai:openai" {
		t.Fatalf("AIAccount = %q", got)
	}
	if got := AIHeaderAccount("openai", "X-Api-Key"); got != "ai-header:openai:X-Api-Key" {
		t.Fatalf("AIHeaderAccount = %q", got)
	}
}
