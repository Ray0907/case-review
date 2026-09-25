package store

import (
	"database/sql"
	"errors"
	"time"

	"tidalwave/backend/internal/auth"
)

var ErrNotFound = errors.New("not found")

type User struct {
	ID    string `json:"id"`
	Email string `json:"email"`
	Name  string `json:"name"`
	Title string `json:"title"`
}

func (s *Store) EnsureUser(email, name, title, hash string) error {
	_, err := s.db.Exec(`INSERT INTO users (id, email, name, title, password_hash) VALUES (?, ?, ?, ?, ?)
		ON CONFLICT(email) DO NOTHING`, auth.NewToken()[:16], email, name, title, hash)
	return err
}

func (s *Store) UserByEmail(email string) (User, string, error) {
	var u User
	var hash string
	err := s.db.QueryRow(`SELECT id, email, name, title, password_hash FROM users WHERE email = ?`, email).
		Scan(&u.ID, &u.Email, &u.Name, &u.Title, &hash)
	if errors.Is(err, sql.ErrNoRows) {
		return u, "", ErrNotFound
	}
	return u, hash, err
}

func (s *Store) CreateSession(userID, token string, ttl time.Duration) error {
	_, err := s.db.Exec(`INSERT INTO sessions (token, user_id, expires_at) VALUES (?, ?, ?)`,
		token, userID, time.Now().Add(ttl).Unix())
	return err
}

func (s *Store) UserBySession(token string) (User, error) {
	var u User
	err := s.db.QueryRow(`SELECT u.id, u.email, u.name, u.title FROM sessions s JOIN users u ON u.id = s.user_id
		WHERE s.token = ? AND s.expires_at > ?`, token, time.Now().Unix()).Scan(&u.ID, &u.Email, &u.Name, &u.Title)
	if errors.Is(err, sql.ErrNoRows) {
		return u, ErrNotFound
	}
	return u, err
}

func (s *Store) DeleteSession(token string) error {
	_, err := s.db.Exec(`DELETE FROM sessions WHERE token = ?`, token)
	return err
}
