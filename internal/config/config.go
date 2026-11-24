// Package config holds runtime settings for WRT AI.
//
// Two storage backends implement the same Store interface:
//   - UCIStore: production, reads/writes /etc/config/wrtai (the file LuCI
//     also edits for enable/port).
//   - JSONStore: development (also used when -dev is set), a plain JSON file
//     under the data directory.
package config

// Provider is one upstream LLM endpoint speaking the OpenAI protocol.
type Provider struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	BaseURL string `json:"base_url"` // API root, e.g. https://api.deepseek.com/v1
	APIKey  string `json:"api_key"`
	Model   string `json:"model"`
}

// Settings is the full persisted configuration.
type Settings struct {
	Enabled         bool       `json:"enabled"`
	Port            int        `json:"port"`
	Listen          string     `json:"listen"`
	ConfirmWrite    bool       `json:"confirm_write"` // write-level ops need approval (dangerous always do)
	DefaultProvider string     `json:"default_provider"`
	Providers       []Provider `json:"providers"`
}

func Default() *Settings {
	return &Settings{
		Enabled:      true,
		Port:         8765,
		Listen:       "0.0.0.0",
		ConfirmWrite: true,
	}
}

// Provider returns the provider with the given id.
func (s *Settings) Provider(id string) *Provider {
	for i := range s.Providers {
		if s.Providers[i].ID == id {
			return &s.Providers[i]
		}
	}
	return nil
}

// DefaultProvider resolves the currently selected provider; falls back to
// the first one when the configured id no longer exists.
func (s *Settings) DefaultProviderCfg() *Provider {
	if p := s.Provider(s.DefaultProvider); p != nil {
		return p
	}
	if len(s.Providers) > 0 {
		return &s.Providers[0]
	}
	return nil
}

// Store persists and reloads Settings.
type Store interface {
	Load() (*Settings, error)
	Save(*Settings) error
}
