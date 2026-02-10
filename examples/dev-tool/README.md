# Dev Tool Proxy

一个功能强大的开发工具代理，支持路由重写、路径重写、请求/响应跟踪等功能。

## 功能特性

- **路由重写 (Route Rewriting)**: 将特定主机和路径的请求路由到不同的目标主机
- **路径重写 (Path Rewriting)**: 使用正则表达式重写请求路径
- **请求/响应跟踪**: 记录所有经过代理的请求和响应
- **基于Origin的过滤**: 只跟踪特定来源的请求
- **Web界面**: 提供直观的界面查看和管理请求记录
- **动态配置**: 支持运行时修改配置

## 快速开始

### 1. 启动代理

```bash
cd examples/dev-tool
go run main.go -addr :8080
```

代理将在端口 8080 上启动。

### 2. 配置浏览器或应用使用代理

将您的浏览器或应用的代理设置为 `127.0.0.1:8080`。

### 3. 访问Web界面

打开浏览器访问 `http://localhost:8080` 查看请求记录和配置。

## 配置说明

### 路由规则 (Routes)

路由规则用于将特定主机和路径的请求重定向到不同的目标。

```json
{
  "routes": [
    {
      "id": "route_1",
      "name": "API Route Rule",
      "match_host": "api.example.com",
      "match_path_prefix": "/v1",
      "target_host": "api-dev.example.com",
      "target_port": 443,
      "enabled": true
    }
  ]
}
```

- `match_host`: 匹配的主机名
- `match_path_prefix`: 匹配的路径前缀
- `target_host`: 目标主机
- `target_port`: 目标端口
- `enabled`: 是否启用规则

### 路径重写规则 (Rewrites)

路径重写规则使用正则表达式重写请求路径。

```json
{
  "rewrites": [
    {
      "id": "rewrite_1",
      "name": "API Version Rewrite",
      "match_path": "/api/test/(.*)",
      "rewrite_path": "/api/v1/$1",
      "target_host": "",
      "enabled": true
    }
  ]
}
```

- `match_path`: 用于匹配路径的正则表达式
- `rewrite_path`: 重写后的路径，支持捕获组（如 $1, $2）
- `target_host`: 可选，如果设置将同时更改主机
- `enabled`: 是否启用规则

### Origin规则 (Origins)

Origin规则用于过滤只跟踪特定来源的请求。

```json
{
  "origins": [
    {
      "id": "origin_1",
      "name": "Example.com",
      "origin": "example.com",
      "enabled": true,
      "record_body": true
    }
  ]
}
```

- `origin`: 匹配的Origin或Referer
- `enabled`: 是否启用规则
- `record_body`: 是否记录请求/响应体

## 配置管理

### 通过Web界面配置

1. 访问 `http://localhost:8080`
2. 在"Configuration"部分查看和编辑配置
3. 点击"Update Configuration"保存更改

### 通过API配置

```bash
# 获取当前配置
curl http://localhost:8080/api/config

# 更新配置
curl -X POST http://localhost:8080/api/config \
  -H "Content-Type: application/json" \
  -d @config.json
```

## 使用示例

### 示例1: API版本迁移

将旧版本API请求重写到新版本：

```json
{
  "rewrites": [
    {
      "id": "api_migration",
      "name": "Migrate v1 to v2",
      "match_path": "/api/v1/(.*)",
      "rewrite_path": "/api/v2/$1",
      "target_host": "",
      "enabled": true
    }
  ]
}
```

### 示例2: 服务路由

将特定API请求路由到开发环境：

```json
{
  "routes": [
    {
      "id": "dev_route",
      "name": "Route to Dev",
      "match_host": "api.production.com",
      "match_path_prefix": "/test",
      "target_host": "api.development.com",
      "target_port": 443,
      "enabled": true
    }
  ]
}
```

### 示例3: 组合使用路由和重写

先重写路径，再路由到目标：

1. 路径重写规则将 `/legacy/api/(.*)` 重写为 `/modern/api/$1`
2. 路由规则将匹配 `/modern` 的请求路由到新的服务

## Web界面功能

### 请求列表
- 查看所有捕获的请求
- 搜索特定请求
- 查看请求/响应的详细内容
- 分页浏览

### 配置管理
- 查看和编辑当前配置
- 更新路由、重写和Origin规则

### Origin规则管理
- 查看现有的Origin规则
- 添加新的Origin规则
- 启用/禁用规则

## 测试配置

项目包含预配置的测试规则，位于 `test-config.json` 文件中，可直接用于测试各种场景。

## API接口

- `GET /api/requests[?q=query]`: 获取请求记录，可选搜索查询
- `POST /api/clear-requests`: 清除所有请求记录
- `GET /api/config`: 获取当前配置
- `POST /api/config`: 更新配置
- `GET /`: Web界面主页

## 证书配置

如果需要处理HTTPS流量，请确保安装了CA证书：

```bash
# 生成CA证书（如果还没有）
./cert/generateCA.sh

# 或使用现有的证书
# cert/cert.pem - 证书文件
# cert/key.pem - 私钥文件
```

## 故障排除

### 请求没有被记录
- 检查Origin规则是否配置正确
- 确认 `record_requests` 设置为 `true`
- 验证代理设置是否正确

### 路由规则不工作
- 检查 `match_host` 和 `match_path_prefix` 是否匹配
- 确认规则的 `enabled` 设置为 `true`
- 验证目标主机和端口是否可达

### 路径重写不生效
- 检查正则表达式是否正确
- 确认规则的 `enabled` 设置为 `true`
- 验证路径是否匹配预期模式

## 性能考虑

- 请求历史记录数量由 `max_request_history` 控制，默认为1000
- 大量请求记录可能影响性能，建议定期清理
- 启用请求体记录会增加内存使用

## 安全注意

- 代理会记录请求和响应内容，请注意敏感信息
- 证书文件应妥善保管
- 仅在开发和测试环境中使用此代理