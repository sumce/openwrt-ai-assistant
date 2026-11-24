// Package fakellm provides a scripted OpenAI-compatible endpoint mounted
// at /fake-llm in dev mode. It lets the whole product (chat streaming, tool
// calls, approval flow) be exercised end-to-end without any external API:
//
//	0 tool results → read-only tool (system info / diagnostics)
//	mutation intent ("改/设置/重启…") → a write or dangerous op → parks for approval
//	after approval   → final answer
package fakellm

import (
	"encoding/json"
	"fmt"
	"net/http"
	"regexp"
	"strings"

	"github.com/sumce/openwrt-ai-assistant/internal/llm"
)

// Handler returns the fake endpoint mounted under /fake-llm (so base_url is
// http://127.0.0.1:<port>/fake-llm/v1).
func Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/v1/models", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"data": []map[string]string{{"id": "wrtai-mock"}},
		})
	})
	mux.HandleFunc("/v1/chat/completions", func(w http.ResponseWriter, r *http.Request) {
		var req llm.ChatRequest
		_ = json.NewDecoder(r.Body).Decode(&req)
		respond(w, req)
	})
	return mux
}

var mutationRe = regexp.MustCompile(`(?i)(修改|改变|更改|设置|改成|加上|添加|新增|删除|关闭|打开|开启|重启|禁用|启用|set |add |delete |restart )`)

func respond(w http.ResponseWriter, req llm.ChatRequest) {
	toolMsgs := 0
	var lastToolContent string
	var lastUser string
	for _, m := range req.Messages {
		switch m.Role {
		case "tool":
			toolMsgs++
			lastToolContent = m.Content
		case "user":
			lastUser = m.Content
		}
	}

	w.Header().Set("Content-Type", "text/event-stream")
	f := w.(http.Flusher)
	write := func(format string, a ...any) {
		fmt.Fprintf(w, "data: "+format+"\n\n", a...)
		f.Flush()
	}
	text := func(s string, finish string) {
		write(`{"choices":[{"index":0,"delta":{"content":%s}}]}`, jsonStr(s))
		write(`{"choices":[{"index":0,"delta":{},"finish_reason":%s}]}`, jsonStr(finish))
		write(`[DONE]`)
	}
	toolCall := func(id, name, args string) {
		write(`{"choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"id":%s,"type":"function","function":{"name":%s,"arguments":""}}]}}]}`,
			jsonStr(id), jsonStr(name))
		write(`{"choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"function":{"arguments":%s}}]}}]}`,
			jsonStr(args))
		write(`{"choices":[{"index":0,"delta":{},"finish_reason":"tool_calls"}]}`)
		write(`[DONE]`)
	}

	switch {
	case toolMsgs == 0:
		if mutationRe.MatchString(lastUser) {
			// Keep the demo deterministic: firewall ops showcase the
			// dangerous level, service ops the write level.
			if strings.Contains(lastUser, "防火墙") || strings.Contains(lastUser, "firewall") {
				toolCall("call_1", "set_config",
					`{"ops":[{"op":"set","config":"firewall","section":"wan_zone","option":"input","value":"ACCEPT"}]}`)
				return
			}
			if strings.Contains(lastUser, "服务") {
				toolCall("call_1", "service_control", `{"service":"dnsmasq","action":"restart"}`)
				return
			}
			toolCall("call_1", "set_config",
				`{"ops":[{"op":"set","config":"system","section":"system","option":"hostname","value":"WRT-AI-Mock"}]}`)
			return
		}
		if strings.Contains(lastUser, "日志") {
			toolCall("call_1", "read_log", `{"source":"logread","lines":10}`)
			return
		}
		if strings.Contains(lastUser, "设备") || strings.Contains(lastUser, "在线") {
			toolCall("call_1", "list_devices", `{}`)
			return
		}
		toolCall("call_1", "get_system_info", `{}`)

	case toolMsgs == 1:
		// Tool result in hand: answer directly.
		if strings.Contains(lastToolContent, "hostname") {
			hostname := regexpField(lastToolContent, `"hostname"`)
			model := regexpField(lastToolContent, `"model"`)
			firmware := regexpField(lastToolContent, `"version"`)
			text(fmt.Sprintf("已读取路由器信息：\n\n| 项目 | 值 |\n|---|---|\n| 主机名 | %s |\n| 型号 | %s |\n| 固件 | %s |",
				hostname, model, firmware), "stop")
			return
		}
		if strings.Contains(lastToolContent, "已应用") || strings.Contains(lastToolContent, "完成") {
			text("配置修改已提交（uci commit）。注意：改动尚未生效，如需立即生效请告诉我执行 apply_config。", "stop")
			return
		}
		text("操作已执行完成。", "stop")

	default:
		text("好的，还有其他需要帮忙的吗？", "stop")
	}
}

func jsonStr(s string) string {
	b, _ := json.Marshal(s)
	return string(b)
}

func regexpField(text, key string) string {
	re := regexp.MustCompile(regexp.QuoteMeta(key) + `\s*:\s*"([^"]+)"`)
	m := re.FindStringSubmatch(text)
	if len(m) > 1 {
		return m[1]
	}
	return "(未知)"
}
