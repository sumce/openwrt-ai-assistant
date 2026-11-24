package agent

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/sumce/openwrt-ai-assistant/internal/config"
	"github.com/sumce/openwrt-ai-assistant/internal/llm"
	"github.com/sumce/openwrt-ai-assistant/internal/runstate"
	"github.com/sumce/openwrt-ai-assistant/internal/safety"
	"github.com/sumce/openwrt-ai-assistant/internal/store"
	"github.com/sumce/openwrt-ai-assistant/internal/tools"
)

// fakeLLM is an OpenAI-compatible endpoint that scripts an agent
// conversation over SSE:
//
//	0 tool results → get_system_info
//	1 tool result  → set_config on firewall (dangerous → must park)
//	≥2             → final answer
func fakeLLM() *httptest.Server {
	mux := http.NewServeMux()
	mux.HandleFunc("/chat/completions", func(w http.ResponseWriter, r *http.Request) {
		var req llm.ChatRequest
		_ = json.NewDecoder(r.Body).Decode(&req)

		toolMsgs := 0
		for _, m := range req.Messages {
			if m.Role == "tool" {
				toolMsgs++
			}
		}

		w.Header().Set("Content-Type", "text/event-stream")
		f := w.(http.Flusher)

		write := func(format string, a ...any) {
			fmt.Fprintf(w, "data: "+format+"\n\n", a...)
			f.Flush()
		}

		switch {
		case toolMsgs == 0:
			write(`{"choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"id":"call_1","type":"function","function":{"name":"get_system_info","arguments":""}}]}}]}`)
			write(`{"choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"function":{"arguments":"{}"}}]}}]}`)
			write(`{"choices":[{"index":0,"delta":{},"finish_reason":"tool_calls"}]}`)
			write(`[DONE]`)
		case toolMsgs == 1:
			write(`{"choices":[{"index":0,"delta":{"content":"我准备修改防火墙 WAN 入站规则，这属于危险操作。"}}]}`)
			write(`{"choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"id":"call_2","type":"function","function":{"name":"set_config","arguments":""}}]}}]}`)
			write(`{"choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"function":{"arguments":"{\"ops\":[{\"op\":\"set\",\"config\":\"firewall\",\"section\":\"wan_zone\",\"option\":\"input\",\"value\":\"ACCEPT\"}]}"}}]}}]}`)
			write(`{"choices":[{"index":0,"delta":{},"finish_reason":"tool_calls"}]}`)
			write(`[DONE]`)
		default:
			write(`{"choices":[{"index":0,"delta":{"content":"操作"}}]}`)
			write(`{"choices":[{"index":0,"delta":{"content":"已完成。"}}]}`)
			write(`{"choices":[{"index":0,"delta":{},"finish_reason":"stop"}]}`)
			write(`[DONE]`)
		}
	})
	return httptest.NewServer(mux)
}

func newTestManager(t *testing.T) (*Manager, *store.Store, string) {
	t.Helper()
	dir := t.TempDir()

	ts := fakeLLM()
	t.Cleanup(ts.Close)

	settings := config.NewJSONStore(filepath.Join(dir, "settings.json"))
	st := config.Default()
	st.Providers = []config.Provider{{
		ID: "p1", Name: "Test", BaseURL: ts.URL, APIKey: "sk-test", Model: "test-model",
	}}
	st.DefaultProvider = "p1"
	if err := settings.Save(st); err != nil {
		t.Fatal(err)
	}

	platform := tools.NewMock()
	registry := tools.NewRegistry(platform)
	backups := safety.NewBackups(filepath.Join(dir, "backups"), platform)
	runStore, err := runstate.New(filepath.Join(dir, "runs"))
	if err != nil {
		t.Fatal(err)
	}
	convs, err := store.New(filepath.Join(dir, "conversations"))
	if err != nil {
		t.Fatal(err)
	}
	m := NewManager(settings, registry, backups, runStore, convs)
	return m, convs, filepath.Join(dir, "backups")
}

func waitForState(t *testing.T, m *Manager, run *Run, states ...string) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		s := run.Snapshot().State
		for _, want := range states {
			if s == want {
				return
			}
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("run did not reach %v, state=%s snapshot=%+v", states, run.Snapshot().State, run.Snapshot())
}

// TestApproveFlow covers: tool call → dangerous op parks in
// waiting_approval → approve → backup created → run completes.
func TestApproveFlow(t *testing.T) {
	m, convs, backupDir := newTestManager(t)

	run, err := m.Start("", "把防火墙 WAN 入站改成 ACCEPT")
	if err != nil {
		t.Fatal(err)
	}
	waitForState(t, m, run, StateWaitingApproval)

	snap := run.Snapshot()
	if snap.Pending == nil || snap.Pending.Tool != "set_config" {
		t.Fatalf("unexpected pending: %+v", snap.Pending)
	}
	if snap.Pending.Level != "dangerous" {
		t.Fatalf("set_config on firewall should be dangerous, got %s", snap.Pending.Level)
	}
	// backup must NOT exist before approval
	if snap.Pending.Configs == nil || len(snap.Pending.Configs) != 1 {
		t.Fatalf("expected backup config list, got %+v", snap.Pending.Configs)
	}

	if err := m.Approve(run.ID, true); err != nil {
		t.Fatal(err)
	}
	waitForState(t, m, run, StateDone)

	entries, err := convs.Entries(run.ConversationID)
	if err != nil {
		t.Fatal(err)
	}
	var sawApproval, sawTool, sawAssistant bool
	for _, e := range entries {
		switch e.Type {
		case "approval":
			if e.Approved != nil && *e.Approved {
				sawApproval = true
			}
		case "tool":
			if e.Tool == "set_config" && strings.Contains(e.Result, "已应用") {
				sawTool = true
			}
		case "assistant":
			if strings.Contains(e.Text, "已完成") {
				sawAssistant = true
			}
		}
	}
	if !sawApproval || !sawTool || !sawAssistant {
		t.Fatalf("transcript incomplete: approval=%v tool=%v assistant=%v", sawApproval, sawTool, sawAssistant)
	}

	// a backup snapshot must have been written before the mutation
	files, _ := os.ReadDir(backupDir)
	if len(files) == 0 {
		t.Fatal("expected a config backup file before dangerous mutation")
	}
}

// TestRejectFlow: rejection feeds back to the model without executing.
func TestRejectFlow(t *testing.T) {
	m, convs, backupDir := newTestManager(t)

	run, err := m.Start("", "把防火墙 WAN 入站改成 ACCEPT")
	if err != nil {
		t.Fatal(err)
	}
	waitForState(t, m, run, StateWaitingApproval)

	if err := m.Approve(run.ID, false); err != nil {
		t.Fatal(err)
	}
	waitForState(t, m, run, StateDone)

	entries, _ := convs.Entries(run.ConversationID)
	var rejected, executed bool
	for _, e := range entries {
		if e.Type == "approval" && e.Approved != nil && !*e.Approved {
			rejected = true
		}
		if e.Type == "tool" && e.Tool == "set_config" && strings.Contains(e.Result, "已应用") {
			executed = true
		}
	}
	if !rejected || executed {
		t.Fatalf("reject flow broken: rejected=%v executed=%v", rejected, executed)
	}
	files, _ := os.ReadDir(backupDir)
	if len(files) != 0 {
		t.Fatal("no backup should exist when the op was rejected")
	}
}

// TestRestartSurvival: a run parked in waiting_approval is restorable from
// disk and can be approved after "restart" (a fresh Manager).
func TestRestartSurvival(t *testing.T) {
	dir := t.TempDir()
	ts := fakeLLM()
	defer ts.Close()

	settings := config.NewJSONStore(filepath.Join(dir, "settings.json"))
	st := config.Default()
	st.Providers = []config.Provider{{ID: "p1", Name: "T", BaseURL: ts.URL, APIKey: "k", Model: "m"}}
	st.DefaultProvider = "p1"
	_ = settings.Save(st)

	platform := tools.NewMock()
	registry := tools.NewRegistry(platform)
	backups := safety.NewBackups(filepath.Join(dir, "backups"), platform)
	runStore, _ := runstate.New(filepath.Join(dir, "runs"))
	convs, _ := store.New(filepath.Join(dir, "conversations"))

	m1 := NewManager(settings, registry, backups, runStore, convs)
	run, err := m1.Start("", "把防火墙 WAN 入站改成 ACCEPT")
	if err != nil {
		t.Fatal(err)
	}
	waitForState(t, m1, run, StateWaitingApproval)

	// "restart": restore persisted runs into a fresh manager
	m2 := NewManager(settings, registry, backups, runStore, convs)
	m2.BootstrapRestore()
	restored := m2.Get(run.ID)
	if restored == nil {
		t.Fatal("waiting_approval run lost after restart")
	}
	if err := m2.Approve(run.ID, true); err != nil {
		t.Fatal(err)
	}
	waitForState(t, m2, restored, StateDone)
}
