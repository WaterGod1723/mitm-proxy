-- 核心为GoRequest函数和GoProxy，go中会调用这两个脚本函数对请求、代理进行重写或者设置

-- 协议、主机、地址重写, 该函数会被代理程序调用，用于protocol, host, path这三个值的重写，如http,www.baidu.com,/search,修改为https,localhost:8080,/search/test
function GoRequest(protocol, host, path)
    local bodyFilePath = ""
    -- 重写chunk-common.js，已实现线上前端资源请求本地的前端资源
    if string.find(host, "amh%-group%.com$", 1, false) and string.find(path, "chunk%-common%..+%.js$", 1, false) then
        bodyFilePath = "./overrides/chunk-common.js"
    end

    if string.find(host, "dev-luban%.amh%-group%.com$", 1, false) then
        if string.find(path, "^/microweb-pc", 1, false) then
            -- 如果满足条件，则修改 host
            host = "web.amh-group.com:8000"
        end
    elseif string.find(host, "knowsearch%.amh%-group%.com$", 1, false) then
        -- 检查路径是否不以 'api' 开头
        if not string.find(path, "^/api", 1, false) then
            -- 如果满足条件，则修改 host
            host = "localhost:8089"
        end
    end

    return protocol, host, path, bodyFilePath
end

-- 代理服务器配置，会被代理程序调用，可以根据host域名来返回不同的代理配置如（proxy.huawei.com:8080)），为""时表示直连
function GoProxy(host)
    return ''
end

-- 会被代理程序调用, html注入的元素文件路径
function GoInject(host)
    -- for _, h in ipairs(hostList) do
    --     if h == host then
    --         return "./inject.html"
    --     end
    -- end
    return ""
end
