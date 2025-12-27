-- example_usage.lua - Lua脚本使用示例（带变量提示）
-- 此文件仅用于展示如何使用变量提示，不参与实际运行

-- 请求处理脚本示例
function process_request_example()
    -- 输入 "url." 会提示 url 是 string 类型
    print("请求URL: " .. url)
    
    -- 输入 "method." 会提示 method 是 string 类型
    if method == "POST" then
        print("这是一个POST请求")
    end
    
    -- 输入 "headers[" 会提示 headers 是 table<string, string> 类型
    local content_type = headers["Content-Type"] or "unknown"
    print("内容类型: " .. content_type)
    
    -- 输入 "has_body" 会提示 has_body 是 boolean 类型
    if has_body then
        print("请求包含body")
    end
    
    -- 输入 "modified_headers[" 会提示 modified_headers 是 table<string, string> 类型
    modified_headers = {}
    for k, v in pairs(headers) do
        modified_headers[k] = v
    end
    modified_headers["X-Custom-Header"] = "value"
end

-- 响应处理脚本示例
function process_response_example()
    -- 输入 "url." 会提示 url 是 string 类型
    print("响应URL: " .. url)
    
    -- 输入 "status." 会提示 status 是 number 类型
    if status == 200 then
        print("请求成功")
    elseif status == 404 then
        print("资源未找到")
    end
    
    -- 输入 "content_length." 会提示 content_length 是 number 类型
    print("内容长度: " .. content_length)
    
    -- 输入 "modified_headers[" 会提示 modified_headers 是 table<string, string> 类型
    modified_headers = {}
    for k, v in pairs(headers) do
        modified_headers[k] = v
    end
    modified_headers["X-Processed"] = "true"
end

-- HTML注入脚本示例
function inject_html_example()
    -- 输入 "url." 会提示 url 是 string 类型
    if string.find(url, "example.com") then
        return [[
            <div style="background:yellow;padding:10px;">
                这是针对example.com的注入内容
            </div>
        ]]
    end
    return ""
end

-- 代理获取脚本示例
function get_proxy_example()
    -- 输入 "url." 会提示 url 是 string 类型
    if string.find(url, "/api/") then
        -- 输入 "return {" 会提示返回值类型是 ProxyConfig
        return {
            protocol = "http",
            address = "api-proxy.example.com:8080"
        }
    else
        return {
            protocol = "http",
            address = "default-proxy.example.com:8080"
        }
    end
end

print("Lua变量提示示例文件")
print("在VS Code中编辑此文件时，输入变量名后会自动显示类型提示")