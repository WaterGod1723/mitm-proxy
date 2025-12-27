-- response.lua - HTTP响应处理脚本
-- 可用全局变量：
--   url: string - 响应对应的请求URL
--   status: number - 响应状态码
--   headers: table - 响应头表
--   has_body: boolean - 是否存在响应体
--   content_length: number - 响应内容长度
--   modified_headers: table - 可修改的响应头表（用于修改响应头）

-- 示例1: 修改响应头，移除安全相关头
function modify_security_headers()
    modified_headers = {}
    
    -- 复制原响应头，但移除某些安全头
    for k, v in pairs(headers) do
        -- 移除CSP、X-Frame-Options等安全头
        if k ~= "Content-Security-Policy" and k ~= "X-Frame-Options" and k ~= "X-XSS-Protection" then
            modified_headers[k] = v
        end
    end
    
    -- 添加自定义响应头
    modified_headers["X-Proxy-Processed"] = "true"
    modified_headers["X-Response-Time"] = os.time()
end

-- 示例2: 修改缓存控制头
function modify_cache_control()
    if modified_headers == nil then
        modified_headers = {}
        for k, v in pairs(headers) do
            modified_headers[k] = v
        end
    end
    
    -- 根据响应状态修改缓存策略
    if status == 200 then
        modified_headers["Cache-Control"] = "public, max-age=3600"
    else
        modified_headers["Cache-Control"] = "no-cache, no-store, must-revalidate"
    end
    
    -- 添加ETag支持
    modified_headers["ETag"] = "W/\"" .. os.time() .. "\""
end

-- 示例3: 修改内容编码和类型
function modify_content_headers()
    if modified_headers == nil then
        modified_headers = {}
        for k, v in pairs(headers) do
            modified_headers[k] = v
        end
    end
    
    -- 确保Content-Type正确设置
    local content_type = modified_headers["Content-Type"] or ""
    if content_type == "" then
        modified_headers["Content-Type"] = "application/octet-stream"
    end
    
    -- 添加X-Content-Type-Options头
    modified_headers["X-Content-Type-Options"] = "nosniff"
end

-- 示例4: 拦截特定响应并返回自定义内容
function intercept_specific_response()
    if status == 404 and string.find(url, "/api/") then
        return {
            status = 200,
            body = '{"error":"Resource not found but intercepted by Lua Proxy","code":404}'
        }
    end
    return nil
end

-- 主响应处理函数
function process_response()
    print("Processing response: " .. url .. " (Status: " .. status .. ")")
    
    -- 执行各种响应处理函数
    modify_security_headers()
    modify_cache_control()
    modify_content_headers()
    
    -- 检查是否需要拦截响应
    local intercept_response = intercept_specific_response()
    if intercept_response then
        return intercept_response
    end
    
    -- 返回nil表示继续处理原始响应
    return nil
end