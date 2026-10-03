package auth

import "testing"

func TestPasswordHashVerifiesOnlyMatchingPassword(t *testing.T) {
	hash, err := HashPassword("demo-password")
	if err != nil {
		t.Fatal(err)
	}
	if !VerifyPassword(hash, "demo-password") {
		t.Fatal("expected correct password to verify")
	}
	if VerifyPassword(hash, "wrong-password") {
		t.Fatal("wrong password verified")
	}
	if VerifyPassword("plaintext-password", "plaintext-password") {
		t.Fatal("plaintext was accepted as a password hash")
	}
}

func TestPasswordHashRejectsMalformedAndOutOfRangeParameters(t *testing.T) {
	for _, encoded := range []string{
		"",
		"$argon2id$v=19$m=999999999,t=3,p=1$YWFhYWFhYWFhYWFhYWFhYQ$YWFhYWFhYWFhYWFhYWFhYWFhYWFhYWFhYWFhYWFhYWE",
		"$argon2id$v=19$m=65536,t=3,p=1$m?not-base64$YWFhYWFhYWFhYWFhYWFhYWFhYWFhYWFhYWFhYWFhYWE",
	} {
		if VerifyPassword(encoded, "anything") {
			t.Errorf("malformed hash unexpectedly verified: %q", encoded)
		}
	}
}
