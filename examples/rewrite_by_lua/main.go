package main

import (
	"bytes"
	"flag"
	"fmt"
	"log"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"text/template"
	"time"

	"github.com/WaterGod1723/mitm-proxy/core"

	lua "github.com/yuin/gopher-lua"
)

// test
var luaPool *sync.Pool
var (
	isAllowedCors = true
	proxyCache    sync.Map
)

func main() {
	configName := flag.String("config", "default", "配置文件名（不含.lua后缀）或完整路径；默认为 configs/default.lua")
	addr := flag.String("addr", "0.0.0.0:8003", "代理服务地址,0.0.0.0监听所有网卡,http协议")
	flag.Parse()

	configPath := resolveConfigPath(*configName)
	luaScriptByte, err := os.ReadFile(configPath)
	if err != nil {
		log.Fatalf("read config error: %v (path: %s)", err, configPath)
	}
	luaScript := string(luaScriptByte)

	luaPool = &sync.Pool{
		New: func() interface{} {
			L := lua.NewState()
			// 加载 Lua 脚本
			if err := L.DoString(luaScript); err != nil {
				log.Fatal("Error:", err)
			}
			return L
		},
	}

	go checkConfChange(configPath)
	c := core.NewMITM()
	mangeRouter(c)
	c.ProcessRequest(func(req *http.Request) core.ResponseWriteFunc {
		protocol, host, path, bodyFilePath, newHeaders, err := LuaRewriteReq(req)
		if err != nil {
			log.Println("LuaRewriteReq error:", err)
			return nil
		}
		// 直接写入响应
		if bodyFilePath != "" {
			f, err := os.ReadFile(bodyFilePath)
			if err != nil {
				log.Println("open file error: ", err)
				return nil
			}
			return func(w *core.ResponseWriter) error {
				w.SetStatus(http.StatusOK)
				for key, values := range newHeaders {
					for _, value := range values {
						w.Header().Set(key, value)
					}
				}
				w.Write(f)
				return nil
			}
		}
		req.URL.Scheme = protocol
		req.URL.Host = host
		req.Host = host
		req.URL.Path = path
		if newHeaders != nil {
			req.Header = newHeaders
		}
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
	localIP := getLocalIP()
	listenAddr := *addr
	fmt.Printf("mitm-proxy started at http://%s (listen on %s)\n", strings.Replace(listenAddr, "0.0.0.0", localIP, 1), listenAddr)

	_, port, _ := net.SplitHostPort(listenAddr)
	if port == "" {
		port = "8003"
	}
	cwd, _ := os.Getwd()
	scriptPath := filepath.Join(cwd, "start_chrome.sh")
	if err := generateChromeScript(localIP, port, scriptPath); err != nil {
		log.Println("generate start_chrome.sh error:", err)
	} else {
		fmt.Printf("Chrome launcher script generated: %s\n", scriptPath)
	}

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		c.Start(*addr)
	}()

	// 监听系统退出信号
	ch := make(chan os.Signal, 1)
	signal.Notify(ch, os.Interrupt, syscall.SIGTERM)
	<-ch

	c.Stop()
	wg.Wait()
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

// resolveConfigPath 解析配置文件路径：
//   - 空字符串 -> configs/default.lua
//   - 绝对路径或含路径分隔符 -> 直接使用（必要时补 .lua 后缀）
//   - 其他 -> 在 configs/ 目录下查找对应的 .lua 文件
func resolveConfigPath(name string) string {
	if name == "" {
		return "configs/default.lua"
	}
	if filepath.IsAbs(name) || strings.ContainsAny(name, "/\\") {
		if !strings.HasSuffix(name, ".lua") {
			name += ".lua"
		}
		return name
	}
	name = strings.TrimSuffix(name, ".lua")
	return filepath.Join("configs", name+".lua")
}

// goHeadersToLua 将 Go 的 http.Header 转为 Lua 表。
// 长度为 1 的切片转为字符串，多个值转为字符串数组。
func goHeadersToLua(L *lua.LState, h http.Header) *lua.LTable {
	tbl := L.NewTable()
	for k, v := range h {
		if len(v) == 1 {
			tbl.RawSetString(k, lua.LString(v[0]))
			continue
		}
		arr := L.NewTable()
		for i, s := range v {
			arr.RawSetInt(i+1, lua.LString(s))
		}
		tbl.RawSetString(k, arr)
	}
	return tbl
}

// luaTableToGoHeaders 将 Lua 表转回 http.Header。
// 字符串值 -> 单元素切片；表值 -> 多元素切片；nil/false -> 删除该 header。
func luaTableToGoHeaders(tbl *lua.LTable) http.Header {
	h := http.Header{}
	tbl.ForEach(func(k, v lua.LValue) {
		keyStr, ok := k.(lua.LString)
		if !ok {
			return
		}
		key := string(keyStr)
		switch v.Type() {
		case lua.LTNil:
			h.Del(key)
		case lua.LTString:
			h.Set(key, string(v.(lua.LString)))
		case lua.LTTable:
			var values []string
			v.(*lua.LTable).ForEach(func(_ik, iv lua.LValue) {
				if s, ok := iv.(lua.LString); ok {
					values = append(values, string(s))
				}
			})
			if len(values) > 0 {
				h[http.CanonicalHeaderKey(key)] = values
			} else {
				h.Del(key)
			}
		}
	})
	return h
}

func LuaRewriteReq(req *http.Request) (protocol, host, path, bodyFilePath string, newHeaders http.Header, err error) {
	L := luaPool.Get().(*lua.LState)
	defer luaPool.Put(L)

	headersTbl := goHeadersToLua(L, req.Header)

	err = L.CallByParam(lua.P{
		Fn:      L.GetGlobal("GoRequest"),
		NRet:    5,
		Protect: true,
	}, lua.LString(req.URL.Scheme), lua.LString(req.Host), lua.LString(req.URL.Path), headersTbl)

	if err != nil {
		log.Println("Error:", err)
		return
	}

	protocol = L.ToString(-5)
	host = L.ToString(-4)
	path = L.ToString(-3)
	bodyFilePath = L.ToString(-2)

	// 第五个返回值为 headers 表：nil 或空表表示保持原请求头不变
	if tbl, ok := L.Get(-1).(*lua.LTable); ok {
		newHeaders = luaTableToGoHeaders(tbl)
		if len(newHeaders) == 0 {
			newHeaders = nil
		}
	}
	L.Pop(5)
	return
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

func checkConfChange(filePath string) {
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

// getLocalIP 获取本机第一个非回环 IPv4 地址
func getLocalIP() string {
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		return "127.0.0.1"
	}
	for _, addr := range addrs {
		if ipNet, ok := addr.(*net.IPNet); ok && !ipNet.IP.IsLoopback() {
			if ipNet.IP.To4() != nil {
				return ipNet.IP.String()
			}
		}
	}
	return "127.0.0.1"
}

const chromeScriptTmpl = `#!/bin/bash

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
USER_DATA_DIR="$SCRIPT_DIR/chrome_dev_profile"
PROXY_ADDR="http://{{.Host}}:{{.Port}}"

echo "Starting Chrome with proxy: $PROXY_ADDR"
echo "User data dir: $USER_DATA_DIR"

mkdir -p "$USER_DATA_DIR"

OS="$(uname -s)"
case "$OS" in
	MINGW*|MSYS*|CYGWIN*)
		CHROME_PATH="/c/Program Files/Google/Chrome/Application/chrome.exe"
		if [ ! -f "$CHROME_PATH" ]; then
			CHROME_PATH="/c/Program Files (x86)/Google/Chrome/Application/chrome.exe"
		fi
		;;
	Darwin)
		CHROME_PATH="/Applications/Google Chrome.app/Contents/MacOS/Google Chrome"
		;;
	*)
		CHROME_PATH="/Applications/Google Chrome.app/Contents/MacOS/Google Chrome"
		;;
esac

"$CHROME_PATH" \
  --user-data-dir="$USER_DATA_DIR" \
  --proxy-server="$PROXY_ADDR" \
  --ignore-certificate-errors \
  --no-first-run \
  --no-default-browser-check \
  --disable-web-security \
  --disable-features=SameSiteByDefaultCookies \
  --disable-site-isolation-trials \
  "$@"
`

type chromeScriptData struct {
	Host string
	Port string
}

func generateChromeScript(host, port, outPath string) error {
	tmpl, err := template.New("chrome").Parse(chromeScriptTmpl)
	if err != nil {
		return err
	}
	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, chromeScriptData{Host: host, Port: port}); err != nil {
		return err
	}
	if err := os.WriteFile(outPath, buf.Bytes(), 0755); err != nil {
		return err
	}
	return nil
}
