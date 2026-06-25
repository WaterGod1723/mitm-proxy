package internal

import (
	"encoding/json"
	"log"
	"net/http"
	"sync/atomic"
	"time"

	"github.com/WaterGod1723/mitm-proxy/core_refactor"
)

// AdminServer 提供本地管理接口，仅对监听地址生效。
type AdminServer struct {
	pool          *Pool
	cfg           *Config
	isAllowedCors atomic.Bool
	logger        *log.Logger
	startTime     time.Time
}

// NewAdminServer 创建管理接口。
func NewAdminServer(pool *Pool, cfg *Config, logger *log.Logger) *AdminServer {
	a := &AdminServer{
		pool:      pool,
		cfg:       cfg,
		logger:    logger,
		startTime: time.Now(),
	}
	a.isAllowedCors.Store(true)
	return a
}

// Register 向 MITM 代理注册管理路由。
func (a *AdminServer) Register(m *core_refactor.MITM) {
	m.HandleFunc("/", a.handleIndex)
	m.HandleFunc("/api/status", a.handleStatus)
	m.HandleFunc("/api/config/reload", a.handleReload)
	m.HandleFunc("/api/cors", a.handleCORSStatus)
	m.HandleFunc("/api/cors/open", a.handleOpenCORS)
	m.HandleFunc("/api/cors/close", a.handleCloseCORS)
}

func (a *AdminServer) handleIndex(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Write([]byte("welcome to mitm-proxy rewrite_by_lua_v2"))
}

func (a *AdminServer) handleStatus(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"status":     "ok",
		"config":     a.cfg.Path(),
		"cors":       a.isAllowedCors.Load(),
		"uptime":     time.Since(a.startTime).String(),
		"started_at": a.startTime.Format(time.RFC3339),
	})
}

func (a *AdminServer) handleReload(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost && r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if err := a.cfg.Reload(); err != nil {
		a.logger.Printf("config reload error: %v", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	a.pool.Recreate()
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	json.NewEncoder(w).Encode(map[string]string{"status": "reloaded"})
}

func (a *AdminServer) handleCORSStatus(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	if a.isAllowedCors.Load() {
		w.Write([]byte("true"))
	} else {
		w.Write([]byte("false"))
	}
}

func (a *AdminServer) handleOpenCORS(w http.ResponseWriter, r *http.Request) {
	a.isAllowedCors.Store(true)
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Write([]byte("true"))
}

func (a *AdminServer) handleCloseCORS(w http.ResponseWriter, r *http.Request) {
	a.isAllowedCors.Store(false)
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Write([]byte("false"))
}

// AllowedCORS 返回当前是否允许 CORS。
func (a *AdminServer) AllowedCORS() bool {
	return a.isAllowedCors.Load()
}
