package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/sumce/openwrt-ai-assistant/internal/llm"
)

// Level classifies how risky a tool is.
type Level int

const (
	LevelRead Level = iota // auto-execute
	LevelWrite             // approval needed by default (configurable)
	LevelDangerous         // approval mandatory + auto backup, never skippable
)

func (l Level) String() string {
	switch l {
	case LevelRead:
		return "read"
	case LevelWrite:
		return "write"
	default:
		return "dangerous"
	}
}

// Tool pairs a definition (schema + metadata) with its executor.
type Tool struct {
	Name        string
	Description string
	Parameters  json.RawMessage
	Level       Level
	// LevelFor allows dynamic classification (set_config is dangerous when
	// it touches network/wireless/firewall/dhcp). nil → static Level.
	LevelFor func(args json.RawMessage) Level
	// Describe renders a human readable action summary for approval cards.
	Describe func(args json.RawMessage) (string, error)

	Exec func(ctx context.Context, args json.RawMessage) (string, error)
}

// Registry holds every available tool.
type Registry struct {
	order []string
	byName map[string]*Tool
}

func (r *Registry) Get(name string) *Tool { return r.byName[name] }
func (r *Registry) Names() []string       { return r.order }

// LLMDefs renders the registry as OpenAI tool definitions.
func (r *Registry) LLMDefs() []llm.ToolDef {
	defs := make([]llm.ToolDef, 0, len(r.order))
	for _, name := range r.order {
		t := r.byName[name]
		defs = append(defs, llm.ToolDef{
			Type: "function",
			Function: llm.ToolFunction{
				Name:        t.Name,
				Description: t.Description,
				Parameters:  t.Parameters,
			},
		})
	}
	return defs
}

// ResolveLevel returns the effective level for a call.
func ResolveLevel(t *Tool, args json.RawMessage) Level {
	if t.LevelFor != nil {
		return t.LevelFor(args)
	}
	return t.Level
}

// NewRegistry builds the standard toolset on top of a Platform.
func NewRegistry(p Platform) *Registry {
	r := &Registry{byName: map[string]*Tool{}}
	add := func(t *Tool) {
		r.order = append(r.order, t.Name)
		r.byName[t.Name] = t
	}

	add(&Tool{
		Name:        "get_system_info",
		Description: "获取路由器系统信息：型号、固件版本、运行时间、内存与负载、磁盘占用",
		Parameters:  json.RawMessage(`{"type":"object","properties":{},"additionalProperties":false}`),
		Level:       LevelRead,
		Describe:    func(json.RawMessage) (string, error) { return "读取系统信息", nil },
		Exec: func(ctx context.Context, _ json.RawMessage) (string, error) {
			return p.SystemInfo(ctx)
		},
	})

	add(&Tool{
		Name:        "get_network_status",
		Description: "获取网络接口状态：WAN/LAN 的 IP、网关、DNS、连接方式",
		Parameters:  json.RawMessage(`{"type":"object","properties":{},"additionalProperties":false}`),
		Level:       LevelRead,
		Describe:    func(json.RawMessage) (string, error) { return "读取网络接口状态", nil },
		Exec: func(ctx context.Context, _ json.RawMessage) (string, error) {
			return p.NetworkStatus(ctx)
		},
	})

	add(&Tool{
		Name:        "get_wireless_status",
		Description: "获取无线状态：radio/SSID/信道/加密方式/已连接客户端",
		Parameters:  json.RawMessage(`{"type":"object","properties":{},"additionalProperties":false}`),
		Level:       LevelRead,
		Describe:    func(json.RawMessage) (string, error) { return "读取无线状态", nil },
		Exec: func(ctx context.Context, _ json.RawMessage) (string, error) {
			return p.WirelessStatus(ctx)
		},
	})

	add(&Tool{
		Name:        "list_devices",
		Description: "列出当前连接到路由器的设备（DHCP 租约 + ARP 邻居表，含 IP/MAC/主机名）",
		Parameters:  json.RawMessage(`{"type":"object","properties":{},"additionalProperties":false}`),
		Level:       LevelRead,
		Describe:    func(json.RawMessage) (string, error) { return "列出在线设备", nil },
		Exec: func(ctx context.Context, _ json.RawMessage) (string, error) {
			return p.Devices(ctx)
		},
	})

	add(&Tool{
		Name:        "read_log",
		Description: "读取系统日志尾部。source: logread(系统日志,默认) 或 dmesg(内核日志)",
		Parameters: json.RawMessage(`{
			"type":"object",
			"properties":{
				"source":{"type":"string","enum":["logread","dmesg"],"description":"日志来源，默认 logread"},
				"lines":{"type":"integer","description":"读取行数，1-100，默认 50"}
			},
			"additionalProperties":false
		}`),
		Level: LevelRead,
		Describe: func(args json.RawMessage) (string, error) {
			var a struct {
				Source string `json:"source"`
				Lines  int    `json:"lines"`
			}
			if err := json.Unmarshal(args, &a); err != nil {
				return "", err
			}
			if a.Source == "" {
				a.Source = "logread"
			}
			return fmt.Sprintf("读取日志 %s 最后 %d 行", a.Source, clampLines(a.Lines)), nil
		},
		Exec: func(ctx context.Context, args json.RawMessage) (string, error) {
			var a struct {
				Source string `json:"source"`
				Lines  int    `json:"lines"`
			}
			if err := json.Unmarshal(args, &a); err != nil {
				return "", err
			}
			return p.ReadLog(ctx, a.Source, clampLines(a.Lines))
		},
	})

	add(&Tool{
		Name:        "run_diagnostics",
		Description: "运行网络诊断命令：ping / traceroute / nslookup",
		Parameters: json.RawMessage(`{
			"type":"object",
			"properties":{
				"command":{"type":"string","enum":["ping","traceroute","nslookup"],"description":"诊断命令"},
				"target":{"type":"string","description":"目标主机名或 IP"},
				"count":{"type":"integer","description":"ping 次数，1-10，默认 4"}
			},
			"required":["command","target"],
			"additionalProperties":false
		}`),
		Level: LevelRead,
		Describe: func(args json.RawMessage) (string, error) {
			var a struct {
				Command string `json:"command"`
				Target  string `json:"target"`
			}
			if err := json.Unmarshal(args, &a); err != nil {
				return "", err
			}
			return fmt.Sprintf("运行诊断: %s %s", a.Command, a.Target), nil
		},
		Exec: func(ctx context.Context, args json.RawMessage) (string, error) {
			var a struct {
				Command string `json:"command"`
				Target  string `json:"target"`
				Count   int    `json:"count"`
			}
			if err := json.Unmarshal(args, &a); err != nil {
				return "", err
			}
			return p.Diagnose(ctx, a.Command, a.Target, clampCount(a.Count))
		},
	})

	add(&Tool{
		Name:        "get_config",
		Description: "读取 UCI 配置。path 可为空(全部)、配置名(如 firewall)或 配置名.段落(如 network.lan)",
		Parameters: json.RawMessage(`{
			"type":"object",
			"properties":{
				"path":{"type":"string","description":"如 firewall 或 network.lan，留空读取全部"}
			},
			"additionalProperties":false
		}`),
		Level: LevelRead,
		Describe: func(args json.RawMessage) (string, error) {
			var a struct {
				Path string `json:"path"`
			}
			if err := json.Unmarshal(args, &a); err != nil {
				return "", err
			}
			if a.Path == "" {
				return "读取全部 UCI 配置", nil
			}
			return "读取配置 " + a.Path, nil
		},
		Exec: func(ctx context.Context, args json.RawMessage) (string, error) {
			var a struct {
				Path string `json:"path"`
			}
			if err := json.Unmarshal(args, &a); err != nil {
				return "", err
			}
			return p.UCIShow(ctx, a.Path)
		},
	})

	add(&Tool{
		Name:        "set_config",
		Description: "修改 UCI 配置（批量原子操作，最后统一 commit）。op: set 修改选项 / add_section 新增段落(type+可选name) / delete_option / delete_section。修改 network/wireless/firewall/dhcp 属危险操作，需用户确认。修改后如需生效请再调用 apply_config。",
		Parameters: json.RawMessage(`{
			"type":"object",
			"properties":{
				"ops":{
					"type":"array",
					"description":"操作列表，按顺序执行",
					"items":{
						"type":"object",
						"properties":{
							"op":{"type":"string","enum":["set","add_section","delete_option","delete_section"]},
							"config":{"type":"string","description":"配置名，如 firewall"},
							"section":{"type":"string","description":"段落名；add_section 新增后用返回的段落名继续 set"},
							"option":{"type":"string"},
							"type":{"type":"string","description":"add_section 时段落类型，如 rule"},
							"name":{"type":"string","description":"add_section 时可选的命名段落名"},
							"value":{"type":"string"}
						},
						"required":["op","config"],
						"additionalProperties":false
					}
				}
			},
			"required":["ops"],
			"additionalProperties":false
		}`),
		Level: LevelWrite,
		LevelFor: func(args json.RawMessage) Level {
			ops, err := parseOps(args)
			if err != nil {
				return LevelWrite
			}
			for _, op := range ops {
				if DangerousConfigs[op.Config] {
					return LevelDangerous
				}
			}
			return LevelWrite
		},
		Describe: func(args json.RawMessage) (string, error) {
			ops, err := parseOps(args)
			if err != nil {
				return "", err
			}
			return describeOps(ops), nil
		},
		Exec: func(ctx context.Context, args json.RawMessage) (string, error) {
			ops, err := parseOps(args)
			if err != nil {
				return "", err
			}
			return p.UCIApply(ctx, ops)
		},
	})

	add(&Tool{
		Name:        "service_control",
		Description: "控制 OpenWRT 服务：/etc/init.d/<service> start|stop|restart|enable|disable",
		Parameters: json.RawMessage(`{
			"type":"object",
			"properties":{
				"service":{"type":"string","description":"服务名，如 dnsmasq、uhttpd"},
				"action":{"type":"string","enum":["start","stop","restart","enable","disable"]}
			},
			"required":["service","action"],
			"additionalProperties":false
		}`),
		Level: LevelWrite,
		Describe: func(args json.RawMessage) (string, error) {
			var a struct {
				Service string `json:"service"`
				Action  string `json:"action"`
			}
			if err := json.Unmarshal(args, &a); err != nil {
				return "", err
			}
			return fmt.Sprintf("%s 服务 %s", actionCN(a.Action), a.Service), nil
		},
		Exec: func(ctx context.Context, args json.RawMessage) (string, error) {
			var a struct {
				Service string `json:"service"`
				Action  string `json:"action"`
			}
			if err := json.Unmarshal(args, &a); err != nil {
				return "", err
			}
			return p.ServiceControl(ctx, a.Service, a.Action)
		},
	})

	add(&Tool{
		Name:        "apply_config",
		Description: "重载配置使修改生效（set_config 只是写入，不生效）。network→network reload, firewall→fw4 reload, wireless→wifi reload, dhcp→dnsmasq restart。属危险操作（可能短暂断网/改防火墙），需用户确认。",
		Parameters: json.RawMessage(`{
			"type":"object",
			"properties":{
				"configs":{
					"type":"array",
					"items":{"type":"string","enum":["network","firewall","wireless","dhcp"]},
					"description":"要重载的配置名"
				}
			},
			"required":["configs"],
			"additionalProperties":false
		}`),
		Level: LevelDangerous,
		Describe: func(args json.RawMessage) (string, error) {
			var a struct {
				Configs []string `json:"configs"`
			}
			if err := json.Unmarshal(args, &a); err != nil {
				return "", err
			}
			return "重载配置使生效: " + strings.Join(a.Configs, ", "), nil
		},
		Exec: func(ctx context.Context, args json.RawMessage) (string, error) {
			var a struct {
				Configs []string `json:"configs"`
			}
			if err := json.Unmarshal(args, &a); err != nil {
				return "", err
			}
			return p.ApplyConfig(ctx, a.Configs)
		},
	})

	return r
}

// ---- helpers ----

func clampLines(n int) int {
	if n <= 0 {
		return 50
	}
	if n > 100 {
		return 100
	}
	return n
}

func clampCount(n int) int {
	if n <= 0 {
		return 4
	}
	if n > 10 {
		return 10
	}
	return n
}

func parseOps(args json.RawMessage) ([]Op, error) {
	var a struct {
		Ops []Op `json:"ops"`
	}
	if err := json.Unmarshal(args, &a); err != nil {
		return nil, err
	}
	if len(a.Ops) == 0 {
		return nil, fmt.Errorf("ops 不能为空")
	}
	for _, op := range a.Ops {
		switch op.Op {
		case "set":
			if op.Config == "" || op.Section == "" || op.Option == "" {
				return nil, fmt.Errorf("set 需要 config/section/option")
			}
		case "add_section":
			if op.Config == "" || op.Type == "" {
				return nil, fmt.Errorf("add_section 需要 config/type")
			}
		case "delete_option":
			if op.Config == "" || op.Section == "" || op.Option == "" {
				return nil, fmt.Errorf("delete_option 需要 config/section/option")
			}
		case "delete_section":
			if op.Config == "" || op.Section == "" {
				return nil, fmt.Errorf("delete_section 需要 config/section")
			}
		default:
			return nil, fmt.Errorf("未知 op: %s", op.Op)
		}
	}
	return a.Ops, nil
}

func describeOps(ops []Op) string {
	var parts []string
	for _, op := range ops {
		switch op.Op {
		case "set":
			parts = append(parts, fmt.Sprintf("修改 %s.%s.%s = %s", op.Config, op.Section, op.Option, op.Value))
		case "add_section":
			name := op.Name
			if name == "" {
				name = "匿名"
			}
			parts = append(parts, fmt.Sprintf("新增 %s 段落 %s(%s)", op.Config, name, op.Type))
		case "delete_option":
			parts = append(parts, fmt.Sprintf("删除 %s.%s.%s", op.Config, op.Section, op.Option))
		case "delete_section":
			parts = append(parts, fmt.Sprintf("删除 %s 段落 %s", op.Config, op.Section))
		}
	}
	return strings.Join(parts, "；")
}

func actionCN(a string) string {
	switch a {
	case "start":
		return "启动"
	case "stop":
		return "停止"
	case "restart":
		return "重启"
	case "enable":
		return "启用开机自启"
	case "disable":
		return "禁用开机自启"
	}
	return a
}
