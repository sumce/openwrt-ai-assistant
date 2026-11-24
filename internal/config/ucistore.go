package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// UCIStore reads/writes the OpenWRT UCI config file /etc/config/wrtai
// directly (atomic tmp+rename). LuCI edits the same file for enabled/port,
// which is why we stick to the standard format.
//
// File layout:
//
//	config wrtai 'main'
//	        option enabled '1'
//	        option port '8765'
//	        option confirm_write '1'
//	        option default_provider 'p1'
//
//	config provider 'p1'
//	        option name 'DeepSeek'
//	        option base_url 'https://api.deepseek.com/v1'
//	        ...
type UCIStore struct {
	path string // /etc/config/wrtai
}

func NewUCIStore(path string) *UCIStore {
	return &UCIStore{path: path}
}

func (s *UCIStore) Load() (*Settings, error) {
	data, err := os.ReadFile(s.path)
	if os.IsNotExist(err) {
		return Default(), nil
	}
	if err != nil {
		return nil, err
	}
	return parseUCI(string(data))
}

func (s *UCIStore) Save(st *Settings) error {
	if err := os.MkdirAll(filepath.Dir(s.path), 0o755); err != nil {
		return err
	}
	tmp := s.path + ".tmp"
	if err := os.WriteFile(tmp, []byte(renderUCI(st)), 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, s.path)
}

func parseUCI(text string) (*Settings, error) {
	st := Default()
	st.Providers = nil

	var curType, curName string

	for _, raw := range strings.Split(text, "\n") {
		line := strings.TrimSpace(raw)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		switch {
		case strings.HasPrefix(line, "config "):
			fields := strings.Fields(strings.TrimPrefix(line, "config "))
			curType, curName = "", ""
			if len(fields) >= 1 {
				curType = unquote(fields[0])
			}
			if len(fields) >= 2 {
				curName = unquote(fields[1])
			}
			if curType == "provider" {
				st.Providers = append(st.Providers, Provider{ID: curName})
			}
		case strings.HasPrefix(line, "option "):
			rest := strings.TrimPrefix(line, "option ")
			key, val, ok := splitOption(rest)
			if !ok {
				continue
			}
			applyOption(st, curType, curName, key, val)
		}
	}
	return st, nil
}

func applyOption(st *Settings, secType, secName, key, val string) {
	if secType == "wrtai" || secName == "main" {
		switch key {
		case "enabled":
			st.Enabled = val == "1" || val == "true" || val == "on"
		case "port":
			if n, err := strconv.Atoi(val); err == nil {
				st.Port = n
			}
		case "listen":
			st.Listen = val
		case "confirm_write":
			st.ConfirmWrite = val == "1" || val == "true" || val == "on"
		case "default_provider":
			st.DefaultProvider = val
		}
		return
	}
	if secType == "provider" && len(st.Providers) > 0 {
		p := &st.Providers[len(st.Providers)-1]
		switch key {
		case "name":
			p.Name = val
		case "base_url":
			p.BaseURL = val
		case "api_key":
			p.APIKey = val
		case "model":
			p.Model = val
		}
	}
}

// splitOption parses `key 'value'` (value may contain spaces).
func splitOption(rest string) (string, string, bool) {
	rest = strings.TrimSpace(rest)
	idx := strings.IndexAny(rest, " \t")
	if idx < 0 {
		return rest, "", false
	}
	key := rest[:idx]
	val := strings.TrimSpace(rest[idx+1:])
	return key, unquote(val), true
}

func unquote(s string) string {
	if len(s) >= 2 && ((s[0] == '\'' && s[len(s)-1] == '\'') || (s[0] == '"' && s[len(s)-1] == '"')) {
		inner := s[1 : len(s)-1]
		// UCI escapes quotes inside single quotes as \' or '\'' sequences;
		// handle the common \' case.
		inner = strings.ReplaceAll(inner, `\'`, `'`)
		return inner
	}
	return s
}

func quote(s string) string {
	return "'" + strings.ReplaceAll(s, `'`, `'\''`) + "'"
}

func renderUCI(st *Settings) string {
	var b strings.Builder
	b.WriteString("# /etc/config/wrtai — WRT AI configuration\n")
	b.WriteString("# enabled/port are managed from LuCI (luci-app-wrtai)\n\n")
	b.WriteString("config wrtai 'main'\n")
	fmt.Fprintf(&b, "\toption enabled '%s'\n", boolStr(st.Enabled))
	fmt.Fprintf(&b, "\toption port '%d'\n", st.Port)
	fmt.Fprintf(&b, "\toption listen '%s'\n", quote(st.Listen))
	fmt.Fprintf(&b, "\toption confirm_write '%s'\n", boolStr(st.ConfirmWrite))
	fmt.Fprintf(&b, "\toption default_provider '%s'\n", quote(st.DefaultProvider))
	for _, p := range st.Providers {
		fmt.Fprintf(&b, "\nconfig provider '%s'\n", sanitizeSection(p.ID))
		fmt.Fprintf(&b, "\toption name %s\n", quote(p.Name))
		fmt.Fprintf(&b, "\toption base_url %s\n", quote(p.BaseURL))
		fmt.Fprintf(&b, "\toption api_key %s\n", quote(p.APIKey))
		fmt.Fprintf(&b, "\toption model %s\n", quote(p.Model))
	}
	return b.String()
}

func boolStr(b bool) string {
	if b {
		return "1"
	}
	return "0"
}

// sanitizeSection keeps UCI section names alphanumeric.
func sanitizeSection(s string) string {
	var b strings.Builder
	for _, r := range s {
		if r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '_' {
			b.WriteRune(r)
		} else {
			b.WriteRune('_')
		}
	}
	if b.Len() == 0 {
		return "p"
	}
	return b.String()
}
