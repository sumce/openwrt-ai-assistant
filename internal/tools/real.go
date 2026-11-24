package tools

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"regexp"
	"sort"
	"strings"
	"time"
	"unicode"
)

// Real shells out to the actual router: uci, ubus, logread, iwinfo, ip...
type Real struct{}

func NewReal() *Real { return &Real{} }

// run executes a command (argv style, no shell) and returns combined output.
func (r *Real) run(ctx context.Context, timeout time.Duration, name string, args ...string) (string, error) {
	if timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, timeout)
		defer cancel()
	}
	cmd := exec.CommandContext(ctx, name, args...)
	var buf bytes.Buffer
	cmd.Stdout = &buf
	cmd.Stderr = &buf
	err := cmd.Run()
	out := strings.TrimSpace(buf.String())
	if err != nil {
		if ctx.Err() != nil {
			return out, fmt.Errorf("%s 超时或被取消: %w", name, ctx.Err())
		}
		return out, fmt.Errorf("%s: %w: %s", name, err, out)
	}
	return out, nil
}

func (r *Real) tryRun(ctx context.Context, timeout time.Duration, name string, args ...string) string {
	out, err := r.run(ctx, timeout, name, args...)
	if err != nil {
		return fmt.Sprintf("[! %s: %v]", name, err)
	}
	return out
}

func (r *Real) SystemInfo(ctx context.Context) (string, error) {
	var b strings.Builder
	b.WriteString("## board\n")
	b.WriteString(r.tryRun(ctx, 5*time.Second, "ubus", "call", "system", "board"))
	b.WriteString("\n\n## info\n")
	b.WriteString(r.tryRun(ctx, 5*time.Second, "ubus", "call", "system", "info"))
	b.WriteString("\n\n## disk\n")
	b.WriteString(r.tryRun(ctx, 5*time.Second, "df", "-h"))
	return truncate(b.String(), capGeneral), nil
}

func (r *Real) NetworkStatus(ctx context.Context) (string, error) {
	out, err := r.run(ctx, 5*time.Second, "ubus", "call", "network.interface", "dump")
	if err == nil {
		return truncate(out, capGeneral), nil
	}
	// Fallback for minimal systems without ubus network dump.
	var b strings.Builder
	b.WriteString("## addresses\n")
	b.WriteString(r.tryRun(ctx, 5*time.Second, "ip", "-4", "addr"))
	b.WriteString("\n\n## routes\n")
	b.WriteString(r.tryRun(ctx, 5*time.Second, "ip", "route"))
	b.WriteString("\n\n## dns\n")
	b.WriteString(r.tryRun(ctx, 5*time.Second, "cat", "/tmp/resolv.conf.d/resolv.conf"))
	return truncate(b.String(), capGeneral), nil
}

func (r *Real) WirelessStatus(ctx context.Context) (string, error) {
	var b strings.Builder
	b.WriteString("## wireless status\n")
	b.WriteString(r.tryRun(ctx, 5*time.Second, "ubus", "call", "network.wireless", "status"))
	b.WriteString("\n\n## iwinfo\n")
	b.WriteString(r.tryRun(ctx, 5*time.Second, "iwinfo"))
	return truncate(b.String(), capGeneral), nil
}

func (r *Real) Devices(ctx context.Context) (string, error) {
	var b strings.Builder
	b.WriteString("## DHCP leases\n")
	b.WriteString(r.tryRun(ctx, 5*time.Second, "cat", "/tmp/dhcp.leases"))
	b.WriteString("\n\n## ARP/neighbors\n")
	b.WriteString(r.tryRun(ctx, 5*time.Second, "ip", "neigh"))
	return truncate(b.String(), capGeneral), nil
}

func (r *Real) ReadLog(ctx context.Context, source string, lines int) (string, error) {
	var cmd string
	switch source {
	case "dmesg":
		cmd = "dmesg"
	default:
		cmd = "logread"
	}
	out, err := r.run(ctx, 10*time.Second, cmd)
	if err != nil && out == "" {
		return "", err
	}
	all := strings.Split(out, "\n")
	if len(all) > lines {
		all = all[len(all)-lines:]
	}
	return truncate(strings.Join(all, "\n"), capLogOrCfg), nil
}

var targetRe = regexp.MustCompile(`^[A-Za-z0-9.\-:_]+$`)

func (r *Real) Diagnose(ctx context.Context, command, target string, count int) (string, error) {
	if !targetRe.MatchString(target) {
		return "", fmt.Errorf("非法目标: %s", target)
	}
	timeout := 60 * time.Second
	var name string
	var args []string
	switch command {
	case "ping":
		name = "ping"
		args = []string{"-c", fmt.Sprint(count), "-W", "3", target}
		timeout = time.Duration(count)*4*time.Second + 10*time.Second
	case "traceroute":
		name = "traceroute"
		args = []string{"-n", "-m", "15", "-w", "2", target}
	case "nslookup":
		name = "nslookup"
		args = []string{target}
	default:
		return "", fmt.Errorf("未知诊断命令: %s", command)
	}
	out, err := r.run(ctx, timeout, name, args...)
	if err != nil && out == "" {
		return "", err
	}
	return truncate(out, capGeneral), nil
}

func (r *Real) UCIShow(ctx context.Context, path string) (string, error) {
	if path == "" {
		out, err := r.run(ctx, 5*time.Second, "uci", "show")
		if err != nil {
			return "", err
		}
		return truncate(out, capLogOrCfg), nil
	}
	if !validUCIName(strings.ReplaceAll(path, ".", "")) {
		return "", fmt.Errorf("非法配置路径: %s", path)
	}
	out, err := r.run(ctx, 5*time.Second, "uci", "show", path)
	if err != nil && out == "" {
		return "", err
	}
	return truncate(out, capLogOrCfg), nil
}

func (r *Real) UCIApply(ctx context.Context, ops []Op) (string, error) {
	touched := map[string]bool{}
	var added []string // section ids created, for the summary

	for i, op := range ops {
		if !validUCIName(op.Config) {
			return "", fmt.Errorf("非法配置名: %s", op.Config)
		}
		switch op.Op {
		case "set":
			if !validUCIName(op.Section) || !validUCIName(op.Option) {
				return "", fmt.Errorf("非法 section/option: %s.%s", op.Section, op.Option)
			}
			if _, err := r.run(ctx, 5*time.Second, "uci", "set",
				fmt.Sprintf("%s.%s.%s=%s", op.Config, op.Section, op.Option, op.Value)); err != nil {
				return "", fmt.Errorf("第 %d 步失败: %w", i+1, err)
			}
		case "add_section":
			if !validUCIName(op.Type) {
				return "", fmt.Errorf("非法类型: %s", op.Type)
			}
			out, err := r.run(ctx, 5*time.Second, "uci", "add", op.Config, op.Type)
			if err != nil {
				return "", fmt.Errorf("第 %d 步失败: %w", i+1, err)
			}
			secID := strings.TrimSpace(out)
			if op.Name != "" && validUCIName(op.Name) {
				if _, err := r.run(ctx, 5*time.Second, "uci", "rename",
					fmt.Sprintf("%s.%s=%s", op.Config, secID, op.Name)); err != nil {
					return "", fmt.Errorf("第 %d 步 rename 失败: %w", i+1, err)
				}
				secID = op.Name
			}
			// Rewrite later ops referencing this index to the real id.
			for j := i + 1; j < len(ops); j++ {
				if ops[j].Section == "" {
					ops[j].Section = secID
				}
			}
			// The op itself may carry a section placeholder; record it.
			added = append(added, secID)
			touched[op.Config] = true
			continue
		case "delete_option":
			if !validUCIName(op.Section) || !validUCIName(op.Option) {
				return "", fmt.Errorf("非法 section/option: %s.%s", op.Section, op.Option)
			}
			if _, err := r.run(ctx, 5*time.Second, "uci", "delete",
				fmt.Sprintf("%s.%s.%s", op.Config, op.Section, op.Option)); err != nil {
				return "", fmt.Errorf("第 %d 步失败: %w", i+1, err)
			}
		case "delete_section":
			if !validUCIName(op.Section) {
				return "", fmt.Errorf("非法 section: %s", op.Section)
			}
			if _, err := r.run(ctx, 5*time.Second, "uci", "delete",
				fmt.Sprintf("%s.%s", op.Config, op.Section)); err != nil {
				return "", fmt.Errorf("第 %d 步失败: %w", i+1, err)
			}
		}
		touched[op.Config] = true
	}

	var committed []string
	for cfg := range touched {
		if _, err := r.run(ctx, 5*time.Second, "uci", "commit", cfg); err != nil {
			return "", fmt.Errorf("commit %s 失败: %w", cfg, err)
		}
		committed = append(committed, cfg)
	}
	sort.Strings(committed)

	summary := fmt.Sprintf("已应用 %d 项修改并提交配置: %s", len(ops), strings.Join(committed, ", "))
	if len(added) > 0 {
		summary += fmt.Sprintf("（新增段落: %s）", strings.Join(added, ", "))
	}
	return summary, nil
}

func (r *Real) ExportConfig(ctx context.Context, config string) (string, error) {
	if !validUCIName(config) {
		return "", fmt.Errorf("非法配置名: %s", config)
	}
	return r.run(ctx, 5*time.Second, "uci", "export", config)
}

func (r *Real) ImportConfig(ctx context.Context, config, text string) (string, error) {
	if !validUCIName(config) {
		return "", fmt.Errorf("非法配置名: %s", config)
	}
	cmd := exec.CommandContext(ctx, "uci", "import", config)
	cmd.Stdin = strings.NewReader(text)
	var buf bytes.Buffer
	cmd.Stderr = &buf
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("uci import %s: %w: %s", config, err, buf.String())
	}
	if _, err := r.run(ctx, 5*time.Second, "uci", "commit", config); err != nil {
		return "", err
	}
	return "已恢复配置 " + config, nil
}

func (r *Real) ServiceControl(ctx context.Context, service, action string) (string, error) {
	if !validUCIName(service) {
		return "", fmt.Errorf("非法服务名: %s", service)
	}
	if service == "wrtai" {
		return "", fmt.Errorf("拒绝操作 WRT AI 自身服务")
	}
	script := "/etc/init.d/" + service
	if _, err := r.run(ctx, 2*time.Second, "test", "-x", script); err != nil {
		return "", fmt.Errorf("服务不存在: %s", script)
	}
	out, err := r.run(ctx, 30*time.Second, script, action)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%s %s 完成 %s", script, action, out), nil
}

func (r *Real) ApplyConfig(ctx context.Context, configs []string) (string, error) {
	var done []string
	for _, cfg := range configs {
		var out string
		var err error
		switch cfg {
		case "network":
			out, err = r.run(ctx, 60*time.Second, "/etc/init.d/network", "reload")
		case "firewall":
			out, err = r.run(ctx, 60*time.Second, "fw4", "reload")
			if err != nil {
				out, err = r.run(ctx, 60*time.Second, "/etc/init.d/firewall", "reload")
			}
		case "wireless":
			out, err = r.run(ctx, 60*time.Second, "wifi", "reload")
			if err != nil {
				out, err = r.run(ctx, 60*time.Second, "/etc/init.d/network", "reload")
			}
		case "dhcp":
			out, err = r.run(ctx, 60*time.Second, "/etc/init.d/dnsmasq", "restart")
		default:
			return "", fmt.Errorf("不支持重载的配置: %s", cfg)
		}
		if err != nil {
			return "", fmt.Errorf("重载 %s 失败: %w: %s", cfg, err, out)
		}
		done = append(done, cfg)
	}
	return "已重载配置: " + strings.Join(done, ", "), nil
}

func validUCIName(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_' || r == '-' {
			continue
		}
		return false
	}
	return true
}
