-- 核心为GoRequest函数和GoProxy，go中会调用这两个脚本函数对请求、代理进行重写或者设置
-- 前端开发服务器地址
local protalAddr = {'http', 'localhost:8088'}

-- 要拦截并处理的域名
local hostList = {
    '100.22.22.22',
}

-- 定义 API 列表和主机列表，不在apiList中的请求路径将会被重写指向protalAddr中的地址
local apiList = {
    '/api/v1',
    '/api/v2',
}

-- 默认代理
local proxy = 'http://urs:pwd@proxy.com:8080'

-- 定义一个映射表，用于存储主机名和对应的代理服务地址
local proxyMap = {
    ['www.baidu.com'] = 'http://urs:pwd@proxy.com:8080',
}

-- 需要直连的地址
local noProxyList = {
    'localhost',
    '10.*',
    '100.10*',
    '100.11*',
    '100.7*',
    '100.8*',
    '100.9*',
    '127.0.0.1*',
    '172.16.*',
}

-- 协议、主机、地址重写, 该函数会被代理程序调用，用于protocol, host, path这三个值的重写，如http,www.baidu.com,/search,修改为https,localhost:8080,/search/test
function GoRequest(protocol, host, path)
    -- 检查主机是否在主机列表中
    local function isHostInList(host, hostList)
        for _, h in ipairs(hostList) do
            if h == host then
                return true
            end
        end
        return false
    end

    -- 检查地址是否以 apiList 中的某个 API 开头
    local function isAddressStartsWithApi(path, apiList)
        for _, api in ipairs(apiList) do
            if string.sub(path, 1, #api) == api then
                return true
            end
        end
        return false
    end

    -- 主逻辑
    if isHostInList(host, hostList) then
        -- 主机在 hostList 中，检查地址
        if not isAddressStartsWithApi(path, apiList) then
            -- 地址不以 apiList 中的某个开头，修改主机为 localhost:8088
            host = protalAddr[2]
            protocol = protalAddr[1]
        end
    end

    return protocol, host, path
end

-- 代理服务器配置，会被代理程序调用，根据域名来返回代理配置，为""时表示直连
function GoProxy(host)
    -- 检查一个主机名是否匹配 NO_PROXY 列表中的某个模式
    local function matchNoProxy(host, noProxyList)
        for _, noProxyHost in ipairs(noProxyList) do
            -- 如果是通配符（以 '*' 开头），则进行匹配
            if string.find(noProxyHost, '*') then
                -- 转换成正则表达式，替换 '*' 为 '.*'
                local pattern = '^' .. string.gsub(noProxyHost, '%*', '.*') .. '$'
                if string.match(host, pattern) then
                    return true
                end
            else
                -- 如果没有通配符，直接比较
                if host == noProxyHost then
                    return true
                end
            end
        end
        return false
    end
    -- 如果主机名在 NO_PROXY 列表中，则不使用代理
    if matchNoProxy(host, noProxyList) then
        return ''
    end

    -- 如果映射表中存在该主机名，则返回对应的代理服务地址
    if proxyMap[host] then
        return proxyMap[host]
    else
        -- 如果不存在，则返回一个默认的代理地址
        return proxy
    end
end

-- 会被代理程序调用, html注入的元素文件路径
function GoInject(host)
    for _, h in ipairs(hostList) do
        if h == host then
            return "./inject.html"
        end
    end
    return ""
end