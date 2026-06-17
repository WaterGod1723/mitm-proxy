-- request.lua - HTTP请求处理脚本
-- 可用全局变量：
--   url: string - 请求的完整URL
--   method: string - 请求方法（GET, POST等）
--   headers: table - 请求头表
--   has_body: boolean - 是否存在请求体
--   modified_headers: table - 可修改的请求头表（用于修改请求头）

-- 示例1: 添加自定义请求头
function add_custom_header()
    modified_headers = {}
    proxy_headers = {}
    
    -- 复制原请求头
    for k, v in pairs(headers) do
        modified_headers[k] = v
    end
    
    -- 添加自定义请求头
    modified_headers["X-Proxy-By"] = "Lua-MITM-Proxy"
    modified_headers["X-Request-ID"] = "request-" .. os.time()
end

-- 示例2: 修改请求头（如修改User-Agent）
function modify_request_headers()
    if modified_headers == nil then
        modified_headers = {}
        for k, v in pairs(headers) do
            modified_headers[k] = v
        end
    end
    
    -- 修改User-Agent
    modified_headers["User-Agent"] = "Mozilla/5.0 (compatible; Lua-MITM-Proxy/1.0)"
    
    -- 移除某些敏感头
    modified_headers["Cookie"] = nil
    modified_headers["Authorization"] = nil
end

-- 示例3: 拦截特定请求并返回自定义响应
function intercept_specific_request()
    if url == "http://example.com/blocked" then
        return {
            status = 403,
            body = "This request has been blocked by Lua MITM Proxy"
        }
    end
    return nil
end

-- 示例4: 根据URL修改请求头
function modify_headers_by_url()
    if modified_headers == nil then
        modified_headers = {}
        for k, v in pairs(headers) do
            modified_headers[k] = v
        end
    end
    
    if string.find(url, "api.example.com") then
        modified_headers["X-Api-Version"] = "v2"
    elseif string.find(url, "static.example.com") then
        modified_headers["Cache-Control"] = "max-age=3600"
    end
end

-- 示例5: 使用proxy_headers配置代理相关头
function setup_proxy_headers()
    -- 初始化代理头表
    proxy_headers = {
        ["Proxy-Authorization"] = "Basic dXNlcjpwYXNz",
        ["X-Forwarded-For"] = "192.168.1.1",
        ["X-Proxy-Id"] = "lua-mitm-001"
    }
    
    -- 根据请求方法添加不同的代理头
    if method == "POST" then
        proxy_headers["X-Proxy-Method"] = "POST"
    end
    
    -- 打印代理头信息
    for k, v in pairs(proxy_headers) do
        print("Proxy Header:", k, "=", v)
    end
end

-- 主请求处理函数
function process_request()
    print("Processing request: " .. method .. " " .. url)
    
    -- 执行各种请求处理函数
    add_custom_header()
    modify_request_headers()
    modify_headers_by_url()
    setup_proxy_headers()
    
    -- 检查是否需要拦截请求
    local intercept_response = intercept_specific_request()
    if intercept_response then
        return intercept_response
    end
    
    -- 返回nil表示继续处理原始请求
    return nil
end