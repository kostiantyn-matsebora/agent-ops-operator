package chat

import "testing"

func TestDeriveAdapterToken(t *testing.T) {
	a := DeriveAdapterToken("master-key", "telegram")

	// stateless: same inputs always re-derive the same token (manager restarts
	// must keep validating previously issued tokens with no stored state)
	if b := DeriveAdapterToken("master-key", "telegram"); b != a {
		t.Fatalf("derivation is not deterministic: %s vs %s", a, b)
	}
	// scoped: different adapter names and different master keys diverge
	if DeriveAdapterToken("master-key", "slack") == a {
		t.Fatal("different adapter names derived the same token")
	}
	if DeriveAdapterToken("other-key", "telegram") == a {
		t.Fatal("different master keys derived the same token")
	}
	// never the bare master key, and non-empty
	if a == "" || a == "master-key" {
		t.Fatalf("degenerate token %q", a)
	}
}
