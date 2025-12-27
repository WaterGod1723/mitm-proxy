-- lua_mitm.d.lua - Lua MITM Proxy类型定义文件
-- 这个文件用于为VS Code Lua插件提供变量提示

--- @class Request
--- @field url string 请求的完整URL
--- @field protocol string 请求协议（http, https等）
--- @field host string 请求主机
--- @field path string 请求路径
--- @field method string 请求方法（GET, POST等）
--- @field headers table<string, string> 请求头表
--- @field has_body boolean 是否存在请求体
--- @field modified_headers table<string, string> 可修改的请求头表
--- @field proxy_headers table<string, string> 代理请求头表（在process_request函数中使用）

--- @class Response
--- @field url string 响应对应的请求URL
--- @field protocol string 响应对应的请求协议（http, https等）
--- @field host string 响应对应的请求主机
--- @field path string 响应对应的请求路径
--- @field status number 响应状态码
--- @field headers table<string, string> 响应头表
--- @field has_body boolean 是否存在响应体
--- @field content_length number 响应内容长度
--- @field modified_headers table<string, string> 可修改的响应头表

--- @class ProxyConfig
--- @field protocol string 代理协议（http, https等）
--- @field address string 代理地址（host:port）
--- @field username string 代理用户名（可选）
--- @field password string 代理密码（可选）

--- 请求处理函数
--- @return nil|{status: number, body: string} 可选的拦截响应
--- @usage
--- function process_request()
---     print("Processing request: " .. method .. " " .. url)
---     modified_headers["X-Proxy"] = "Lua-MITM"
---     return nil
--- end
function process_request() end

--- 响应处理函数
--- @return nil|{status: number, body: string} 可选的拦截响应
--- @usage
--- function process_response()
---     print("Processing response: " .. url .. " (" .. status .. ")")
---     modified_headers["X-Processed"] = "true"
---     return nil
--- end
function process_response() end

--- HTML注入函数
--- @return string 要注入的HTML字符串
--- @usage
--- function inject_html()
---     return "<script>console.log('Injected by Lua')</script>"
--- end
function inject_html() end

--- 代理获取函数
--- @return ProxyConfig 代理配置
--- @usage
--- function get_proxy()
---     return {
---         protocol = "http",
---         address = "proxy.example.com:8080"
---     }
--- end
function get_proxy() end

--- 全局变量：请求URL
--- @type string
url = ""

--- 全局变量：请求协议
--- @type string
protocol = ""

--- 全局变量：请求主机
--- @type string
host = ""

--- 全局变量：请求路径
--- @type string
path = ""

--- 全局变量：请求方法
--- @type string
method = ""

--- 全局变量：请求头表
--- @type table<string, string>
headers = {}

--- 全局变量：是否存在请求体
--- @type boolean
has_body = false

--- 全局变量：响应状态码
--- @type number
status = 0

--- 全局变量：响应内容长度
--- @type number
content_length = 0

--- 全局变量：可修改的请求头表（在process_request函数中使用）
--- @type table<string, string>
modified_headers = {}

--- 全局变量：代理服务器头表（用于配置代理相关头信息）
--- proxy_headers["Proxy-Authorization"] = "Basic dXNlcjpwYXNz"
--- proxy_headers["X-Forwarded-For"] = "192.168.1.1"
proxy_headers = {}

--- 全局变量：可修改的响应头表（在process_response函数中使用）
--- @type table<string, string>
-- modified_headers = {}  -- 注意：在实际使用中，请求和响应处理函数共享同一个modified_headers变量