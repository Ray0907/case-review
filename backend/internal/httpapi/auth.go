package httpapi

import (
	"context"
	"net/http"
	"time"

	"tidalwave/backend/internal/auth"
	"tidalwave/backend/internal/store"
)

const sessionCookie = "tw_session"

type ctxKey struct{}

func userFrom(r *http.Request) store.User { return r.Context().Value(ctxKey{}).(store.User) }

func (s *Server) requireUser(h http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		c, err := r.Cookie(sessionCookie)
		if err != nil {
			writeError(w, http.StatusUnauthorized, "sign in required")
			return
		}
		u, err := s.store.UserBySession(c.Value)
		if err != nil {
			writeError(w, http.StatusUnauthorized, "session expired, sign in again")
			return
		}
		h(w, r.WithContext(context.WithValue(r.Context(), ctxKey{}, u)))
	}
}

func (s *Server) login(w http.ResponseWriter, r *http.Request) {
	var in struct{ Email, Password string }
	if err := readJSON(r, &in); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	u, hash, err := s.store.UserByEmail(in.Email)
	if err != nil || !auth.CheckPassword(hash, in.Password) {
		writeError(w, http.StatusUnauthorized, "email or password is incorrect")
		return
	}
	token := auth.NewToken()
	if err := s.store.CreateSession(u.ID, token, 12*time.Hour); err != nil {
		writeError(w, http.StatusInternalServerError, "could not start session")
		return
	}
	http.SetCookie(w, &http.Cookie{Name: sessionCookie, Value: token, Path: "/", HttpOnly: true,
		SameSite: http.SameSiteLaxMode, MaxAge: int((12 * time.Hour).Seconds())})
	writeJSON(w, http.StatusOK, u)
}

func (s *Server) logout(w http.ResponseWriter, r *http.Request) {
	if c, err := r.Cookie(sessionCookie); err == nil {
		_ = s.store.DeleteSession(c.Value)
	}
	http.SetCookie(w, &http.Cookie{Name: sessionCookie, Value: "", Path: "/", MaxAge: -1, HttpOnly: true})
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) me(w http.ResponseWriter, r *http.Request) { writeJSON(w, http.StatusOK, userFrom(r)) }
