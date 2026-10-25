package main

import (
	"embed"
	"encoding/json"
	"io/fs"
	"log"
	"net/http"
	"os"
)

//go:embed web/*
var webFS embed.FS

// ServerConfig HTTP 服务配置
type ServerConfig struct {
	BindAddr string
	Port     int
}

// StartHTTPServer 启动 HTTP 服务
func StartHTTPServer(cfg ServerConfig) error {
	mux := http.NewServeMux()

	// 静态文件服务 (内嵌 web/ 目录)
	sub, err := fs.Sub(webFS, "web")
	if err != nil {
		return err
	}
	mux.Handle("/", http.FileServer(http.FS(sub)))

	// API 路由
	mux.HandleFunc("/api/status", handleStatus(cfg))
	mux.HandleFunc("/api/forward/list", handleForwardList)
	mux.HandleFunc("/api/forward/add", handleForwardAdd)
	mux.HandleFunc("/api/forward/delete", handleForwardDelete)
	mux.HandleFunc("/api/ruleset", handleGetRuleset)
	mux.HandleFunc("/api/ruleset/save", handleSaveRuleset)
	mux.HandleFunc("/api/ruleset/flush", handleFlushRuleset)

	server := &http.Server{
		Addr:    cfg.BindAddr,
		Handler: mux,
	}

	return server.ListenAndServe()
}

func jsonResponse(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(data)
}

func handleStatus(cfg ServerConfig) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		forwardEnabled := false
		if b, err := os.ReadFile("/proc/sys/net/ipv4/ip_forward"); err == nil {
			if len(b) > 0 && b[0] == '1' {
				forwardEnabled = true
			}
		}

		jsonResponse(w, http.StatusOK, map[string]interface{}{
			"bind_addr":    cfg.BindAddr,
			"port":         cfg.Port,
			"ipv4_forward": forwardEnabled,
			"local_ips":    GetLocalIPs(),
		})
	}
}

func handleForwardList(w http.ResponseWriter, r *http.Request) {
	rules, err := GetForwardRules()
	if err != nil {
		jsonResponse(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	if rules == nil {
		rules = []ForwardRule{}
	}
	jsonResponse(w, http.StatusOK, rules)
}

type AddForwardReq struct {
	Protocol string `json:"protocol"`
	SrcPort  int    `json:"src_port"`
	DstHost  string `json:"dst_host"`
	DstPort  int    `json:"dst_port"`
	Comment  string `json:"comment"`
}

func handleForwardAdd(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
		return
	}

	var req AddForwardReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonResponse(w, http.StatusBadRequest, map[string]interface{}{"success": false, "error": "无效的 JSON 数据"})
		return
	}

	if req.SrcPort <= 0 || req.SrcPort > 65535 {
		jsonResponse(w, http.StatusBadRequest, map[string]interface{}{"success": false, "error": "监听端口必须在 1-65535 之间"})
		return
	}

	if err := AddForwardRule(req.Protocol, req.SrcPort, req.DstHost, req.DstPort, req.Comment); err != nil {
		log.Printf("[API] 添加端口转发失败: %v", err)
		jsonResponse(w, http.StatusInternalServerError, map[string]interface{}{"success": false, "error": err.Error()})
		return
	}

	jsonResponse(w, http.StatusOK, map[string]interface{}{"success": true})
}

type DeleteForwardReq struct {
	Handle int `json:"handle"`
}

func handleForwardDelete(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
		return
	}

	var req DeleteForwardReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonResponse(w, http.StatusBadRequest, map[string]interface{}{"success": false, "error": "无效的参数"})
		return
	}

	if err := DeleteForwardRuleByHandle(req.Handle); err != nil {
		log.Printf("[API] 删除规则失败: %v", err)
		jsonResponse(w, http.StatusInternalServerError, map[string]interface{}{"success": false, "error": err.Error()})
		return
	}

	jsonResponse(w, http.StatusOK, map[string]interface{}{"success": true})
}

func handleGetRuleset(w http.ResponseWriter, r *http.Request) {
	out, err := GetRawRuleset()
	if err != nil {
		jsonResponse(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	jsonResponse(w, http.StatusOK, map[string]string{"ruleset": out})
}

func handleSaveRuleset(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
		return
	}

	if err := SaveRulesetToFile(); err != nil {
		jsonResponse(w, http.StatusInternalServerError, map[string]interface{}{"success": false, "error": err.Error()})
		return
	}
	jsonResponse(w, http.StatusOK, map[string]interface{}{"success": true})
}

func handleFlushRuleset(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
		return
	}

	if err := FlushAllRules(); err != nil {
		jsonResponse(w, http.StatusInternalServerError, map[string]interface{}{"success": false, "error": err.Error()})
		return
	}
	jsonResponse(w, http.StatusOK, map[string]interface{}{"success": true})
}
