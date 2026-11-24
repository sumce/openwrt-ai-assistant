package tools

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"
)

// Mock is a fake router with an in-memory UCI state. It lets the whole
// product (chat, tools, approval flow, backups) be exercised on a PC —
// mutations really land in the fake state, so get_config reflects them.
type Mock struct {
	mu      sync.Mutex
	started time.Time
	configs map[string]map[string]*MockSection
	order   map[string][]string
}

type MockSection struct {
	Type    string
	Options map[string]string
}

func NewMock() *Mock {
	m := &Mock{
		started: time.Now().Add(-3 * 26 * time.Hour), // ~3 days uptime
		configs: map[string]map[string]*MockSection{},
		order:   map[string][]string{},
	}
	m.seed()
	return m
}

func (m *Mock) seed() {
	add := func(cfg, name, typ string, opts map[string]string) {
		if m.configs[cfg] == nil {
			m.configs[cfg] = map[string]*MockSection{}
			m.order[cfg] = nil
		}
		m.configs[cfg][name] = &MockSection{Type: typ, Options: opts}
		m.order[cfg] = append(m.order[cfg], name)
	}

	add("system", "system", "system", map[string]string{
		"hostname": "WRT-AI-Mock", "timezone": "CST-8", "zonename": "Asia/Shanghai",
	})
	add("system", "@system[0]", "ntp", nil)

	add("network", "lan", "interface", map[string]string{
		"device": "br-lan", "proto": "static", "ipaddr": "192.168.1.1", "netmask": "255.255.255.0",
	})
	add("network", "wan", "interface", map[string]string{
		"device": "eth1", "proto": "dhcp",
	})
	add("network", "guest", "interface", map[string]string{
		"device": "br-guest", "proto": "static", "ipaddr": "192.168.9.1", "netmask": "255.255.255.0",
	})

	add("wireless", "radio0", "wifi-device", map[string]string{
		"type": "mt7621", "path": "mock/pci0000/0100", "band": "5g", "channel": "149",
		"htmode": "HE80", "disabled": "0",
	})
	add("wireless", "radio1", "wifi-device", map[string]string{
		"type": "mt7621", "path": "mock/pci0000/0101", "band": "2g", "channel": "6",
		"htmode": "HE20", "disabled": "0",
	})
	add("wireless", "default_radio0", "wifi-iface", map[string]string{
		"device": "radio0", "network": "lan", "mode": "ap", "ssid": "MockHome-5G",
		"encryption": "sae-mixed", "key": "mockpassword123",
	})
	add("wireless", "default_radio1", "wifi-iface", map[string]string{
		"device": "radio1", "network": "lan", "mode": "ap", "ssid": "MockHome-2.4G",
		"encryption": "psk2", "key": "mockpassword123",
	})

	add("firewall", "wan_zone", "zone", map[string]string{
		"name": "wan", "input": "REJECT", "output": "ACCEPT", "forward": "REJECT", "masq": "1", "mtu_fix": "1",
	})
	add("firewall", "lan_zone", "zone", map[string]string{
		"name": "lan", "input": "ACCEPT", "output": "ACCEPT", "forward": "ACCEPT",
	})
	add("firewall", "wan_fwd", "forwarding", map[string]string{"src": "lan", "dest": "wan"})

	add("dhcp", "@dnsmasq[0]", "dnsmasq", map[string]string{
		"domainneeded": "1", "local_service": "1", "leasefile": "/tmp/dhcp.leases",
	})
	add("dhcp", "lan", "dhcp", map[string]string{
		"interface": "lan", "start": "100", "limit": "150", "leasetime": "12h", "dhcpv4": "server",
	})

	add("wrtai", "main", "wrtai", map[string]string{
		"enabled": "1", "port": "8765",
	})
}

// ---- renders ----

func (m *Mock) renderShow(path string) string {
	m.mu.Lock()
	defer m.mu.Unlock()
	var b strings.Builder
	cfgFilter, secFilter := "", ""
	if path != "" {
		parts := strings.SplitN(path, ".", 2)
		cfgFilter = parts[0]
		if len(parts) > 1 {
			secFilter = parts[1]
		}
	}
	cfgNames := sortedKeys(m.configs)
	for _, cfg := range cfgNames {
		if cfgFilter != "" && cfg != cfgFilter {
			continue
		}
		for _, sec := range m.order[cfg] {
			if secFilter != "" && sec != secFilter {
				continue
			}
			s := m.configs[cfg][sec]
			for _, k := range sortedOptionKeys(s.Options) {
				fmt.Fprintf(&b, "%s.%s.%s='%s'\n", cfg, sec, k, s.Options[k])
			}
		}
	}
	if b.Len() == 0 {
		return "uci: 未找到配置条目"
	}
	return b.String()
}

func (m *Mock) renderExport(cfg string) string {
	m.mu.Lock()
	defer m.mu.Unlock()
	var b strings.Builder
	for _, sec := range m.order[cfg] {
		s := m.configs[cfg][sec]
		fmt.Fprintf(&b, "config %s '%s'\n", s.Type, sec)
		for _, k := range sortedOptionKeys(s.Options) {
			fmt.Fprintf(&b, "\toption %s '%s'\n", k, s.Options[k])
		}
		b.WriteString("\n")
	}
	return b.String()
}

// ---- Platform ----

func (m *Mock) SystemInfo(ctx context.Context) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	uptime := int(time.Since(m.started).Seconds())
	return fmt.Sprintf(`{
  "hostname": "WRT-AI-Mock",
  "model": "MockRouter MT7981A (mock)",
  "board_name": "mock,mt7981a",
  "release": {"distribution": "OpenWrt", "version": "24.10.2", "revision": "r28899-c073da8d97", "target": "mediatek/filogic", "description": "OpenWrt 24.10.2 (mock)"},
  "uptime": %d,
  "loadavg": [0.42, 0.35, 0.30],
  "memory": {"total": 262144, "free": 131072, "used": 98304, "buffered": 16384, "cached": 32768},
  "disk": {"/overlay": "35.2M used of 112.0M (31%%)"}
}`, uptime), nil
}

func (m *Mock) NetworkStatus(ctx context.Context) (string, error) {
	return `{
  "interface": [
    {"interface": "lan", "up": true, "proto": "static",
     "address": "192.168.1.1", "netmask": "255.255.255.0",
     "device": "br-lan"},
    {"interface": "wan", "up": true, "proto": "dhcp",
     "address": "192.168.50.23", "netmask": "255.255.255.0", "gateway": "192.168.50.1",
     "dns": ["192.168.50.1"], "uptime": 268542, "device": "eth1"},
    {"interface": "guest", "up": true, "proto": "static",
     "address": "192.168.9.1", "netmask": "255.255.255.0", "device": "br-guest"}
  ],
  "route": "default via 192.168.50.1 dev eth1 (mock)"
}`, nil
}

func (m *Mock) WirelessStatus(ctx context.Context) (string, error) {
	return `{
  "radio0": {"up": true, "pending": false, "band": "5G", "channel": 149, "country": "CN", "interfaces": [
      {"section": "default_radio0", "ifname": "phy0-ap0", "config": {"mode": "ap", "ssid": "MockHome-5G", "encryption": "sae-mixed"}}]},
  "radio1": {"up": true, "pending": false, "band": "2.4G", "channel": 6, "country": "CN", "interfaces": [
      {"section": "default_radio1", "ifname": "phy1-ap0", "config": {"mode": "ap", "ssid": "MockHome-2.4G", "encryption": "psk2"}}]}
}
## iwinfo
phy0-ap0  ESSID: "MockHome-5G"    Channel: 149 (5745.0 MHz)  Signal: -32 dBm  Tx Power: 20 dBm
phy1-ap0  ESSID: "MockHome-2.4G"  Channel: 6 (2437.0 MHz)    Signal: -41 dBm  Tx Power: 20 dBm`, nil
}

func (m *Mock) Devices(ctx context.Context) (string, error) {
	return `## DHCP leases
1728345600 a4:83:e7:11:22:33 192.168.1.101 iphone-of-user 01:a4:83:e7:11:22:33
1728345512 8c:85:90:aa:bb:cc 192.168.1.102 macbook-pro 01:8c:85:90:aa:bb:cc
1728345000 00:11:32:dd:ee:ff 192.168.1.103 nas 01:00:11:32:dd:ee:ff
## ARP/neighbors
192.168.1.101 dev br-lan lladdr a4:83:e7:11:22:33 REACHABLE
192.168.1.102 dev br-lan lladdr 8c:85:90:aa:bb:cc REACHABLE
192.168.1.103 dev br-lan lladdr 00:11:32:dd:ee:ff STALE
fe80::1 dev eth1 lladdr 192.168.50.1-router REACHABLE`, nil
}

func (m *Mock) ReadLog(ctx context.Context, source string, lines int) (string, error) {
	sample := []string{
		"Mon Nov 24 08:01:23 2026 daemon.info hostapd: phy0-ap0: STA a4:83:e7:11:22:33 WPA group key handshake completed",
		"Mon Nov 24 08:15:02 2026 kern.warn kernel: [31842.115] mt76xx 14c0,0000: IC: MT7981A, Ext PA linearity",
		"Mon Nov 24 09:20:41 2026 daemon.notice netifd: wan (1213): udhcpc: lease of 192.168.50.23 obtained, lease time 7200",
		"Mon Nov 24 09:20:42 2026 daemon.notice netifd: Interface 'wan' is now up",
		"Mon Nov 24 10:02:11 2026 authpriv.info dropbear[5121]: Password auth succeeded for 'root' from 192.168.1.101:54321",
		"Mon Nov 24 11:44:05 2026 kern.info kernel: [51230.441] br-guest: port 2(phy1-ap1) entered forwarding state",
		"Mon Nov 24 12:30:59 2026 daemon.err uhttpd[1130]: luci: failed login for root from 192.168.1.55",
		"Mon Nov 24 13:11:34 2026 daemon.info dnsmasq-dhcp[1042]: DHCPREQUEST(br-lan) 192.168.1.102 8c:85:90:aa:bb:cc",
	}
	if source == "dmesg" {
		sample = []string{
			"[    0.000000] Booting Linux on physical CPU 0x0",
			"[    0.412000] mt7981-pinctrl 11f00000.pinctrl: invalid group",
			"[    1.234000] mtk_soc_eth 15100000.ethernet eth0: mediatek frame engine",
			"[    2.891000] mt76xx 14c0,0000.wmac: HW/SW Version: 0x8a1084a1",
			"[   12.554000] br-lan: port 1(eth0) entered forwarding state",
		}
	}
	if len(sample) > lines {
		sample = sample[len(sample)-lines:]
	}
	return strings.Join(sample, "\n"), nil
}

func (m *Mock) Diagnose(ctx context.Context, command, target string, count int) (string, error) {
	switch command {
	case "ping":
		var b strings.Builder
		fmt.Fprintf(&b, "PING %s (%s): 56 data bytes\n", target, target)
		for i := 0; i < count; i++ {
			fmt.Fprintf(&b, "64 bytes from %s: seq=%d ttl=57 time=%.2f ms\n", target, i, 9.0+float64(i)*0.3)
		}
		fmt.Fprintf(&b, "\n--- %s ping statistics ---\n%d packets transmitted, %d received, 0%% packet loss\n",
			target, count, count)
		return b.String(), nil
	case "traceroute":
		return fmt.Sprintf("traceroute to %s, 15 hops max\n 1  192.168.50.1  0.5 ms\n 2  100.64.0.1  3.2 ms\n 3  %s  8.9 ms", target, target), nil
	case "nslookup":
		return fmt.Sprintf("Server:    192.168.50.1\nAddress:   192.168.50.1\n\nName:      %s\nAddress 1: 93.184.216.34", target), nil
	}
	return "", fmt.Errorf("未知诊断命令: %s", command)
}

func (m *Mock) UCIShow(ctx context.Context, path string) (string, error) {
	return truncate(m.renderShow(path), capLogOrCfg), nil
}

func (m *Mock) UCIApply(ctx context.Context, ops []Op) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	for i, op := range ops {
		if m.configs[op.Config] == nil {
			m.configs[op.Config] = map[string]*MockSection{}
			m.order[op.Config] = nil
		}
		switch op.Op {
		case "set":
			sec, ok := m.configs[op.Config][op.Section]
			if !ok {
				return "", fmt.Errorf("第 %d 步失败: 段落 %s.%s 不存在", i+1, op.Config, op.Section)
			}
			sec.Options[op.Option] = op.Value
		case "add_section":
			name := op.Name
			if name == "" {
				name = fmt.Sprintf("cfg%04x", time.Now().UnixNano()&0xffff)
			}
			m.configs[op.Config][name] = &MockSection{Type: op.Type, Options: map[string]string{}}
			m.order[op.Config] = append(m.order[op.Config], name)
			for j := i + 1; j < len(ops); j++ {
				if ops[j].Section == "" {
					ops[j].Section = name
				}
			}
		case "delete_option":
			sec, ok := m.configs[op.Config][op.Section]
			if ok {
				delete(sec.Options, op.Option)
			}
		case "delete_section":
			delete(m.configs[op.Config], op.Section)
			for idx, s := range m.order[op.Config] {
				if s == op.Section {
					m.order[op.Config] = append(m.order[op.Config][:idx], m.order[op.Config][idx+1:]...)
					break
				}
			}
		}
	}
	return fmt.Sprintf("已应用 %d 项修改并提交配置 (mock)", len(ops)), nil
}

func (m *Mock) ExportConfig(ctx context.Context, config string) (string, error) {
	if m.configs[config] == nil {
		return "", fmt.Errorf("配置 %s 不存在", config)
	}
	return m.renderExport(config), nil
}

func (m *Mock) ImportConfig(ctx context.Context, config, text string) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	newCfg := map[string]*MockSection{}
	var newOrder []string
	var cur string
	for _, raw := range strings.Split(text, "\n") {
		line := strings.TrimSpace(raw)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if strings.HasPrefix(line, "config ") {
			fields := strings.Fields(strings.TrimPrefix(line, "config "))
			typ, name := "anonymous", fmt.Sprintf("cfg%04x", len(newOrder))
			if len(fields) >= 1 {
				typ = fields[0]
			}
			if len(fields) >= 2 {
				name = strings.Trim(fields[1], `'"`)
			}
			newCfg[name] = &MockSection{Type: typ, Options: map[string]string{}}
			newOrder = append(newOrder, name)
			cur = name
			continue
		}
		if strings.HasPrefix(line, "option ") && cur != "" {
			rest := strings.TrimSpace(strings.TrimPrefix(line, "option "))
			k, v, ok := splitUCIOption(rest)
			if ok {
				newCfg[cur].Options[k] = v
			}
		}
	}
	m.configs[config] = newCfg
	m.order[config] = newOrder
	return "已恢复配置 " + config + " (mock)", nil
}

func (m *Mock) ServiceControl(ctx context.Context, service, action string) (string, error) {
	if service == "wrtai" {
		return "", fmt.Errorf("拒绝操作 WRT AI 自身服务")
	}
	return fmt.Sprintf("/etc/init.d/%s %s 完成 (mock)", service, action), nil
}

func (m *Mock) ApplyConfig(ctx context.Context, configs []string) (string, error) {
	return "已重载配置: " + strings.Join(configs, ", ") + " (mock)", nil
}

// ---- small helpers ----

func sortedKeys(m map[string]map[string]*MockSection) []string {
	var keys []string
	for k := range m {
		keys = append(keys, k)
	}
	for i := 0; i < len(keys); i++ {
		for j := i + 1; j < len(keys); j++ {
			if keys[j] < keys[i] {
				keys[i], keys[j] = keys[j], keys[i]
			}
		}
	}
	return keys
}

func sortedOptionKeys(m map[string]string) []string {
	var keys []string
	for k := range m {
		keys = append(keys, k)
	}
	for i := 0; i < len(keys); i++ {
		for j := i + 1; j < len(keys); j++ {
			if keys[j] < keys[i] {
				keys[i], keys[j] = keys[j], keys[i]
			}
		}
	}
	return keys
}

func splitUCIOption(rest string) (string, string, bool) {
	idx := strings.IndexByte(rest, ' ')
	if idx < 0 {
		return "", "", false
	}
	key := rest[:idx]
	val := strings.TrimSpace(rest[idx+1:])
	val = strings.Trim(val, `'"`)
	return key, val, true
}
