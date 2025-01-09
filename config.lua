-- 协议、主机、地址重写, 该函数会被代理程序调用，用于protocol, host, path这三个值的重写,如将http,www.baidu.com,/search，可以重写为https,localhost:8080,/hhh
function GoRequest(protocol, host, path)
  return protocol, host, path
end

-- 代理服务器配置，会被代理程序调用，根据域名来返回代理配置，为""时表示直连
function GoProxy(host)
  return "http://usr:pwd@proxy.com:8080"
end
