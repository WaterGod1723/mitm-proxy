package main

import (
	"flag"
	"log"
	"net"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/WaterGod1723/mitm-proxy/core_refactor"
	"github.com/WaterGod1723/mitm-proxy/examples/rewrite_by_lua_v2/internal"
)

func main() {
	addr := flag.String("addr", "0.0.0.0:8003", "代理服务监听地址，格式为 host:port")
	configName := flag.String("config", "default", "配置文件名（不含.lua后缀）或完整路径")
	certPath := flag.String("cert", "./cert/cert.pem", "根证书路径")
	keyPath := flag.String("key", "./cert/key.pem", "根证书私钥路径")
	verbose := flag.Bool("v", false, "启用详细日志")
	flag.Parse()

	logger := newLogger(*verbose)

	// 加载 Lua 配置。
	configPath := internal.ResolveConfigPath(*configName)
	cfg, err := internal.LoadConfig(configPath)
	if err != nil {
		logger.Fatalf("load config error: %v (path: %s)", err, configPath)
	}
	logger.Printf("loaded config: %s", cfg.Path())

	// Lua 状态池。
	pool := internal.NewPool(cfg, 8)

	// 配置热重载：配置变更时清空状态池，后续请求使用新脚本。
	cfg.SetOnChange(func(script string) {
		logger.Println("config changed, reloading lua states...")
		pool.Recreate()
	})
	stopWatch := make(chan struct{})
	defer close(stopWatch)
	go cfg.Watch(2*time.Second, stopWatch)

	// 构造 MITM 代理。
	m, err := core_refactor.New(
		core_refactor.WithCAPath(*certPath, *keyPath),
		core_refactor.WithLogger(logger),
		core_refactor.WithRequestHandler(internal.NewRequestHandler(pool, logger)),
		core_refactor.WithResponseHandler(internal.NewResponseHandler(pool, logger)),
		core_refactor.WithProxy(internal.NewProxySelector(pool, logger)),
		core_refactor.WithHTMLInjector(internal.NewHTMLInjector(pool, logger)),
	)
	if err != nil {
		logger.Fatalf("create mitm proxy error: %v", err)
	}

	// 注册管理接口。
	admin := internal.NewAdminServer(pool, cfg, logger)
	admin.Register(m)

	// 计算监听与展示地址。
	localIP := internal.GetLocalIP()
	host, port, _ := net.SplitHostPort(*addr)
	if port == "" {
		port = "8003"
	}
	if host == "" || host == "0.0.0.0" {
		host = localIP
	}
	displayAddr := net.JoinHostPort(host, port)

	cwd, _ := os.Getwd()
	chromeScriptPath := filepath.Join(cwd, "start_chrome.sh")
	if err := internal.GenerateChromeScript(localIP, port, chromeScriptPath); err != nil {
		logger.Printf("generate chrome script error: %v", err)
	} else {
		logger.Printf("chrome launcher script: %s", chromeScriptPath)
	}

	// 启动代理。
	listenAddr := *addr
	logger.Printf("mitm-proxy started at http://%s (listen on %s)", displayAddr, listenAddr)

	errCh := make(chan error, 1)
	go func() {
		errCh <- m.Start(listenAddr)
	}()

	// 优雅退出。
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)
	select {
	case err := <-errCh:
		if err != nil {
			logger.Fatalf("mitm proxy error: %v", err)
		}
	case sig := <-sigCh:
		logger.Printf("received signal %v, shutting down...", sig)
		if err := m.Stop(); err != nil {
			logger.Printf("stop mitm proxy error: %v", err)
		}
	}
}

// newLogger 根据 verbose 创建日志输出器。
func newLogger(verbose bool) *log.Logger {
	if verbose {
		return log.New(os.Stdout, "[rewrite] ", log.LstdFlags|log.Lmicroseconds)
	}
	return log.New(os.Stdout, "[rewrite] ", log.LstdFlags)
}
