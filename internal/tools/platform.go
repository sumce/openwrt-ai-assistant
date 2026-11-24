package tools

import (
	"context"
	"fmt"
)

// Op is one UCI mutation inside a set_config call.
type Op struct {
	Op      string `json:"op"`                          // set | add_section | delete_option | delete_section
	Config  string `json:"config"`                      // e.g. firewall
	Section string `json:"section,omitempty"`           // existing section name (or returned id for add)
	Option  string `json:"option,omitempty"`
	Type    string `json:"type,omitempty"`              // add_section: section type, e.g. rule
	Name    string `json:"name,omitempty"`              // add_section: optional named section id
	Value   string `json:"value,omitempty"`
}

// Platform abstracts the router system. Real implementation shells out to
// uci/ubus/logread; the mock keeps an in-memory fake router so the whole
// product can be developed and tested on a PC.
type Platform interface {
	SystemInfo(ctx context.Context) (string, error)
	NetworkStatus(ctx context.Context) (string, error)
	WirelessStatus(ctx context.Context) (string, error)
	Devices(ctx context.Context) (string, error)
	ReadLog(ctx context.Context, source string, lines int) (string, error)
	Diagnose(ctx context.Context, command, target string, count int) (string, error)
	UCIShow(ctx context.Context, path string) (string, error)
	UCIApply(ctx context.Context, ops []Op) (string, error)
	ExportConfig(ctx context.Context, config string) (string, error)
	ImportConfig(ctx context.Context, config, text string) (string, error)
	ServiceControl(ctx context.Context, service, action string) (string, error)
	ApplyConfig(ctx context.Context, configs []string) (string, error)
}

// DangerousConfigs are configs whose change can cut the administrator off
// or take down connectivity; mutations there are always approval-gated.
var DangerousConfigs = map[string]bool{
	"network":  true,
	"wireless": true,
	"firewall": true,
	"dhcp":     true,
}

// truncate caps tool output so a fat result cannot blow up the model
// context.
func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max] + fmt.Sprintf("\n...[输出过长，已截断，原始长度 %d 字节]", len(s))
}

const (
	capGeneral  = 4000
	capLogOrCfg = 6000
)
