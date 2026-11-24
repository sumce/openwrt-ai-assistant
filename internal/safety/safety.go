// Package safety implements the graded guardrails: deciding whether a tool
// call may run directly or needs user approval, plus config backup/restore.
package safety

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/sumce/openwrt-ai-assistant/internal/tools"
)

type Decision int

const (
	Allow Decision = iota
	NeedApproval
)

// Decide maps a tool's risk level + user setting to a decision.
// Dangerous ops always need approval; write ops respect the setting.
func Decide(level tools.Level, confirmWrite bool) Decision {
	switch level {
	case tools.LevelRead:
		return Allow
	case tools.LevelWrite:
		if confirmWrite {
			return NeedApproval
		}
		return Allow
	default: // LevelDangerous
		return NeedApproval
	}
}

// ConfigsToBackup extracts the unique config names an operation touches,
// so the caller can snapshot them before approval.
func ConfigsToBackup(ops []tools.Op) []string {
	seen := map[string]bool{}
	var out []string
	for _, op := range ops {
		if !seen[op.Config] {
			seen[op.Config] = true
			out = append(out, op.Config)
		}
	}
	sort.Strings(out)
	return out
}

// ---- backups ----

type BackupInfo struct {
	Name   string `json:"name"`
	Config string `json:"config"`
	Size   int64  `json:"size"`
	Time   int64  `json:"time"`
}

const maxBackups = 30

var backupNameRe = regexp.MustCompile(`^(\d{8}-\d{6})_([A-Za-z0-9_-]+)\.uci$`)

// Backups stores uci export snapshots before risky mutations.
type Backups struct {
	mu       sync.Mutex
	dir      string
	platform tools.Platform
}

func NewBackups(dir string, p tools.Platform) *Backups {
	return &Backups{dir: dir, platform: p}
}

// Create snapshots one config; returns the backup file name.
func (b *Backups) Create(ctx context.Context, config string) (string, error) {
	text, err := b.platform.ExportConfig(ctx, config)
	if err != nil {
		return "", fmt.Errorf("导出配置 %s 失败: %w", config, err)
	}
	if err := os.MkdirAll(b.dir, 0o700); err != nil {
		return "", err
	}
	name := fmt.Sprintf("%s_%s.uci", time.Now().Format("20060102-150405"), config)
	path := filepath.Join(b.dir, name)
	if err := os.WriteFile(path, []byte(text), 0o600); err != nil {
		return "", err
	}
	b.prune()
	return name, nil
}

// List returns all backups, newest first.
func (b *Backups) List() ([]BackupInfo, error) {
	entries, err := os.ReadDir(b.dir)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var out []BackupInfo
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		m := backupNameRe.FindStringSubmatch(e.Name())
		if m == nil {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		t, _ := time.ParseInLocation("20060102-150405", m[1], time.Local)
		out = append(out, BackupInfo{
			Name:   e.Name(),
			Config: m[2],
			Size:   info.Size(),
			Time:   t.Unix(),
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Time > out[j].Time })
	return out, nil
}

// Restore imports a backup file back into UCI.
func (b *Backups) Restore(ctx context.Context, name string) (string, error) {
	if strings.Contains(name, "/") || strings.Contains(name, "\\") || strings.Contains(name, "..") {
		return "", fmt.Errorf("非法备份文件名")
	}
	m := backupNameRe.FindStringSubmatch(name)
	if m == nil {
		return "", fmt.Errorf("非法备份文件名")
	}
	config := m[2]
	data, err := os.ReadFile(filepath.Join(b.dir, name))
	if err != nil {
		return "", err
	}
	return b.platform.ImportConfig(ctx, config, string(data))
}

func (b *Backups) prune() {
	entries, err := os.ReadDir(b.dir)
	if err != nil {
		return
	}
	type item struct {
		name string
		t    time.Time
	}
	var items []item
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		m := backupNameRe.FindStringSubmatch(e.Name())
		if m == nil {
			continue
		}
		t, _ := time.ParseInLocation("20060102-150405", m[1], time.Local)
		items = append(items, item{e.Name(), t})
	}
	if len(items) <= maxBackups {
		return
	}
	sort.Slice(items, func(i, j int) bool { return items[i].t.Before(items[j].t) })
	for _, it := range items[:len(items)-maxBackups] {
		_ = os.Remove(filepath.Join(b.dir, it.name))
	}
}
