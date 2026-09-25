package httpapi

import (
	"net/http"
	"testing"
)

func TestLoginWrongPassword(t *testing.T) {
	env := newTestEnv(t)
	res := env.do(t, "POST", "/api/auth/login", map[string]string{"email": "reviewer@casereview.test", "password": "nope"})
	if res.StatusCode != http.StatusUnauthorized {
		t.Fatalf("want 401 got %d", res.StatusCode)
	}
}

func TestMeRequiresSession(t *testing.T) {
	env := newTestEnv(t)
	res, err := http.Get(env.url + "/api/auth/me")
	if err != nil {
		t.Fatal(err)
	}
	if res.StatusCode != http.StatusUnauthorized {
		t.Fatalf("anonymous: want 401 got %d", res.StatusCode)
	}
	me := decode[map[string]string](t, env.do(t, "GET", "/api/auth/me", nil))
	if me["name"] != "Maya Park" || me["title"] != "Senior Underwriter" {
		t.Fatalf("unexpected me %v", me)
	}
}

func TestLogoutEndsSession(t *testing.T) {
	env := newTestEnv(t)
	env.do(t, "POST", "/api/auth/logout", nil)
	if res := env.do(t, "GET", "/api/auth/me", nil); res.StatusCode != http.StatusUnauthorized {
		t.Fatalf("after logout want 401 got %d", res.StatusCode)
	}
}
