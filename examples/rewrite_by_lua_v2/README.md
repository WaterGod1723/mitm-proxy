# rewrite_by_lua_v2 —— 基于 core_refactor 的 MITM 代理示例

本示例是对 `examples/rewrite_by_lua` 的重构版本，基于 `core_refactor` 构建，目标是更清晰的代码结构、更稳定的热重载机制以及更易于理解的配置方式。

## 主要改进

1. **基于 `core_refactor`**
   - 使用显式初始化的 `core_refactor.New(...)`，不再依赖全局 `init()` 加载证书。
   - 出错时返回 `error`，启动失败信息更明确。

2. **职责拆分**
   - `internal/config.go`：配置加载与热重载。
   - `internal/lua.go`：Lua 状态池与 Go/Lua 数据转换。
   - `internal/handlers.go`：请求重写、代理选择、HTML 注入。
   - `internal/server.go`：本地管理接口。
   - `internal/scripts.go`：Chrome 启动脚本与本机 IP 工具。

3. **非中断式热重载**
   - 原示例通过 `exec` 重启进程实现配置重载。
   - 新版本检测到配置文件变更后，仅清空 Lua 状态池，后续请求自动使用新脚本，无需重启进程。

4. **更安全的并发模型**
   - Lua 状态池带容量上限与互斥保护，避免状态复用冲突。
   - 代理选择、HTML 注入结果带本地缓存。

5. **标准化管理接口**
   - `GET  /`：欢迎信息
   - `GET  /api/status`：运行状态
   - `POST /api/config/reload`：手动重载配置
   - `GET  /api/cors`：CORS 开关状态
   - `GET  /api/cors/open`：开启 CORS
   - `GET  /api/cors/close`：关闭 CORS

## 目录结构

```
rewrite_by_lua_v2/
├── main.go                 # 程序入口
├── internal/
│   ├── config.go           # 配置加载与热重载
│   ├── lua.go              # Lua 状态池与互操作
│   ├── handlers.go         # 请求/响应/代理/HTML 注入处理
│   ├── server.go           # 本地管理接口
│   └── scripts.go          # Chrome 脚本与 IP 工具
├── cert/                   # CA 证书目录
├── configs/                # Lua 配置文件
│   ├── default.lua
│   ├── custom.lua
│   └── README.md
├── inject.html             # HTML 注入示例
├── build.sh                # 跨平台构建脚本
└── README.md               # 本文件
```

## 编译运行

```bash
cd examples/rewrite_by_lua_v2

# 编译
go build -o mitm main.go

# 运行（使用默认配置 configs/default.lua）
./mitm

# 指定监听地址和配置
./mitm -addr 0.0.0.0:8888 -config custom -v
```

## 命令行参数

| 参数 | 默认值 | 说明 |
|------|--------|------|
| `-addr` | `0.0.0.0:8003` | 代理监听地址 |
| `-config` | `default` | 配置名称或完整路径 |
| `-cert` | `./cert/cert.pem` | 根证书路径 |
| `-key` | `./cert/key.pem` | 根证书私钥路径 |
| `-v` | `false` | 启用详细日志 |

## 配置文件

配置文件为 Lua 脚本，需实现以下三个全局函数：

```lua
--- 请求重写
-- @return protocol string      重写后的协议
-- @return host string          重写后的主机
-- @return path string          重写后的路径
-- @return bodyFilePath string  本地文件路径，非空则直接返回该文件
-- @return headers table        完整替换后的请求头（nil/空表表示不变）
function GoRequest(protocol, host, path, headers)
    return protocol, host, path, "", headers
end

--- 代理选择
-- @return string 代理 URL，空字符串表示直连
function GoProxy(host)
    return ''
end

--- HTML 注入
-- @return string 要注入的 HTML 文件路径，空字符串表示不注入
function GoInject(host)
    return ""
end
```

详细说明见 `configs/README.md`。

## 热重载

修改 `configs/*.lua` 后，程序会在 2 秒内自动检测到变更并重载 Lua 脚本，无需重启。也可以手动触发：

```bash
curl -X POST http://127.0.0.1:8003/api/config/reload
```

## 生成证书

若 `cert/` 目录下没有证书或需要自定义证书：

```bash
cd cert
bash generateCA.sh
```

然后将 `cert/cert.pem`（或生成的 `cert.crt`）安装到系统/浏览器并信任。

## 浏览器代理设置

运行后会在当前目录生成 `start_chrome.sh`，可直接执行启动已配置代理的 Chrome：

```bash
./start_chrome.sh
```

也可手动设置系统/浏览器代理为程序监听地址（如 `127.0.0.1:8003`）。

## 跨平台构建

```bash
./build.sh
```

构建产物输出到 `dist/` 目录。

## 与原示例的差异

| 项 | `rewrite_by_lua` | `rewrite_by_lua_v2` |
|---|---|---|
| 核心库 | `core` | `core_refactor` |
| 初始化 | 全局 init，可能 panic | `New(...)` 返回 error |
| 热重载 | `exec` 重启进程 | 清空 Lua 池，非中断重载 |
| 代码组织 | 单文件 `main.go` | 按职责拆分 `internal` 包 |
| 管理接口 | 简单接口 | 增加 `/api/status`、`/api/config/reload` 等 |
| 日志 | 直接打印 | 统一 logger，支持 `-v` |

## 注意事项

1. MITM 代理需要客户端信任根证书，否则 HTTPS 会触发安全警告。
2. 不要在生产环境或未经授权的网络中使用本代理。
3. 提交的 CA 私钥仅用于本地开发，请勿用于生产环境。
