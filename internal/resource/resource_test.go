package resource

import "testing"

func TestInferType(t *testing.T) {
	cases := map[string]string{
		"credentials.github.work": TypeCredential, // type-first
		"resend.credentials.key":  TypeCredential, // entity-first
		"resend.credential.key":   TypeCredential, // singular
		"jason.birthday":          TypeDate,
		"people.jason.birthday":   TypeDate,
		"github.tokens.ci":        TypeToken,
		"links.mongodb.prod":      TypeLink,
		"random.thing":            TypeNote, // fallback
	}
	for key, want := range cases {
		if got := InferType(key); got != want {
			t.Errorf("InferType(%q) = %q, want %q", key, got, want)
		}
	}
}

func TestValidateKey(t *testing.T) {
	for _, ok := range []string{"a", "a.b-c.d_e", "creds.x9"} {
		if err := ValidateKey(ok); err != nil {
			t.Errorf("ValidateKey(%q) unexpected error: %v", ok, err)
		}
	}
	for _, bad := range []string{"", "A.b", "a..b", ".a", "a.", "a b"} {
		if err := ValidateKey(bad); err == nil {
			t.Errorf("ValidateKey(%q) expected error", bad)
		}
	}
}

func TestAncestors(t *testing.T) {
	got := Ancestors("a.b.c")
	if len(got) != 2 || got[0] != "a" || got[1] != "a.b" {
		t.Errorf("Ancestors(a.b.c) = %v", got)
	}
}
