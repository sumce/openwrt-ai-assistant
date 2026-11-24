package server

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"strings"

	"github.com/sumce/openwrt-ai-assistant/internal/config"
	"github.com/sumce/openwrt-ai-assistant/internal/llm"
	"github.com/sumce/openwrt-ai-assistant/internal/safety"
)

// ---- small helpers ----

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

func readJSON(w http.ResponseWriter, r *http.Request, v any) bool {
	if err := json.NewDecoder(r.Body).Decode(v); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "请求体解析失败: " + err.Error()})
		return false
	}
	return true
}

func fail(w http.ResponseWriter, code int, msg string) {
	writeJSON(w, code, map[string]any{"error": msg})
}

func randHex(n int) string {
	b := make([]byte, n)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

// ---- auth ----

func (s *Server) handleBootstrap(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"initialized": s.cfg.Auth.Initialized(),
		"version":     Version,
		"dev":         s.cfg.Dev,
	})
}

func (s *Server) handleSetup(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Password string `json:"password"`
	}
	if !readJSON(w, r, &body) {
		return
	}
	if len(body.Password) < 6 {
		fail(w, http.StatusBadRequest, "密码至少 6 位")
		return
	}
	if s.cfg.Auth.Initialized() {
		fail(w, http.StatusBadRequest, "已初始化，请直接登录")
		return
	}
	if err := s.cfg.Auth.Initialize(body.Password); err != nil {
		fail(w, http.StatusInternalServerError, err.Error())
		return
	}
	s.setSession(w)
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (s *Server) handleLogin(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Password string `json:"password"`
	}
	if !readJSON(w, r, &body) {
		return
	}
	if !s.cfg.Auth.Verify(body.Password) {
		fail(w, http.StatusUnauthorized, "密码错误")
		return
	}
	s.setSession(w)
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (s *Server) handleLogout(w http.ResponseWriter, r *http.Request) {
	s.clearSession(w)
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (s *Server) handleChangePassword(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Current string `json:"current"`
		New     string `json:"new"`
	}
	if !readJSON(w, r, &body) {
		return
	}
	if len(body.New) < 6 {
		fail(w, http.StatusBadRequest, "新密码至少 6 位")
		return
	}
	if err := s.cfg.Auth.ChangePassword(body.Current, body.New); err != nil {
		fail(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// ---- settings ----

func (s *Server) handleGetSettings(w http.ResponseWriter, r *http.Request) {
	st, err := s.cfg.Settings.Load()
	if err != nil {
		fail(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, st)
}

// handlePutSettings persists provider/security settings. enabled/port/listen
// are LuCI-owned and ignored here.
func (s *Server) handlePutSettings(w http.ResponseWriter, r *http.Request) {
	var body config.Settings
	if !readJSON(w, r, &body) {
		return
	}
	cur, err := s.cfg.Settings.Load()
	if err != nil {
		fail(w, http.StatusInternalServerError, err.Error())
		return
	}

	cur.ConfirmWrite = body.ConfirmWrite
	cur.DefaultProvider = body.DefaultProvider
	cur.Providers = body.Providers[:0]
	for _, p := range body.Providers {
		p.Name = strings.TrimSpace(p.Name)
		p.BaseURL = strings.TrimRight(strings.TrimSpace(p.BaseURL), "/")
		p.Model = strings.TrimSpace(p.Model)
		p.APIKey = strings.TrimSpace(p.APIKey)
		if p.Name == "" || p.BaseURL == "" {
			continue
		}
		if p.ID == "" {
			p.ID = "p" + randHex(4)
		}
		cur.Providers = append(cur.Providers, p)
	}
	if cur.DefaultProvider != "" && cur.Provider(cur.DefaultProvider) == nil {
		cur.DefaultProvider = ""
	}

	if err := s.cfg.Settings.Save(cur); err != nil {
		fail(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, cur)
}

func (s *Server) handleTestProvider(w http.ResponseWriter, r *http.Request) {
	var body struct {
		ID      string `json:"id"`
		BaseURL string `json:"base_url"`
		APIKey  string `json:"api_key"`
	}
	if !readJSON(w, r, &body) {
		return
	}
	baseURL, apiKey := body.BaseURL, body.APIKey
	if body.ID != "" && baseURL == "" {
		st, err := s.cfg.Settings.Load()
		if err != nil {
			fail(w, http.StatusInternalServerError, err.Error())
			return
		}
		p := st.Provider(body.ID)
		if p == nil {
			fail(w, http.StatusNotFound, "提供方不存在")
			return
		}
		baseURL, apiKey = p.BaseURL, p.APIKey
	}
	if baseURL == "" {
		fail(w, http.StatusBadRequest, "base_url 不能为空")
		return
	}
	models, err := llm.NewClient(strings.TrimRight(baseURL, "/"), apiKey).Test(context.Background())
	writeJSON(w, http.StatusOK, map[string]any{
		"ok":     err == nil,
		"models": models,
		"error":  errString(err),
	})
}

func errString(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

// ---- conversations ----

func (s *Server) handleListConvs(w http.ResponseWriter, r *http.Request) {
	list, err := s.cfg.Convs.List()
	if err != nil {
		fail(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, list)
}

func (s *Server) handleDeleteConv(w http.ResponseWriter, r *http.Request) {
	if err := s.cfg.Convs.Delete(r.PathValue("id")); err != nil {
		fail(w, http.StatusNotFound, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (s *Server) handleConvMessages(w http.ResponseWriter, r *http.Request) {
	entries, err := s.cfg.Convs.Entries(r.PathValue("id"))
	if err != nil {
		fail(w, http.StatusNotFound, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, entries)
}

// ---- chat & runs ----

func (s *Server) handleChat(w http.ResponseWriter, r *http.Request) {
	var body struct {
		ConversationID string `json:"conversation_id"`
		Message        string `json:"message"`
	}
	if !readJSON(w, r, &body) {
		return
	}
	run, err := s.cfg.Manager.Start(body.ConversationID, body.Message)
	if err != nil {
		fail(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"run_id":         run.ID,
		"conversation_id": run.ConversationID,
	})
}

func (s *Server) handleActiveRuns(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, s.cfg.Manager.ListActive())
}

func (s *Server) handleRunSnapshot(w http.ResponseWriter, r *http.Request) {
	run := s.cfg.Manager.Get(r.PathValue("id"))
	if run == nil {
		fail(w, http.StatusNotFound, "任务不存在")
		return
	}
	writeJSON(w, http.StatusOK, run.Snapshot())
}

func (s *Server) handleApprove(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Approved bool `json:"approved"`
	}
	if !readJSON(w, r, &body) {
		return
	}
	if err := s.cfg.Manager.Approve(r.PathValue("id"), body.Approved); err != nil {
		fail(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (s *Server) handleCancel(w http.ResponseWriter, r *http.Request) {
	if err := s.cfg.Manager.Cancel(r.PathValue("id")); err != nil {
		fail(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// ---- backups ----

func (s *Server) handleListBackups(w http.ResponseWriter, r *http.Request) {
	list, err := s.cfg.Backups.List()
	if err != nil {
		fail(w, http.StatusInternalServerError, err.Error())
		return
	}
	if list == nil {
		list = []safety.BackupInfo{}
	}
	writeJSON(w, http.StatusOK, list)
}

func (s *Server) handleRestoreBackup(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Name string `json:"name"`
	}
	if !readJSON(w, r, &body) {
		return
	}
	msg, err := s.cfg.Backups.Restore(r.Context(), body.Name)
	if err != nil {
		fail(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "message": msg})
}
