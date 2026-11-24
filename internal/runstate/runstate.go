// Package runstate persists agent runs to disk so approval requests survive
// server restarts: a run parked in waiting_approval can be approved after
// the process comes back.
package runstate

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/sumce/openwrt-ai-assistant/internal/llm"
)

// PendingAction is a tool call waiting for the user's decision.
type PendingAction struct {
	ToolCallID  string          `json:"tool_call_id"`
	Tool        string          `json:"tool"`
	Args        json.RawMessage `json:"args"`
	Level       string          `json:"level"`
	Description string          `json:"description"`
	Configs     []string        `json:"configs,omitempty"` // configs to snapshot before executing
}

// Persisted is the on-disk shape of a run.
type Persisted struct {
	ID             string          `json:"id"`
	ConversationID string          `json:"conversation_id"`
	State          string          `json:"state"`
	Messages       []llm.Message   `json:"messages"`
	Pending        *PendingAction  `json:"pending,omitempty"`
	Error          string          `json:"error,omitempty"`
	UpdatedAt      int64           `json:"updated_at"`
}

type Store struct {
	dir string
}

func New(dir string) (*Store, error) {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, err
	}
	return &Store{dir: dir}, nil
}

func (s *Store) path(id string) string {
	return filepath.Join(s.dir, id+".json")
}

func (s *Store) Save(p *Persisted) error {
	p.UpdatedAt = nowUnix()
	data, err := json.MarshalIndent(p, "", "  ")
	if err != nil {
		return err
	}
	tmp := s.path(p.ID) + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, s.path(p.ID))
}

func (s *Store) Load(id string) (*Persisted, error) {
	data, err := os.ReadFile(s.path(id))
	if err != nil {
		return nil, err
	}
	var p Persisted
	if err := json.Unmarshal(data, &p); err != nil {
		return nil, fmt.Errorf("解析 run %s: %w", id, err)
	}
	return &p, nil
}

// LoadAll returns every persisted run.
func (s *Store) LoadAll() ([]*Persisted, error) {
	entries, err := os.ReadDir(s.dir)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var out []*Persisted
	for _, e := range entries {
		if e.IsDir() || filepath.Ext(e.Name()) != ".json" {
			continue
		}
		id := filepath.Base(e.Name())
		p, err := s.Load(id[:len(id)-len(".json")])
		if err == nil {
			out = append(out, p)
		}
	}
	return out, nil
}

func (s *Store) Delete(id string) error {
	err := os.Remove(s.path(id))
	if os.IsNotExist(err) {
		return nil
	}
	return err
}

func nowUnix() int64 { return time.Now().Unix() }
