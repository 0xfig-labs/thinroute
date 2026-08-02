package apikeys

import (
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"fmt"
	"strings"
	"time"
)

type Key struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	Key       string    `json:"key"`
	CreatedAt time.Time `json:"created_at"`
	LastUsed  time.Time `json:"last_used,omitempty"`
}

type Store struct{ db *sql.DB }

func New(db *sql.DB) (*Store, error) {
	if db == nil {
		return nil, fmt.Errorf("database connection is required")
	}
	_, err := db.Exec(`CREATE TABLE IF NOT EXISTS api_keys (
		id TEXT PRIMARY KEY, name TEXT NOT NULL, key TEXT NOT NULL UNIQUE,
		created_at DATETIME NOT NULL, last_used_at DATETIME
	)`)
	if err != nil {
		return nil, fmt.Errorf("create api_keys table: %w", err)
	}
	return &Store{db: db}, nil
}

func (s *Store) Create(name string) (Key, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		name = "default"
	}
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return Key{}, err
	}
	now := time.Now().UTC()
	k := Key{ID: "key_" + hex.EncodeToString(buf[:8]), Name: name, Key: "tr_" + hex.EncodeToString(buf), CreatedAt: now}
	_, err := s.db.Exec(`INSERT INTO api_keys(id,name,key,created_at) VALUES(?,?,?,?)`, k.ID, k.Name, k.Key, now)
	if err != nil {
		return Key{}, fmt.Errorf("create api key: %w", err)
	}
	return k, nil
}

func (s *Store) List() ([]Key, error) {
	rows, err := s.db.Query(`SELECT id,name,key,created_at,COALESCE(last_used_at, '') FROM api_keys ORDER BY created_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Key
	for rows.Next() {
		var k Key
		var last string
		if err := rows.Scan(&k.ID, &k.Name, &k.Key, &k.CreatedAt, &last); err != nil {
			return nil, err
		}
		if last != "" {
			k.LastUsed, _ = time.Parse(time.RFC3339Nano, last)
		}
		out = append(out, k)
	}
	return out, rows.Err()
}

func (s *Store) Revoke(id string) error {
	res, err := s.db.Exec(`DELETE FROM api_keys WHERE id = ?`, id)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return fmt.Errorf("api key %q not found", id)
	}
	return nil
}

func (s *Store) Authenticate(value string) bool {
	value = strings.TrimSpace(value)
	if value == "" {
		return false
	}
	var id string
	err := s.db.QueryRow(`SELECT id FROM api_keys WHERE key = ?`, value).Scan(&id)
	if err != nil {
		return false
	}
	_, _ = s.db.Exec(`UPDATE api_keys SET last_used_at = ? WHERE id = ?`, time.Now().UTC(), id)
	return true
}
