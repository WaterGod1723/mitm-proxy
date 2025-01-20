package main

import (
	"flag"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"

	"github.com/WaterGod1723/mitm-proxy/core"

	lua "github.com/yuin/gopher-lua"
)

// test
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

var (
	isAllowedCors = true
	proxyCache    sync.Map
)

func init() {
	luaScriptByte, err := os.ReadFile("config.lua")
	if err != nil {
		log.Fatal("read config error", err)
	}
	luaScript = string(luaScriptByte)
}

func main() {
	go checkConfChange()
	addr := flag.String("addr", "0.0.0.0:8003", "代理服务地址,0.0.0.0监听所有网卡,http协议")

	flag.Parse()
	c := core.NewMITM()
	mangeRouter(c)
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

	c.InsertHTMLToHTMLBody(func(resp *http.Response) string {
		host := strings.Split(resp.Request.Host, ":")[0]
		return LuaGetInjectHTML(host)
	})

	// c.ProcessResponse(func(resp *http.Response) core.ResponseWriteFunc {
	// 	if resp.StatusCode == http.StatusInternalServerError {
	// 		return func(w *core.ResponseWriter) error {
	// 			w.SetStatus(http.StatusInternalServerError)
	// 			w.Write([]byte("server error----from mitm-proxy"))
	// 			return nil
	// 		}
	// 	}
	// 	return nil
	// })

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

func LuaGetInjectHTML(host string) string {
	L := luaPool.Get().(*lua.LState)
	defer luaPool.Put(L)

	err := L.CallByParam(lua.P{
		Fn:      L.GetGlobal("GoInject"),
		NRet:    1,
		Protect: true,
	}, lua.LString(host))

	if err != nil {
		log.Println("Error:", err)
		return ""
	}
	result := L.Get(-1)
	L.Pop(1)
	p := string(result.(lua.LString))
	b, err := os.ReadFile(p)
	if err != nil {
		log.Println(err)
		return ""
	}
	return string(b)
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

func mangeRouter(c *core.Container) {
	c.HandleFunc("/", func(w *core.ResponseWriter, r *http.Request) {
		w.SetStatus(http.StatusOK)
		w.Write([]byte("welcome to mitm-proxy"))
	})

	c.HandleFunc("/", func(w *core.ResponseWriter, r *http.Request) {
		w.SetStatus(http.StatusOK)
		w.Write([]byte("welcome to mitm-proxy"))
	})

	c.HandleFunc("/open-cors", func(w *core.ResponseWriter, r *http.Request) {
		w.SetStatus(http.StatusOK)
		w.Write([]byte("true"))
		isAllowedCors = true
	})

	c.HandleFunc("/close-cors", func(w *core.ResponseWriter, r *http.Request) {
		w.SetStatus(http.StatusOK)
		w.Write([]byte("false"))
		isAllowedCors = false
	})

	c.HandleFunc("/can-cors", func(w *core.ResponseWriter, r *http.Request) {
		w.SetStatus(http.StatusOK)
		if isAllowedCors {
			w.Write([]byte("true"))
		} else {
			w.Write([]byte("false"))
		}
	})

}

// 检测文件是否更新的函数
func checkFileAndRestart(filePath string, lastModTime *time.Time) {
	for {
		// 获取文件的当前状态
		fileInfo, err := os.Stat(filePath)
		if err != nil {
			fmt.Printf("Error checking file: %v\n", err)
			time.Sleep(2 * time.Second)
			continue
		}

		// 如果文件的修改时间大于上次记录的时间，表示文件已更新
		if fileInfo.ModTime().After(*lastModTime) {
			fmt.Println("Config file updated. Restarting program...")

			// 更新最后修改时间
			*lastModTime = fileInfo.ModTime()

			// 调用重启程序的函数
			restartProgram()
		}

		// 每 2 秒检查一次
		time.Sleep(2 * time.Second)
	}
}

// 重启程序
func restartProgram() {
	execPath, err := os.Executable()
	if err != nil {
		log.Fatal("Error finding executable path:", err)
	}

	cmd := exec.Command(execPath, os.Args[1:]...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin

	if err := cmd.Start(); err != nil {
		log.Fatal("Failed to restart:", err)
	}

	os.Exit(0)
}

func checkConfChange() {
	// 配置文件路径
	filePath := "config.lua"

	// 初始化最后修改时间
	fileInfo, err := os.Stat(filePath)
	if err != nil {
		fmt.Printf("Error getting file info: %v\n", err)
		return
	}
	lastModTime := fileInfo.ModTime()

	// 开始检测文件更新
	checkFileAndRestart(filePath, &lastModTime)
}
