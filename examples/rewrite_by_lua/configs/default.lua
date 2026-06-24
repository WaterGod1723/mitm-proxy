local portalServer = "web.amh-group.com:8000"
-- 核心为GoRequest函数和GoProxy，go中会调用这两个脚本函数对请求、代理进行重写或者设置

--- GoRequest 函数用于重写HTTP/HTTPS请求的协议、主机、路径和请求头
-- @param protocol string 请求协议，如 "http" 或 "https"
-- @param host string 请求主机，包括域名和端口（如果有），如 "www.baidu.com" 或 "web.amh-group.com:8000"
-- @param path string 请求路径，如 "/search" 或 "/microweb-pc/home"
-- @param headers table 请求头表，键为header名称，值为字符串或字符串数组
-- @return protocol string 重写后的协议
-- @return host string 重写后的主机
-- @return path string 重写后的路径
-- @return bodyFilePath string (可选) 用于替换响应体的本地文件路径，不指定则正常转发请求；当有值时，返回的 headers 会被写入响应头
-- @return headers table (可选) 修改后的请求头表，返回nil或空表保持原请求头不变
-- @description 该函数会在每次请求到达代理服务器时被调用，允许根据请求的协议、主机、路径和请求头进行灵活的重写
--              可以用于实现环境切换、路径重定向、请求头修改、响应mock等功能
--              当返回bodyFilePath时，代理会直接返回该文件的内容作为响应，而不转发请求到原始服务器
--              当返回修改后的headers时，会替换原始请求头（注意：是完整替换，不是合并）
-- @example
-- -- 读取请求头
-- local auth = headers["Authorization"]
-- local userAgent = headers["User-Agent"]
-- 
-- -- 修改请求头
-- headers["Authorization"] = "Bearer new-token"
-- headers["X-Custom-Header"] = "custom-value"
-- 
-- -- 删除请求头（设置为nil或空字符串）
-- headers["Cookie"] = nil
-- 
-- -- 添加多个同名的请求头
-- headers["Accept-Encoding"] = {"gzip", "deflate"}
-- 
-- -- 返回修改后的请求头
-- return protocol, host, path, bodyFilePath, headers
function GoRequest(protocol, host, path, headers)
    local bodyFilePath = ""
    local newHost = host
    local secFetchDest = headers["Sec-Fetch-Dest"] or headers["sec-fetch-dest"]
    
    -- 处理 static.amh-group.com 域名的请求
    if host:match("static") and host:match("%.amh%-group%.com$") and not string.find(path, "^/microweb%-pc") then
        local shouldRewrite = false
        
        -- 规则1：路径以 micro.json 结尾
        if path:match("micro%.json$") then
            shouldRewrite = true
        end
        
        -- 规则2：请求头 sec-fetch-dest 不是 empty（fetch/XHR 请求）
        if secFetchDest and secFetchDest ~= "empty" then
            shouldRewrite = true
        end
        
        -- 如果满足任一规则，重写 host
        if shouldRewrite then
            newHost = portalServer
        end
    end
    
    -- 处理 /src/ 路径下的图片请求
    if path:match("^/src/") and secFetchDest == "image" then
        newHost = portalServer
    end
    
    if path:match("/ws") and host:match("web%.amh%-group%.com") then
        newHost = portalServer
        headers["Origin"] =  protocol .. "://" .. newHost
    end
    
    return protocol, newHost, path, bodyFilePath, headers
end

--- GoProxy 函数用于配置请求的代理服务器
-- @param host string 请求主机，用于确定是否需要使用特定代理
-- @return string 代理服务器地址，格式为 "host:port"，如 "proxy.huawei.com:8080"；返回空字符串表示直连（不使用代理）
-- @description 该函数会在每次请求到达代理服务器时被调用，允许根据请求的主机决定是否使用代理以及使用哪个代理
--              可以用于实现不同域名使用不同代理的场景
function GoProxy(host)
    -- 当前实现：所有请求都直连，不使用额外代理
    return ''
end

--- GoInject 函数用于配置HTML页面注入
-- @param host string 请求主机，用于确定是否需要注入HTML
-- @return string 要注入的HTML文件路径，如 "./inject.html"；返回空字符串表示不注入
-- @description 该函数会在每次响应HTML内容时被调用，允许根据请求的主机决定是否注入额外的HTML内容
--              可以用于注入调试工具、监控脚本或自定义CSS等
function GoInject(host)
    -- 当前实现：所有请求都不注入HTML内容
    return "./inject.html"
end
