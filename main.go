package main

import (
	"flag"
	"log"
	"mitm/core"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"

	lua "github.com/yuin/gopher-lua"
)

var luaScript string
var luaPool = sync.Pool{
	New: func() interface{} {
		L := lua.NewState()
		// 加载 Lua 脚本
		if err := L.DoString(luaScript); err != nil {
			log.Fatal("Error:", err)
		}
		return L
	},
}

var proxyCache sync.Map

func init() {
	luaScriptByte, err := os.ReadFile("config.lua")
	if err != nil {
		log.Fatal("read config error", err)
	}
	luaScript = string(luaScriptByte)
}

func main() {
	addr := flag.String("addr", "0.0.0.0:8003", "代理服务地址,0.0.0.0监听所有网卡,http协议")
	flag.Parse()
	c := core.NewMITM()
	c.ProcessRequest(func(req *http.Request) core.ResponseWriteFunc {
		arr := [...]string{
			req.URL.Scheme,
			req.Host,
			req.URL.Path,
		}
		LuaRewriteReq(&arr)
		req.URL.Scheme = arr[0]
		req.URL.Host = arr[1]
		req.Host = arr[1]
		req.URL.Path = arr[2]
		return nil
	})

	c.SetProxy(func(req *http.Request) core.ProxyArray {
		host := strings.Split(req.Host, ":")[0]
		return LuaGetProxy(host)
	})

	c.Start(*addr)
}

func LuaGetProxy(host string) core.ProxyArray {
	if value, ok := proxyCache.Load(host); ok {
		return value.(core.ProxyArray)
	}
	L := luaPool.Get().(*lua.LState)
	defer luaPool.Put(L)

	err := L.CallByParam(lua.P{
		Fn:      L.GetGlobal("GoProxy"),
		NRet:    1,
		Protect: true,
	}, lua.LString(host))

	if err != nil {
		log.Println("Error:", err)
		return [4]string{}
	}
	result := L.Get(-1)
	L.Pop(1)
	s := string(result.(lua.LString))
	u, err := url.Parse(s)
	if err != nil {
		return [4]string{}
	}

	pwd, _ := u.User.Password()
	p := [4]string{
		u.Scheme,
		u.Host,
		u.User.Username(),
		pwd,
	}
	proxyCache.Store(host, p)
	return p
}

func LuaRewriteReq(uri *[3]string) {
	L := luaPool.Get().(*lua.LState)
	defer luaPool.Put(L)
	err := L.CallByParam(lua.P{
		Fn:      L.GetGlobal("GoRequest"),
		NRet:    3,
		Protect: true,
	}, lua.LString(uri[0]), lua.LString(uri[1]), lua.LString(uri[2]))

	if err != nil {
		log.Println("Error:", err)
		return
	}

	uri[0] = L.ToString(-3)
	uri[1] = L.ToString(-2)
	uri[2] = L.ToString(-1)
	L.Pop(3)
}
