// Package store is a thin SQLite CRUD layer for bot state that must survive restarts.
package store

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"time"

	_ "modernc.org/sqlite"
)

type Store struct {
	db *sql.DB
}

const schema = `
CREATE TABLE IF NOT EXISTS group_settings (
  jid       TEXT PRIMARY KEY,
  antilink  INTEGER NOT NULL DEFAULT 0,
  welcome   TEXT NOT NULL DEFAULT '',
  goodbye   TEXT NOT NULL DEFAULT ''
);

CREATE TABLE IF NOT EXISTS warns (
  group_jid  TEXT NOT NULL,
  user_jid   TEXT NOT NULL,
  reason     TEXT NOT NULL,
  created_at INTEGER NOT NULL
);
CREATE INDEX IF NOT EXISTS warns_lookup ON warns(group_jid, user_jid);

CREATE TABLE IF NOT EXISTS blocked (
  user_jid   TEXT PRIMARY KEY,
  reason     TEXT NOT NULL DEFAULT '',
  created_at INTEGER NOT NULL
);

CREATE TABLE IF NOT EXISTS chat_history (
  id         INTEGER PRIMARY KEY AUTOINCREMENT,
  chat_jid   TEXT NOT NULL,
  role       TEXT NOT NULL,
  content    TEXT NOT NULL,
  created_at INTEGER NOT NULL
);
CREATE INDEX IF NOT EXISTS hist_by_chat ON chat_history(chat_jid, created_at DESC);

CREATE TABLE IF NOT EXISTS personas (
  chat_jid TEXT PRIMARY KEY,
  name     TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS ai_mode (
  chat_jid TEXT PRIMARY KEY,
  active   INTEGER NOT NULL DEFAULT 0
);

CREATE TABLE IF NOT EXISTS voices (
  name     TEXT PRIMARY KEY,
  voice_id TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS tts_voice (
  chat_jid TEXT PRIMARY KEY,
  voice    TEXT NOT NULL
);
`

// Open opens (or creates) the sqlite file at dir/state.db and applies the schema.
func Open(dir string) (*Store, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("mkdir: %w", err)
	}
	path := filepath.Join(dir, "state.db")
	dsn := fmt.Sprintf("file:%s?_pragma=foreign_keys(1)&_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)", path)
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("open: %w", err)
	}
	if _, err := db.ExecContext(context.Background(), schema); err != nil {
		db.Close()
		return nil, fmt.Errorf("schema: %w", err)
	}
	return &Store{db: db}, nil
}

func (s *Store) Close() error { return s.db.Close() }

// ---------- Group settings ----------

type GroupSettings struct {
	Antilink bool
	Welcome  string
	Goodbye  string
}

func (s *Store) Group(ctx context.Context, jid string) (GroupSettings, error) {
	var g GroupSettings
	var al int
	err := s.db.QueryRowContext(ctx,
		`SELECT antilink, welcome, goodbye FROM group_settings WHERE jid=?`, jid,
	).Scan(&al, &g.Welcome, &g.Goodbye)
	if err == sql.ErrNoRows {
		return GroupSettings{}, nil
	}
	if err != nil {
		return g, err
	}
	g.Antilink = al == 1
	return g, nil
}

func (s *Store) SetAntilink(ctx context.Context, jid string, on bool) error {
	v := 0
	if on {
		v = 1
	}
	_, err := s.db.ExecContext(ctx, `
INSERT INTO group_settings(jid, antilink) VALUES(?, ?)
ON CONFLICT(jid) DO UPDATE SET antilink=excluded.antilink`, jid, v)
	return err
}

func (s *Store) SetWelcome(ctx context.Context, jid, msg string) error {
	_, err := s.db.ExecContext(ctx, `
INSERT INTO group_settings(jid, welcome) VALUES(?, ?)
ON CONFLICT(jid) DO UPDATE SET welcome=excluded.welcome`, jid, msg)
	return err
}

func (s *Store) SetGoodbye(ctx context.Context, jid, msg string) error {
	_, err := s.db.ExecContext(ctx, `
INSERT INTO group_settings(jid, goodbye) VALUES(?, ?)
ON CONFLICT(jid) DO UPDATE SET goodbye=excluded.goodbye`, jid, msg)
	return err
}

// ---------- Warns ----------

func (s *Store) AddWarn(ctx context.Context, groupJID, userJID, reason string) (int, error) {
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO warns(group_jid, user_jid, reason, created_at) VALUES(?,?,?,?)`,
		groupJID, userJID, reason, time.Now().Unix())
	if err != nil {
		return 0, err
	}
	return s.CountWarns(ctx, groupJID, userJID)
}

func (s *Store) CountWarns(ctx context.Context, groupJID, userJID string) (int, error) {
	var n int
	err := s.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM warns WHERE group_jid=? AND user_jid=?`,
		groupJID, userJID).Scan(&n)
	return n, err
}

func (s *Store) ClearWarns(ctx context.Context, groupJID, userJID string) error {
	_, err := s.db.ExecContext(ctx,
		`DELETE FROM warns WHERE group_jid=? AND user_jid=?`, groupJID, userJID)
	return err
}

// ---------- Blocklist ----------

func (s *Store) Block(ctx context.Context, userJID, reason string) error {
	_, err := s.db.ExecContext(ctx, `
INSERT INTO blocked(user_jid, reason, created_at) VALUES(?,?,?)
ON CONFLICT(user_jid) DO UPDATE SET reason=excluded.reason`,
		userJID, reason, time.Now().Unix())
	return err
}

func (s *Store) Unblock(ctx context.Context, userJID string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM blocked WHERE user_jid=?`, userJID)
	return err
}

func (s *Store) IsBlocked(ctx context.Context, userJID string) bool {
	var one int
	err := s.db.QueryRowContext(ctx,
		`SELECT 1 FROM blocked WHERE user_jid=?`, userJID).Scan(&one)
	return err == nil
}

// ---------- Chat history ----------

type HistoryMsg struct {
	Role    string // "user" or "assistant"
	Content string
}

func (s *Store) AppendHistory(ctx context.Context, chatJID, role, content string) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO chat_history(chat_jid, role, content, created_at) VALUES(?,?,?,?)`,
		chatJID, role, content, time.Now().UnixNano())
	return err
}

// History returns the last n messages for a chat, oldest-first.
func (s *Store) History(ctx context.Context, chatJID string, n int) ([]HistoryMsg, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT role, content FROM chat_history WHERE chat_jid=? ORDER BY created_at DESC LIMIT ?`,
		chatJID, n)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []HistoryMsg
	for rows.Next() {
		var m HistoryMsg
		if err := rows.Scan(&m.Role, &m.Content); err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	for i, j := 0, len(out)-1; i < j; i, j = i+1, j-1 {
		out[i], out[j] = out[j], out[i]
	}
	return out, nil
}

func (s *Store) ClearHistory(ctx context.Context, chatJID string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM chat_history WHERE chat_jid=?`, chatJID)
	return err
}

// ---------- Personas ----------

func (s *Store) SetPersona(ctx context.Context, chatJID, name string) error {
	_, err := s.db.ExecContext(ctx, `
INSERT INTO personas(chat_jid, name) VALUES(?,?)
ON CONFLICT(chat_jid) DO UPDATE SET name=excluded.name`, chatJID, name)
	return err
}

func (s *Store) GetPersona(ctx context.Context, chatJID string) string {
	var name string
	_ = s.db.QueryRowContext(ctx, `SELECT name FROM personas WHERE chat_jid=?`, chatJID).Scan(&name)
	return name
}

// ---------- AI session mode ----------

func (s *Store) SetAIMode(ctx context.Context, chatJID string, on bool) error {
	v := 0
	if on {
		v = 1
	}
	_, err := s.db.ExecContext(ctx, `
INSERT INTO ai_mode(chat_jid, active) VALUES(?,?)
ON CONFLICT(chat_jid) DO UPDATE SET active=excluded.active`, chatJID, v)
	return err
}

func (s *Store) IsAIMode(ctx context.Context, chatJID string) bool {
	var v int
	_ = s.db.QueryRowContext(ctx,
		`SELECT active FROM ai_mode WHERE chat_jid=?`, chatJID).Scan(&v)
	return v == 1
}

// ---------- Voices ----------

func (s *Store) SetVoice(ctx context.Context, name, voiceID string) error {
	_, err := s.db.ExecContext(ctx, `
INSERT INTO voices(name, voice_id) VALUES(?,?)
ON CONFLICT(name) DO UPDATE SET voice_id=excluded.voice_id`, name, voiceID)
	return err
}

func (s *Store) GetVoice(ctx context.Context, name string) (string, bool) {
	var v string
	err := s.db.QueryRowContext(ctx,
		`SELECT voice_id FROM voices WHERE name=?`, name).Scan(&v)
	if err != nil {
		return "", false
	}
	return v, true
}

// ---------- Per-chat TTS voice ----------

func (s *Store) SetTTSVoice(ctx context.Context, chatJID, voice string) error {
	_, err := s.db.ExecContext(ctx, `
INSERT INTO tts_voice(chat_jid, voice) VALUES(?,?)
ON CONFLICT(chat_jid) DO UPDATE SET voice=excluded.voice`, chatJID, voice)
	return err
}

func (s *Store) GetTTSVoice(ctx context.Context, chatJID string) string {
	var v string
	_ = s.db.QueryRowContext(ctx,
		`SELECT voice FROM tts_voice WHERE chat_jid=?`, chatJID).Scan(&v)
	return v
}

func (s *Store) ListVoices(ctx context.Context) ([]string, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT name FROM voices ORDER BY name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var n string
		if err := rows.Scan(&n); err != nil {
			return nil, err
		}
		out = append(out, n)
	}
	return out, nil
}
