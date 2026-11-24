package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/sumce/openwrt-ai-assistant/internal/config"
	"github.com/sumce/openwrt-ai-assistant/internal/llm"
	"github.com/sumce/openwrt-ai-assistant/internal/runstate"
	"github.com/sumce/openwrt-ai-assistant/internal/safety"
	"github.com/sumce/openwrt-ai-assistant/internal/store"
	"github.com/sumce/openwrt-ai-assistant/internal/tools"
)

const (
	maxSteps        = 15                 // tool-calling rounds per run
	runTimeout      = 15 * time.Minute   // hard cap on one run
	maxHistoryMsgs  = 60                 // sliding window over stored history
	toolResultStore = 1500               // chars of tool result kept in transcripts
)

const systemPrompt = `你是 WRT AI，一个运行在 OpenWRT 路由器上的 AI 助手，可以通过工具直接操作这台路由器。
规则：
1. 涉及路由器状态、配置、日志、网络的问题，必须先用工具获取真实数据，绝不凭空编造。
2. 修改配置前，先用一两句话向用户说明你准备做什么、有什么影响。
3. 修改 network/wireless/firewall/dhcp 属于危险操作（可能断网甚至把管理员锁在门外），系统会要求用户在界面上确认；确认被拒绝时不要反复重试，改为解释和询问。
4. set_config 只写入配置，不会立即生效；需要生效时再调用 apply_config。
5. 用用户的语言回答（默认中文），简洁、专业、可操作。`

// Manager owns all runs.
type Manager struct {
	mu        sync.Mutex
	runs      map[string]*Run
	sem       chan struct{} // single-run concurrency for router CPUs
	settings  config.Store
	registry  *tools.Registry
	backups   *safety.Backups
	runStore  *runstate.Store
	convs     *store.Store
}

func NewManager(settings config.Store, registry *tools.Registry, backups *safety.Backups,
	runStore *runstate.Store, convs *store.Store) *Manager {
	return &Manager{
		runs:     map[string]*Run{},
		sem:      make(chan struct{}, 1),
		settings: settings,
		registry: registry,
		backups:  backups,
		runStore: runStore,
		convs:    convs,
	}
}

// Get returns a live run.
func (m *Manager) Get(id string) *Run {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.runs[id]
}

// ListActive returns snapshots of runs that are not terminal yet — used by
// the frontend to reattach after a page refresh.
func (m *Manager) ListActive() []Snapshot {
	m.mu.Lock()
	defer m.mu.Unlock()
	var out []Snapshot
	for _, r := range m.runs {
		s := r.Snapshot()
		if !isTerminal(s.State) {
			out = append(out, s)
		}
	}
	return out
}

// BootstrapRestore reloads persisted runs at startup: waiting_approval runs
// stay approvable, everything else is dropped (its transcript already
// carries the interruption note).
func (m *Manager) BootstrapRestore() {
	list, err := m.runStore.LoadAll()
	if err != nil {
		return
	}
	for _, p := range list {
		if p.State == StateWaitingApproval && p.Pending != nil {
			r := newRun(p.ConversationID)
			r.mu.Lock()
			r.ID = p.ID
			r.State = p.State
			r.Messages = p.Messages
			r.Pending = p.Pending
			r.mu.Unlock()
			m.mu.Lock()
			m.runs[r.ID] = r
			m.mu.Unlock()
			continue
		}
		if p.State == StateRunning || p.State == StateQueued {
			_ = m.convs.Append(p.ConversationID, store.Entry{
				Type: "error",
				Text: "服务重启，本次任务已中断。",
			})
		}
		_ = m.runStore.Delete(p.ID)
	}
}

// Start creates (or reuses) a conversation, appends the user message and
// launches a run.
func (m *Manager) Start(convID, userText string) (*Run, error) {
	if strings.TrimSpace(userText) == "" {
		return nil, fmt.Errorf("消息不能为空")
	}
	var conv *store.Conversation
	if convID == "" {
		c, err := m.convs.Create(store.TruncateTitle(userText))
		if err != nil {
			return nil, err
		}
		conv = c
	} else {
		c, err := m.convs.Get(convID)
		if err != nil {
			return nil, err
		}
		conv = c
	}

	if err := m.convs.Append(conv.ID, store.Entry{Type: "user", Text: userText}); err != nil {
		return nil, err
	}

	entries, err := m.convs.Entries(conv.ID)
	if err != nil {
		return nil, err
	}
	msgs := buildMessages(entries, userText)

	run := newRun(conv.ID)
	run.Messages = msgs
	m.mu.Lock()
	m.runs[run.ID] = run
	m.mu.Unlock()
	go m.execute(run)
	return run, nil
}

// buildMessages: system prompt + bounded history + new user message.
func buildMessages(entries []store.Entry, newUser string) []llm.Message {
	msgs := []llm.Message{{Role: "system", Content: systemPrompt}}
	var hist []llm.Message
	for _, e := range entries {
		switch e.Type {
		case "user":
			hist = append(hist, llm.Message{Role: "user", Content: e.Text})
		case "assistant":
			if e.Text != "" {
				hist = append(hist, llm.Message{Role: "assistant", Content: e.Text})
			}
		}
	}
	if len(hist) > maxHistoryMsgs {
		hist = hist[len(hist)-maxHistoryMsgs:]
	}
	return append(msgs, hist...)
}

// execute drives the agent loop. It also handles the resume path (run has a
// pending action that was approved).
func (m *Manager) execute(run *Run) {
	m.sem <- struct{}{}
	defer func() { <-m.sem }()

	ctx, cancel := context.WithTimeout(context.Background(), runTimeout)
	defer cancel()
	run.mu.Lock()
	run.cancel = cancel
	run.mu.Unlock()
	run.setState(StateRunning)

	// Resume: run an approved pending action first.
	if run.Pending != nil {
		m.execPending(run, ctx)
		if run.isTerminal() {
			return
		}
	}

	for step := 1; ; step++ {
		if run.isTerminal() {
			return
		}

		settings, err := m.settings.Load()
		if err != nil {
			run.fail("读取配置失败: " + err.Error())
			return
		}
		provider := settings.DefaultProviderCfg()
		if provider == nil || provider.BaseURL == "" || provider.Model == "" {
			run.fail("尚未配置上游模型，请到设置页添加 API 提供方")
			return
		}

		forced := step > maxSteps
		if forced {
			run.Messages = append(run.Messages, llm.Message{
				Role:    "user",
				Content: "工具调用已达上限，请不要再调用工具，直接根据已有信息回答用户。",
			})
		}

		var toolDefs []llm.ToolDef
		if !forced {
			toolDefs = m.registry.LLMDefs()
		}
		client := llm.NewClient(provider.BaseURL, provider.APIKey)
		res, err := client.StreamChat(ctx, provider.Model, run.Messages, toolDefs, llm.StreamCallbacks{
			OnText: func(delta string) {
				run.emit(Event{Type: "text_delta", Text: delta})
			},
		})
		if err != nil {
			if ctx.Err() != nil {
				m.completeCancelled(run)
			} else {
				run.fail("调用上游模型失败: " + err.Error())
			}
			return
		}

		run.Messages = append(run.Messages, llm.Message{
			Role:      "assistant",
			Content:   res.Content,
			ToolCalls: res.ToolCalls,
		})

		if len(res.ToolCalls) == 0 {
			m.completeDone(run, res.Content)
			return
		}

		for _, tc := range res.ToolCalls {
			if run.isTerminal() {
				return
			}
			if !m.handleToolCall(run, ctx, tc, settings) {
				return // parked for approval
			}
		}
	}
}

// handleToolCall executes one tool call; returns false when the run parked
// in waiting_approval.
func (m *Manager) handleToolCall(run *Run, ctx context.Context, tc llm.ToolCall, settings *config.Settings) bool {
	tool := m.registry.Get(tc.Function.Name)
	if tool == nil {
		m.feedToolResult(run, tc.ID, tc.Function.Name,
			fmt.Sprintf("未知工具: %s", tc.Function.Name), "", false, false)
		return true
	}

	args := json.RawMessage(strings.TrimSpace(tc.Function.Arguments))
	if len(args) == 0 {
		args = json.RawMessage("{}")
	}
	desc, err := tool.Describe(args)
	if err != nil {
		m.feedToolResult(run, tc.ID, tc.Function.Name,
			"参数解析失败: "+err.Error(), "", false, false)
		return true
	}
	level := tools.ResolveLevel(tool, args)
	run.emit(Event{Type: "tool_start", Tool: tool.Name, Desc: desc, Level: level.String()})

	if safety.Decide(level, settings.ConfirmWrite) == safety.NeedApproval {
		var configs []string
		if tool.Name == "set_config" {
			if ops, err := parseSetOps(args); err == nil {
				configs = safety.ConfigsToBackup(ops)
			}
		}
		run.mu.Lock()
		run.Pending = &runstate.PendingAction{
			ToolCallID:  tc.ID,
			Tool:        tc.Function.Name,
			Args:        args,
			Level:       level.String(),
			Description: desc,
			Configs:     configs,
		}
		run.mu.Unlock()
		pending := run.Pending
		run.setState(StateWaitingApproval)
		run.emit(Event{Type: "approval_request", Pending: pending})
		m.persistRun(run)
		return false
	}

	m.execTool(run, ctx, tc.ID, tc.Function.Name, args, desc)
	return true
}

// execPending runs an approved pending action (with backup) and feeds the
// result back into the loop.
func (m *Manager) execPending(run *Run, ctx context.Context) {
	p := run.Pending
	tool := m.registry.Get(p.Tool)

	var backupNote string
	for _, cfg := range p.Configs {
		name, err := m.backups.Create(ctx, cfg)
		if err != nil {
			m.feedToolResult(run, p.ToolCallID, p.Tool,
				"备份失败，为安全起见已中止执行: "+err.Error(), p.Description, false, true)
			run.mu.Lock()
			run.Pending = nil
			run.mu.Unlock()
			return
		}
		backupNote = fmt.Sprintf("（已备份 %s → %s）", cfg, name)
	}

	result, execErr := "", error(nil)
	if tool == nil {
		execErr = fmt.Errorf("工具不存在: %s", p.Tool)
	} else {
		result, execErr = tool.Exec(ctx, p.Args)
	}
	resText := result
	ok := execErr == nil
	if execErr != nil {
		resText = "执行失败: " + execErr.Error()
	}

	// Transcript: the approval record was already appended by Approve();
	// feedToolResult appends the tool record. Only the backup note is
	// unique to this path.
	if backupNote != "" {
		_ = m.convs.Append(run.ConversationID, store.Entry{Type: "approval", Desc: backupNote, Approved: boolPtr(true)})
	}
	m.feedToolResult(run, p.ToolCallID, p.Tool, resText, p.Description, ok, true)

	run.mu.Lock()
	run.Pending = nil
	run.mu.Unlock()
	m.runStore.Delete(run.ID)
}

// execTool runs an already-allowed tool call (with backup when mutating).
func (m *Manager) execTool(run *Run, ctx context.Context, callID, toolName string, args json.RawMessage, desc string) {
	tool := m.registry.Get(toolName)

	// Backup before direct (user-disabled-confirmation) mutations.
	if toolName == "set_config" {
		if ops, err := parseSetOps(args); err == nil {
			for _, cfg := range safety.ConfigsToBackup(ops) {
				if _, err := m.backups.Create(ctx, cfg); err != nil {
					m.feedToolResult(run, callID, toolName,
						"备份失败，为安全起见已中止执行: "+err.Error(), desc, false, false)
					return
				}
			}
		}
	}

	var resText string
	var ok bool
	if tool == nil {
		resText, ok = "工具不存在: "+toolName, false
	} else {
		result, err := tool.Exec(ctx, args)
		ok = err == nil
		resText = result
		if err != nil {
			resText = "执行失败: " + err.Error()
		}
	}
	m.feedToolResult(run, callID, toolName, resText, desc, ok, false)
}

// feedToolResult emits the event, appends the tool message for the LLM and
// records the transcript entry. keepPending removes a parked pending action
// (used by execPending).
func (m *Manager) feedToolResult(run *Run, callID, toolName, result, desc string, ok, approved bool) {
	run.emit(Event{Type: "tool_result", Tool: toolName, Desc: desc, Result: result, OK: ok})
	run.Messages = append(run.Messages, llm.Message{
		Role:       "tool",
		ToolCallID: callID,
		Content:    result,
	})
	_ = m.convs.Append(run.ConversationID, store.Entry{
		Type: "tool", Tool: toolName, Desc: desc,
		Result: truncateStr(result, toolResultStore),
	})
}

func (m *Manager) completeDone(run *Run, text string) {
	_ = m.convs.Append(run.ConversationID, store.Entry{Type: "assistant", Text: text})
	m.runStore.Delete(run.ID)
	run.finishDone()
}

func (m *Manager) completeCancelled(run *Run) {
	_ = m.convs.Append(run.ConversationID, store.Entry{Type: "error", Text: "任务已取消。"})
	m.runStore.Delete(run.ID)
	run.finishCancelled()
}

// persistRun writes the full run state for restart survival.
func (m *Manager) persistRun(run *Run) {
	run.mu.Lock()
	p := &runstate.Persisted{
		ID:             run.ID,
		ConversationID: run.ConversationID,
		State:          run.State,
		Messages:       run.Messages,
		Pending:        run.Pending,
	}
	run.mu.Unlock()
	_ = m.runStore.Save(p)
}

// Approve resumes (or aborts) a run parked in waiting_approval.
func (m *Manager) Approve(runID string, approved bool) error {
	run := m.Get(runID)
	if run == nil {
		return fmt.Errorf("任务不存在")
	}
	run.mu.Lock()
	if run.State != StateWaitingApproval || run.Pending == nil {
		run.mu.Unlock()
		return fmt.Errorf("当前任务不在等待确认状态")
	}
	p := run.Pending
	if !approved {
		run.Pending = nil
		run.Messages = append(run.Messages, llm.Message{
			Role:       "tool",
			ToolCallID: p.ToolCallID,
			Content:    "用户在界面上拒绝了该操作。请简要说明情况，并询问用户希望如何继续，不要再次尝试执行。",
		})
	}
	run.mu.Unlock()

	_ = m.convs.Append(run.ConversationID, store.Entry{
		Type: "approval", Desc: p.Description, Level: p.Level, Approved: boolPtr(approved),
	})

	run.setState(StateQueued)
	go m.execute(run)
	return nil
}

// Cancel stops a queued/running/waiting run.
func (m *Manager) Cancel(runID string) error {
	run := m.Get(runID)
	if run == nil {
		return fmt.Errorf("任务不存在")
	}
	run.mu.Lock()
	state := run.State
	switch state {
	case StateWaitingApproval:
		run.Pending = nil
		run.mu.Unlock()
		m.runStore.Delete(run.ID)
		run.finishCancelled()
		return nil
	case StateQueued, StateRunning:
		cancel := run.cancel
		run.mu.Unlock()
		if cancel != nil {
			cancel()
		}
		return nil
	default:
		run.mu.Unlock()
		return fmt.Errorf("任务已结束，无需取消")
	}
}

func parseSetOps(args json.RawMessage) ([]tools.Op, error) {
	var a struct {
		Ops []tools.Op `json:"ops"`
	}
	if err := json.Unmarshal(args, &a); err != nil {
		return nil, err
	}
	return a.Ops, nil
}

func truncateStr(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n]) + "…"
}

func boolPtr(b bool) *bool { return &b }
