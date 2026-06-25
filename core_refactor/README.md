# core_refactor —— MITM 代理核心重构版

本目录是对 `core/` 的重新设计与实现，目标是解决原实现中职责混杂、并发隐患、
测试困难以及证书加载耦合等问题，同时保持对外的能力等价。

## 主要改进

1. **显式初始化，不再 panic**
   - 原 `core/singer.go` 在 `init()` 中加载 `./cert/cert.pem` 与 `./cert/key.pem`，
     文件缺失会直接 panic，导致示例和测试难以在不同目录运行。
   - 重构后 `New(...)` 返回 `(*MITM, error)`，证书路径、CA 对象均可通过 `Option` 注入。

2. **职责拆分，模块清晰**
   - `ca.go`：根证书加载、缓存与主机证书签名。
   - `client.go` / `server.go`：客户端与上游连接的读写、TLS 升级。
   - `session.go`：单个客户端连接的生命周期、请求串行读取、响应顺序写入、WebSocket 隧道。
   - `proxy.go`：上游代理配置与 Basic 认证。
   - `html_inject.go`：HTML 响应体注入。
   - `response_writer.go`：实现标准 `http.ResponseWriter`。
   - `mitm.go` + `options.go`：公共 API 与生命周期管理。

3. **更安全的并发模型**
   - 使用每个连接独立的 `session.writeCh` 保证响应顺序。
   - 上游连接池 `servers` 使用 `sync.Mutex` 保护。
   - 证书缓存使用 `sync.RWMutex` 保护。
   - `Stop()` 关闭监听并等待所有连接处理完毕。

4. **更自然的回调接口**
   - 原 `ProcessRequest` / `ProcessResponse` 返回 `ResponseWriteFunc`，职责倒置。
   - 重构后回调返回 `*http.Response`：
     - `WithRequestHandler`：返回非 nil 则直接短路响应。
     - `WithResponseHandler`：返回非 nil 则替换原始响应。
   - `HandleFunc` 使用标准 `http.HandlerFunc`。

5. **ResponseWriter 符合 `http.ResponseWriter` 契约**
   - 实现 `WriteHeader` / `Write` / `Header` / `Flush`。
   - 首次 `Write` 时自动补全 `Content-Length`，避免无长度响应导致客户端挂起。

6. **可测试性**
   - 测试用例在临时目录中动态生成根证书，不依赖仓库 `cert/` 目录。
   - 覆盖 CA 加载/签名缓存、代理解析、HTML 注入、管理接口、响应写入等核心路径。

## 快速开始

```go
package main

import (
    "log"
    "net/http"

    "github.com/WaterGod1723/mitm-proxy/core_refactor"
)

func main() {
    m, err := core_refactor.New(
        core_refactor.WithCAPath("./cert/cert.pem", "./cert/key.pem"),
        core_refactor.WithLogger(log.Default()),
        core_refactor.WithRequestHandler(func(req *http.Request) *http.Response {
            // 返回 nil 继续转发；返回 *http.Response 则直接响应。
            return nil
        }),
        core_refactor.WithProxy(core_refactor.DirectProxy),
    )
    if err != nil {
        log.Fatal(err)
    }

    m.HandleFunc("/api/status", func(w http.ResponseWriter, r *http.Request) {
        w.Write([]byte("ok"))
    })

    log.Fatal(m.Start("0.0.0.0:8003"))
}
```

## 选项说明

| 选项 | 说明 |
|---|---|
| `WithCAPath(cert, key)` | 指定根证书路径 |
| `WithCA(ca)` | 直接注入已加载的 `*CA` |
| `WithLogger(logger)` | 日志输出；`nil` 关闭日志 |
| `WithRequestHandler(fn)` | 请求预处理钩子 |
| `WithResponseHandler(fn)` | 响应后处理钩子 |
| `WithHTMLInjector(fn)` | HTML 注入器 |
| `WithProxy(fn)` | 上游代理选择器 |
| `WithDialTimeout(d)` | 上游连接超时 |
| `WithIdleTimeout(d)` | 客户端空闲超时 |

## 迁移提示

- 原 `core.Container` 对应 `core_refactor.MITM`。
- 原 `core.NewMITM()` 对应 `core_refactor.New(...)`，会返回 `error`。
- 原 `core.ProxyArray` 替换为 `core_refactor.Proxy`。
- 原 `core.ResponseWriteFunc` 不再使用；改为直接返回 `*http.Response`。
- 若示例需要完全兼容原 API，可在 `core_refactor` 之上再包一层适配器。
