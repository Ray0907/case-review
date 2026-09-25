package auth

import "testing"

func TestHashAndCheck(t *testing.T) {
	h, err := HashPassword("s3cret")
	if err != nil {
		t.Fatal(err)
	}
	if !CheckPassword(h, "s3cret") || CheckPassword(h, "wrong") {
		t.Fatal("password check mismatch")
	}
}

func TestNewTokenUnique(t *testing.T) {
	a, b := NewToken(), NewToken()
	if a == b || len(a) != 64 {
		t.Fatalf("bad tokens %q %q", a, b)
	}
}
