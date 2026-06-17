# Lua脚本MITM代理示例

这是一个基于`mitm-proxy`库的Lua脚本扩展示例，展示了如何使用Lua脚本实现精细的HTTP动态修改、HTML注入和代理动态设置能力。

## 功能特性

### 1. HTTP请求动态修改
- 添加自定义请求头
- 修改请求体（支持JSON等格式）
- 拦截特定请求并返回自定义响应
- 请求参数动态调整

### 2. HTTP响应动态修改
- 修改/移除响应头
- 替换响应体内容
- 响应内容压缩
- 拦截特定响应并返回自定义内容

### 3. HTML内容注入
- 注入自定义CSS样式
- 注入JavaScript代码
- 根据URL注入不同内容
- 动态添加页面元素

### 4. 动态代理设置
- 根据URL模式选择不同代理
- 基于时间的代理切换
- 代理故障转移逻辑
- 支持不同协议的代理配置

## 目录结构

```
lua_proxy/
├── main.go          # Go主程序，集成Lua脚本功能
├── request.lua      # 请求处理脚本
├── response.lua     # 响应处理脚本
├── html_inject.lua  # HTML注入脚本
├── proxy.lua        # 动态代理设置脚本
└── README.md        # 说明文档
```

## 快速开始

### 1. 安装依赖

确保已经安装了Go 1.22.1或更高版本，然后安装依赖：

```bash
go mod tidy
```

### 2. 配置VS Code Lua变量提示（可选但推荐）

1. 安装VS Code扩展：`sumneko.lua`（Lua Language Server）
2. 打开项目根目录
3. VS Code会自动加载`.vscode/settings.json`配置
4. Lua脚本会自动获得变量提示

### 3. 运行代理

```bash
go run main.go
```

代理将在默认端口`8080`上启动。

### 4. 配置浏览器

将浏览器的HTTP代理设置为：
- 地址：`localhost`
- 端口：`8080`

## 使用示例

### 修改请求脚本 (request.lua)

```lua
-- 添加自定义请求头
function add_custom_header()
    modified_headers = {}
    for k, v in pairs(headers) do
        modified_headers[k] = v
    end
    modified_headers["X-Proxy-By"] = "Lua-MITM-Proxy"
end
```

### 修改响应脚本 (response.lua)

```lua
-- 替换响应文本
function replace_response_text()
    if string.find(content_type, "text/") then
        modified_body = string.gsub(body, "Hello World", "Hello Lua MITM Proxy!")
    end
end
```

### HTML注入脚本 (html_inject.lua)

```lua
-- 注入自定义JavaScript
function inject_custom_js()
    return [[
        <script>
        console.log('页面被Lua MITM Proxy处理');
        </script>
    ]]
end
```

### 动态代理脚本 (proxy.lua)

```lua
-- 根据URL设置不同代理
function get_proxy_by_url_pattern()
    if string.find(url, "/api/") then
        return {
            protocol = "http",
            address = "api-proxy.example.com:8080"
        }
    end
    return {
        protocol = "http",
        address = "default-proxy.example.com:8080"
    }
end
```

## 自定义扩展

您可以根据需要修改这四个Lua脚本文件，实现更复杂的功能：

### 请求处理脚本
- 修改`request.lua`中的`process_request`函数
- 访问全局变量：`url`、`method`、`headers`、`body`
- 输出变量：`modified_headers`、`modified_body`

### 响应处理脚本
- 修改`response.lua`中的`process_response`函数
- 访问全局变量：`status`、`headers`、`body`
- 输出变量：`modified_headers`、`modified_body`

### HTML注入脚本
- 修改`html_inject.lua`中的`inject_html`函数
- 访问全局变量：`url`、`status`
- 返回值：要注入的HTML字符串

### 动态代理脚本
- 修改`proxy.lua`中的`get_proxy`函数
- 访问全局变量：`url`、`method`
- 返回值：包含`protocol`、`address`、`username`、`password`的表

## 安全考虑

1. 本示例仅用于开发和测试环境
2. 生产环境使用时请确保代理服务器的安全性
3. 不要在公共网络上暴露未受保护的代理
4. 注意保护代理认证信息

## 性能优化

1. 避免在Lua脚本中执行过于复杂的计算
2. 对大型响应体进行处理时注意内存使用
3. 使用高效的字符串处理函数
4. 考虑在Go代码中实现性能敏感的功能

## 故障排查

### 代理无法启动
- 检查端口是否被占用
- 确保证书文件存在且有效
- 查看控制台输出的错误信息

### Lua脚本不生效
- 检查脚本语法是否正确
- 确保函数名与Go代码中的调用一致
- 查看控制台输出的Lua错误信息

### 页面无法加载
- 检查浏览器代理设置是否正确
- 确保代理服务器正常运行
- 查看控制台输出的请求日志

## 扩展建议

1. 添加Lua模块支持，如JSON解析、正则表达式等
2. 实现Lua脚本热加载，无需重启代理即可更新脚本
3. 添加Web管理界面，用于实时修改Lua脚本
4. 实现脚本调试功能，支持断点和变量查看
5. 支持脚本沙箱，提高安全性

## 许可证

MIT License