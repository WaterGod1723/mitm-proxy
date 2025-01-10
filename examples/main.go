package main

import (
	"compress/gzip"
	"compress/zlib"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"

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
var injectScript string
var isAllowedCors = true

var proxyCache sync.Map

func init() {
	luaScriptByte, err := os.ReadFile("config.lua")
	if err != nil {
		log.Fatal("read config error", err)
	}
	luaScript = string(luaScriptByte)

	injectScriptByte, err := os.ReadFile("inject.html")
	if err != nil {
		log.Fatal("read config error", err)
	}
	injectScript = string(injectScriptByte)
}

func main() {
	addr := flag.String("addr", "0.0.0.0:8003", "代理服务地址,0.0.0.0监听所有网卡,http协议")
	port := strings.Split(*addr, ":")[1]
	injectScript = strings.ReplaceAll(injectScript, "{{port}}", port)

	flag.Parse()
	c := core.NewMITM()
	c.ProcessRequest(func(req *http.Request) core.ResponseWriteFunc {
		// if req.Method == http.MethodOptions && isAllowedCors {
		// 	return handleCors()
		// }

		if req.Host == "localhost:"+port {
			return mangeRouter(req)
		}

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

	c.ProcessResponse(func(resp *http.Response) core.ResponseWriteFunc {
		// if isAllowedCors {
		// 	resp.Header.Set("Access-Control-Allow-Origin", "*")
		// }
		err := InsertStringToHTMLBody(resp, injectScript)
		if err != nil {
			return func(ww io.Writer) error {
				w := core.NewResponseWriter(ww)
				w.SetStatus(http.StatusInternalServerError)
				w.Write([]byte("proxy decompress html response body error"))
				return nil
			}
		}
		return nil
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

// InsertStringToHTMLBody 判断响应是否为 HTML，如果是，则在 </body> 前插入指定字符串
func InsertStringToHTMLBody(resp *http.Response, insertString string) error {
	// 检查 Content-Type 是否为 HTML
	contentType := resp.Header.Get("Content-Type")
	if !strings.Contains(contentType, "text/html") {
		return nil // 不是 HTML，直接返回原响应
	}

	// 在 </body> 标签前插入指定字符串
	bodyStr, err := DecompressBody(resp)
	if err != nil {
		return err
	}
	bodyTagIndex := strings.LastIndex(bodyStr, "</body>")
	if bodyTagIndex == -1 {
		// 构建新的响应体
		newBody := io.NopCloser(strings.NewReader(bodyStr))
		resp.Body = newBody
		// 如果没有找到 </body> 标签，直接返回原响应
		return nil
	}

	// 构建新的响应体
	newBodyStr := bodyStr[:bodyTagIndex] + insertString + bodyStr[bodyTagIndex:]
	newBody := io.NopCloser(strings.NewReader(newBodyStr))

	// 更新响应体
	resp.Body = newBody
	resp.ContentLength = int64(len(newBodyStr))
	resp.Header.Set("Content-Length", fmt.Sprintf("%d", len(newBodyStr)))

	return nil
}

func handleCors() core.ResponseWriteFunc {
	return func(ww io.Writer) error {
		w := core.NewResponseWriter(ww)
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
		w.Header().Set("Access-Control-Max-Age", "86400")
		w.Header().Set("Access-Control-Allow-Credentials", "true")
		w.SetStatus(http.StatusNoContent)
		w.Write([]byte{})
		return nil
	}
}

func mangeRouter(req *http.Request) core.ResponseWriteFunc {
	if req.URL.Path == "/open-cors" {
		isAllowedCors = true
	} else if req.URL.Path == "/close-cors" {
		isAllowedCors = false
	} else if req.URL.Path == "/can-cors" {
		return func(ww io.Writer) error {
			w := core.NewResponseWriter(ww)
			w.Header().Set("Access-Control-Allow-Origin", "*")
			w.SetStatus(http.StatusOK)
			if isAllowedCors {
				w.Write([]byte("true"))
			} else {
				w.Write([]byte("false"))
			}
			return nil
		}
	}
	return func(ww io.Writer) error {
		w := core.NewResponseWriter(ww)
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.SetStatus(http.StatusOK)
		w.Write([]byte("ok"))
		return nil
	}
}

func DecompressBody(resp *http.Response) (string, error) {
	var reader io.ReadCloser
	var err error

	// 根据 Content-Encoding 头选择解压缩方式
	encoding := resp.Header.Get("Content-Encoding")
	switch strings.ToLower(encoding) {
	case "gzip":
		reader, err = gzip.NewReader(resp.Body)
		if err != nil {
			return "", fmt.Errorf("failed to create gzip reader: %v", err)
		}
		defer reader.Close()
	case "deflate":
		reader, err = zlib.NewReader(resp.Body)
		if err != nil {
			return "", fmt.Errorf("failed to create zlib reader: %v", err)
		}
		defer reader.Close()
	default:
		reader = resp.Body // 未压缩，直接使用原响应体
	}

	// 读取解压缩后的数据
	bodyBytes, err := io.ReadAll(reader)
	if err != nil {
		return "", fmt.Errorf("failed to read response body: %v", err)
	}

	// 删除 Content-Encoding 头
	resp.Header.Del("Content-Encoding")

	return string(bodyBytes), nil
}
