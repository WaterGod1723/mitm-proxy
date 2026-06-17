-- proxy.lua - 动态代理设置脚本
-- 可用全局变量：
--   url: string - 请求的完整URL
--   method: string - 请求方法（GET, POST等）

-- 示例1: 根据URL模式设置不同代理
function get_proxy_by_url_pattern()
    -- 内部网络请求直接连接（无代理）
    if string.find(url, "^http://internal%.") or string.find(url, "^https://internal%.") then
        return {
            protocol = "",
            address = "",
            username = "",
            password = ""
        }
    end
    
    -- API请求使用特定代理
    if string.find(url, "/api/") then
        return {
            protocol = "http",
            address = "proxy.example.com:8080",
            username = "api_user",
            password = "api_pass"
        }
    end
    
    -- 图片资源使用CDN代理
    if string.find(url, "%.jpg") or string.find(url, "%.png") or string.find(url, "%.gif") then
        return {
            protocol = "http",
            address = "cdn-proxy.example.com:3128",
            username = "",
            password = ""
        }
    end
    
    -- 默认代理
    return {
        protocol = "http",
        address = "default-proxy.example.com:8080",
        username = "default_user",
        password = "default_pass"
    }
end

-- 示例2: 基于时间的动态代理切换
function get_proxy_by_time()
    local hour = tonumber(os.date("%H"))
    
    -- 工作时间（9-18点）使用公司代理
    if hour >= 9 and hour < 18 then
        return {
            protocol = "http",
            address = "company-proxy.example.com:8080",
            username = "company_user",
            password = "company_pass"
        }
    else
        -- 非工作时间使用公共代理
        return {
            protocol = "http",
            address = "public-proxy.example.com:3128",
            username = "",
            password = ""
        }
    end
end

-- 示例3: 代理故障转移逻辑
function get_proxy_with_failover()
    -- 这里可以实现更复杂的代理健康检查逻辑
    -- 简化示例：随机选择一个代理
    local proxies = {
        {
            protocol = "http",
            address = "proxy1.example.com:8080",
            username = "user",
            password = "pass"
        },
        {
            protocol = "http",
            address = "proxy2.example.com:8080",
            username = "user",
            password = "pass"
        }
    }
    
    local index = math.random(1, #proxies)
    return proxies[index]
end

-- 主代理获取函数
function get_proxy()
    print("Determining proxy for: " .. url)
    
    -- 根据不同策略获取代理
    -- 这里可以根据需要选择不同的策略
    return ""
    -- return get_proxy_by_time()
    -- return get_proxy_with_failover()
end