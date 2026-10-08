package access_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/jonaskahn/relo/internal/access"
)

func TestMintAndParse(t *testing.T) {
	t.Run("a minted token round-trips", func(t *testing.T) {
		id, err := access.NewID()
		if err != nil {
			t.Fatalf("NewID() error = %v", err)
		}
		token, err := access.Mint(id)
		if err != nil {
			t.Fatalf("Mint() error = %v", err)
		}
		if !strings.HasPrefix(token.Raw, access.Prefix) {
			t.Fatalf("Raw = %q, want the relo prefix", token.Raw)
		}
		parsedID, secret, err := access.Parse(token.Raw)
		if err != nil {
			t.Fatalf("Parse() error = %v", err)
		}
		if parsedID != id || secret != token.Secret {
			t.Fatalf("Parse() = %q, %q, want %q, %q", parsedID, secret, id, token.Secret)
		}
		if token.Digest != access.Digest(token.Secret) {
			t.Fatal("the digest does not cover the secret")
		}
		if len(token.Digest) != 64 {
			t.Fatalf("digest length = %d, want a sha256 hex digest", len(token.Digest))
		}
	})

	t.Run("two minted tokens never repeat", func(t *testing.T) {
		id, err := access.NewID()
		if err != nil {
			t.Fatalf("NewID() error = %v", err)
		}
		first, err := access.Mint(id)
		if err != nil {
			t.Fatalf("Mint() error = %v", err)
		}
		second, err := access.Mint(id)
		if err != nil {
			t.Fatalf("Mint() error = %v", err)
		}
		if first.Secret == second.Secret || first.Digest == second.Digest {
			t.Fatal("two minted tokens share a secret")
		}
	})

	t.Run("the hint identifies a secret without being one", func(t *testing.T) {
		id, err := access.NewID()
		if err != nil {
			t.Fatalf("NewID() error = %v", err)
		}
		token, err := access.Mint(id)
		if err != nil {
			t.Fatalf("Mint() error = %v", err)
		}
		if token.Hint == token.Secret {
			t.Fatal("the hint is the secret")
		}
		if !strings.Contains(token.Hint, "...") {
			t.Fatalf("hint = %q, want the elided middle", token.Hint)
		}
		if strings.Contains(token.Hint, token.Secret[6:len(token.Secret)-4]) {
			t.Fatal("the hint carries the middle of the secret")
		}
	})
}

func TestParseRefusesJunk(t *testing.T) {
	for _, raw := range []string{
		"", "   ", "rlo_ak_", "rlo_ak_x", "rlo_ak_zzzz_abc", "sk-live-abc",
		"rlo_ak_" + strings.Repeat("z", 32) + "_secret", "bearer rlo_ak_ab_xy",
	} {
		t.Run(raw, func(t *testing.T) {
			if _, _, err := access.Parse(raw); !errors.Is(err, access.ErrMalformed) {
				t.Fatalf("Parse(%q) error = %v, want ErrMalformed", raw, err)
			}
		})
	}
}

// TestHintNeverShortensASecret covers the short-secret branch: a hint of a
// value too small to elide is fully masked rather than partly shown.
func TestHintNeverShortensASecret(t *testing.T) {
	for _, secret := range []string{"", "a", "abc", "abcde"} {
		hint := access.Hint(secret)
		if hint != strings.Repeat("*", len(secret)) {
			t.Fatalf("Hint(%q) = %q, want it fully masked", secret, hint)
		}
	}
	if hint := access.Hint("abcdefgh"); hint != "abcd...gh" {
		t.Fatalf("Hint() = %q, want the masked middle", hint)
	}
}

func TestKnownClientIDs(t *testing.T) {
	list := access.KnownClientIDs()
	if list == "" {
		t.Fatal("KnownClientIDs() is empty")
	}
	for _, id := range access.ClientIDs() {
		if !strings.Contains(list, id) {
			t.Fatalf("KnownClientIDs() = %q, want it to name %s", list, id)
		}
	}
}

// TestParseToleratesASecretWithTheSeparator covers the base64url alphabet,
// which shares `_` with the separator: only the first one splits the token.
func TestParseToleratesASecretWithTheSeparator(t *testing.T) {
	id := strings.Repeat("ab", access.IDBytes)
	secret := "a_b-c_d"
	parsedID, parsedSecret, err := access.Parse(access.Prefix + id + "_" + secret)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	if parsedID != id || parsedSecret != secret {
		t.Fatalf("Parse() = %q, %q, want %q, %q", parsedID, parsedSecret, id, secret)
	}
}

func TestClientCatalog(t *testing.T) {
	t.Run("every profile is reachable by identifier", func(t *testing.T) {
		for _, id := range access.ClientIDs() {
			if _, found := access.ClientProfileFor(id); !found {
				t.Fatalf("ClientProfileFor(%q) found nothing", id)
			}
		}
	})

	t.Run("the custom client is not a profile", func(t *testing.T) {
		if _, found := access.ClientProfileFor(access.CustomClient); found {
			t.Fatal("the custom client resolved to a profile")
		}
	})

	t.Run("a profile fills its steps in", func(t *testing.T) {
		profile, found := access.ClientProfileFor("codex")
		if !found {
			t.Fatal("the codex profile is missing")
		}
		fill := access.SetupFill{Token: "rlo_ak_x_y", BaseURL: "http://127.0.0.1:10100", Model: "gpt-4o"}
		steps := profile.Instructions(fill)
		joined := strings.Join(steps, "\n")
		if !strings.Contains(joined, fill.Token) || !strings.Contains(joined, fill.BaseURL) {
			t.Fatalf("steps = %q, want the token and the address", joined)
		}
		for _, step := range steps {
			if strings.Contains(step, "{{") {
				t.Fatalf("step %q kept a placeholder", step)
			}
		}
		if !strings.Contains(profile.Verification(fill), "gpt-4o") {
			t.Fatalf("verify = %q, want the model", profile.Verification(fill))
		}
	})
}
