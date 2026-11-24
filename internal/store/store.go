// Package store persists chat conversations as JSONL files, one file per
// conversation, plus an index for listing.
package store

import (
	"bufio"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

// Entry is one display-oriented item in a conversation transcript.
// Types: user | assistant | tool | approval | error.
type Entry struct {
	Type     string `json:"t"`
	Text     string `json:"text,omitempty"`
	Tool     string `json:"tool,omitempty"`
	Desc     string `json:"desc,omitempty"`
	Result   string `json:"result,omitempty"`
	Level    string `json:"level,omitempty"`
	Approved *bool  `json:"approved,omitempty"`
	Ts       int64  `json:"ts"`
}

type Conversation struct {
	ID      string `json:"id"`
	Title   string `json:"title"`
	Created int64  `json:"created"`
	Updated int64  `json:"updated"`
}

type index struct {
	Convs map[string]*Conversation `json:"convs"`
}

type Store struct {
	mu    sync.Mutex
	dir   string
	idx   *index
	dirty bool
}

func New(dir string) (*Store, error) {
	s := &Store{dir: dir, idx: &index{Convs: map[string]*Conversation{}}}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, err
	}
	s.loadIndex()
	return s, nil
}

func (s *Store) loadIndex() {
	data, err := os.ReadFile(s.indexPath())
	if err != nil {
		return
	}
	_ = json.Unmarshal(data, s.idx)
}

func (s *Store) saveIndexLocked() {
	data, err := json.MarshalIndent(s.idx, "", "  ")
	if err != nil {
		return
	}
	tmp := s.indexPath() + ".tmp"
	if os.WriteFile(tmp, data, 0o600) == nil {
		_ = os.Rename(tmp, s.indexPath())
	}
}

func (s *Store) indexPath() string  { return filepath.Join(s.dir, "index.json") }
func (s *Store) convPath(id string) string {
	return filepath.Join(s.dir, id+".jsonl")
}

func newID() string {
	b := make([]byte, 8)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

// Create starts a new conversation.
func (s *Store) Create(title string) (*Conversation, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := time.Now().Unix()
	c := &Conversation{ID: newID(), Title: title, Created: now, Updated: now}
	s.idx.Convs[c.ID] = c
	s.saveIndexLocked()
	return c, nil
}

func (s *Store) Get(id string) (*Conversation, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	c, ok := s.idx.Convs[id]
	if !ok {
		return nil, fmt.Errorf("会话不存在")
	}
	return c, nil
}

// List returns conversations, newest first.
func (s *Store) List() ([]Conversation, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]Conversation, 0, len(s.idx.Convs))
	for _, c := range s.idx.Convs {
		out = append(out, *c)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Updated > out[j].Updated })
	return out, nil
}

func (s *Store) Delete(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.idx.Convs[id]; !ok {
		return fmt.Errorf("会话不存在")
	}
	delete(s.idx.Convs, id)
	s.saveIndexLocked()
	return os.Remove(s.convPath(id))
}

// Append appends one transcript entry and touches the conversation.
func (s *Store) Append(id string, e Entry) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	c, ok := s.idx.Convs[id]
	if !ok {
		return fmt.Errorf("会话不存在")
	}
	e.Ts = time.Now().Unix()
	f, err := os.OpenFile(s.convPath(id), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return err
	}
	defer f.Close()
	data, err := json.Marshal(e)
	if err != nil {
		return err
	}
	if _, err := f.Write(append(data, '\n')); err != nil {
		return err
	}
	c.Updated = e.Ts
	s.saveIndexLocked()
	return nil
}

// Entries returns the full transcript of a conversation in order.
func (s *Store) Entries(id string) ([]Entry, error) {
	s.mu.Lock()
	_, ok := s.idx.Convs[id]
	s.mu.Unlock()
	if !ok {
		return nil, fmt.Errorf("会话不存在")
	}
	f, err := os.Open(s.convPath(id))
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	defer f.Close()
	var out []Entry
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" {
			continue
		}
		var e Entry
		if json.Unmarshal([]byte(line), &e) == nil {
			out = append(out, e)
		}
	}
	return out, sc.Err()
}

// TruncateTitle makes a conversation title from the first user message.
func TruncateTitle(text string) string {
	text = strings.TrimSpace(strings.Join(strings.Fields(text), " "))
	r := []rune(text)
	if len(r) > 24 {
		return string(r[:24]) + "…"
	}
	if text == "" {
		return "新会话"
	}
	return text
}
