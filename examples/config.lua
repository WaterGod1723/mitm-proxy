-- 核心为GoRequest函数和GoProxy，go中会调用这两个脚本函数对请求、代理进行重写或者设置

--- GoRequest 函数用于重写HTTP/HTTPS请求的协议、主机和路径
-- @param protocol string 请求协议，如 "http" 或 "https"
-- @param host string 请求主机，包括域名和端口（如果有），如 "www.baidu.com" 或 "web.amh-group.com:8000"
-- @param path string 请求路径，如 "/search" 或 "/microweb-pc/home"
-- @return protocol string 重写后的协议
-- @return host string 重写后的主机
-- @return path string 重写后的路径
-- @return bodyFilePath string (可选) 用于替换响应体的本地文件路径，不指定则正常转发请求
-- @description 该函数会在每次请求到达代理服务器时被调用，允许根据请求的协议、主机和路径进行灵活的重写
--              可以用于实现环境切换、路径重定向、响应mock等功能
--              当返回bodyFilePath时，代理会直接返回该文件的内容作为响应，而不转发请求到原始服务器
function GoRequest(protocol, host, path)
    -- 示例：根据特定条件修改请求主机
    -- 如果请求主机是 "web.amh-group.com:8000" 并且路径以 "/smart-truck" 或 "/microweb-pc/" 开头
    -- 则将主机修改为 "web.amh-group.com:8001"
    if host == "web.amh-group.com:8000" and (string.find(path, "^/smart%-truck", 1, false)  or string.find(path, "^/microweb%-pc/", 1, false) ) then
        host = "web.amh-group.com:8001"
        print("change host")
    end

    -- 返回重写后的协议、主机和路径
    -- 注意：如果需要替换请求体，可以返回第四个参数bodyFilePath，指向本地文件
    return protocol, host, path
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
    return ""
end
