package main

import (
	"fmt"
	"net/http"
	"os"
	"sync"
	"time"

	"github.com/WaterGod1723/mitm-proxy/core"
	lua "github.com/yuin/gopher-lua"
)

// scriptCacheItem 脚本缓存项
type scriptCacheItem struct {
	content    []byte
	modTime    time.Time
	compileErr error
}

// LuaMITM 集成Lua脚本的MITM代理
type LuaMITM struct {
	mitm        *core.Container
	config      *luaConfig
	scriptCache map[string]*scriptCacheItem
	luaPool     chan *lua.LState
	poolMutex   sync.Mutex
	poolSize    int
	maxPoolSize int
}

// luaConfig Lua配置结构
type luaConfig struct {
	port             string
	requestScript    string
	responseScript   string
	htmlInjectScript string
	proxyScript      string
}

// NewLuaMITM 创建新的LuaMITM实例
func NewLuaMITM() *LuaMITM {
	config := &luaConfig{
		port:             "8080",
		requestScript:    "request.lua",
		responseScript:   "response.lua",
		htmlInjectScript: "html_inject.lua",
		proxyScript:      "proxy.lua",
	}

	mitm := core.NewMITM()

	// 设置合理的最大对象池大小，根据预期的并发请求数调整
	maxPoolSize := 100

	return &LuaMITM{
		mitm:        mitm,
		config:      config,
		scriptCache: make(map[string]*scriptCacheItem),
		luaPool:     make(chan *lua.LState, maxPoolSize),
		maxPoolSize: maxPoolSize,
	}
}

// getLuaVM 从对象池获取Lua状态机
func (lm *LuaMITM) getLuaVM() *lua.LState {
	// 尝试从通道获取
	select {
	case vm := <-lm.luaPool:
		return vm
	default:
		// 通道为空，检查是否可以创建新的
		lm.poolMutex.Lock()
		if lm.poolSize < lm.maxPoolSize {
			lm.poolSize++
			lm.poolMutex.Unlock()
			return lua.NewState()
		}
		lm.poolMutex.Unlock()
		// 超出池大小限制，直接创建
		return lua.NewState()
	}
}

// putLuaVM 将Lua状态机放回对象池
func (lm *LuaMITM) putLuaVM(vm *lua.LState) {
	if vm == nil {
		return
	}

	// 重置状态机
	vm.SetGlobal("url", lua.LNil)
	vm.SetGlobal("method", lua.LNil)
	vm.SetGlobal("headers", lua.LNil)
	vm.SetGlobal("body", lua.LNil)
	vm.SetGlobal("has_body", lua.LNil)
	vm.SetGlobal("status", lua.LNil)
	vm.SetGlobal("modified_headers", lua.LNil)
	vm.SetGlobal("protocol", lua.LNil)
	vm.SetGlobal("host", lua.LNil)
	vm.SetGlobal("path", lua.LNil)

	// 尝试放回通道
	select {
	case lm.luaPool <- vm:
		return
	default:
		// 通道已满，关闭状态机
		vm.Close()
		lm.poolMutex.Lock()
		lm.poolSize--
		lm.poolMutex.Unlock()
	}
}

// loadScript 加载Lua脚本（带缓存机制和并发安全）
func (lm *LuaMITM) loadScript(vm *lua.LState, filename string) error {
	if filename == "" {
		return nil
	}

	// 检查文件是否存在
	info, err := os.Stat(filename)
	if os.IsNotExist(err) {
		return nil // 脚本不存在，跳过
	}
	if err != nil {
		return fmt.Errorf("failed to stat script %s: %w", filename, err)
	}

	// 检查缓存
	cacheItem, exists := lm.scriptCache[filename]
	if exists && info.ModTime().Equal(cacheItem.modTime) {
		// 缓存有效，直接执行
		if cacheItem.compileErr != nil {
			return cacheItem.compileErr
		}
		if err := vm.DoString(string(cacheItem.content)); err != nil {
			return fmt.Errorf("failed to execute cached script %s: %w", filename, err)
		}
		return nil
	}

	// 缓存无效或不存在，重新读取和缓存
	content, err := os.ReadFile(filename)
	if err != nil {
		return fmt.Errorf("failed to read script %s: %w", filename, err)
	}

	// 先测试执行脚本
	if err := vm.DoString(string(content)); err != nil {
		compileErr := fmt.Errorf("failed to execute script %s: %w", filename, err)
		// 缓存编译错误
		lm.scriptCache[filename] = &scriptCacheItem{
			content:    content,
			modTime:    info.ModTime(),
			compileErr: compileErr,
		}
		return compileErr
	}

	// 缓存有效脚本
	lm.scriptCache[filename] = &scriptCacheItem{
		content:    content,
		modTime:    info.ModTime(),
		compileErr: nil,
	}

	return nil
}

// setupRequestProcessing 设置请求处理
func (lm *LuaMITM) setupRequestProcessing() {
	lm.mitm.ProcessRequest(func(req *http.Request) core.ResponseWriteFunc {
		// 从对象池获取Lua状态
		vm := lm.getLuaVM()
		defer lm.putLuaVM(vm)

		// 加载请求处理脚本
		if err := lm.loadScript(vm, lm.config.requestScript); err != nil {
			fmt.Printf("Error loading request script: %v\n", err)
			return nil
		}

		// 将请求信息传递给Lua
		vm.SetGlobal("url", lua.LString(req.URL.String()))
		vm.SetGlobal("method", lua.LString(req.Method))
		vm.SetGlobal("protocol", lua.LString(req.URL.Scheme))
		vm.SetGlobal("host", lua.LString(req.URL.Host))
		vm.SetGlobal("path", lua.LString(req.URL.Path))

		// 设置请求头表
		headers := lua.LTable{}
		for key, values := range req.Header {
			for _, value := range values {
				headers.RawSet(lua.LString(key), lua.LString(value))
			}
		}
		vm.SetGlobal("headers", &headers)

		// 不直接读取请求体，仅在Lua脚本明确需要时才读取
		vm.SetGlobal("body", lua.LString(""))
		vm.SetGlobal("has_body", lua.LBool(req.Body != nil && req.ContentLength > 0))

		// 调用Lua处理函数
		if err := vm.CallByParam(lua.P{Fn: vm.GetGlobal("process_request"), NRet: 1}); err != nil {
			fmt.Printf("Error calling process_request: %v\n", err)
			return nil
		}

		// 获取返回结果
		result := vm.Get(-1)
		vm.Pop(1)

		// 如果返回了新的响应函数，则执行
		if tbl, ok := result.(*lua.LTable); ok {
			status, ok1 := tbl.RawGetString("status").(lua.LNumber)
			body, ok2 := tbl.RawGetString("body").(lua.LString)
			if ok1 && ok2 {
				return func(w *core.ResponseWriter) error {
					w.SetStatus(int(status))
					w.Write([]byte(body))
					return nil
				}
			}
		}

		// 应用请求头修改
		if modifiedHeaders, ok := vm.GetGlobal("modified_headers").(*lua.LTable); ok {
			// 清空原请求头
			req.Header = make(http.Header)
			// 添加修改后的请求头
			modifiedHeaders.ForEach(func(k lua.LValue, v lua.LValue) {
				key := k.String()
				value := v.String()
				req.Header.Add(key, value)
			})
		}

		return nil
	})
}

// setupResponseProcessing 设置响应处理
func (lm *LuaMITM) setupResponseProcessing() {
	lm.mitm.ProcessResponse(func(resp *http.Response) core.ResponseWriteFunc {
		// 从对象池获取Lua状态
		vm := lm.getLuaVM()
		defer lm.putLuaVM(vm)

		// 加载响应处理脚本
		if err := lm.loadScript(vm, lm.config.responseScript); err != nil {
			fmt.Printf("Error loading response script: %v\n", err)
			return nil
		}

		// 将响应信息传递给Lua
		vm.SetGlobal("status", lua.LNumber(resp.StatusCode))
		vm.SetGlobal("url", lua.LString(resp.Request.URL.String()))
		vm.SetGlobal("protocol", lua.LString(resp.Request.URL.Scheme))
		vm.SetGlobal("host", lua.LString(resp.Request.URL.Host))
		vm.SetGlobal("path", lua.LString(resp.Request.URL.Path))

		// 设置响应头表
		headers := lua.LTable{}
		for key, values := range resp.Header {
			for _, value := range values {
				headers.RawSet(lua.LString(key), lua.LString(value))
			}
		}
		vm.SetGlobal("headers", &headers)

		// 不直接读取响应体，仅在Lua脚本明确需要时才读取
		vm.SetGlobal("body", lua.LString(""))
		vm.SetGlobal("has_body", lua.LBool(resp.Body != nil && resp.ContentLength > 0))
		vm.SetGlobal("content_length", lua.LNumber(resp.ContentLength))

		// 调用Lua处理函数
		if err := vm.CallByParam(lua.P{Fn: vm.GetGlobal("process_response"), NRet: 1}); err != nil {
			fmt.Printf("Error calling process_response: %v\n", err)
			return nil
		}

		// 获取返回结果
		result := vm.Get(-1)
		vm.Pop(1)

		// 如果返回了新的响应函数，则执行
		if tbl, ok := result.(*lua.LTable); ok {
			status, ok1 := tbl.RawGetString("status").(lua.LNumber)
			body, ok2 := tbl.RawGetString("body").(lua.LString)
			if ok1 && ok2 {
				return func(w *core.ResponseWriter) error {
					w.SetStatus(int(status))
					w.Write([]byte(body))
					return nil
				}
			}
		}

		// 应用响应头修改
		if modifiedHeaders, ok := vm.GetGlobal("modified_headers").(*lua.LTable); ok {
			// 清空原响应头
			resp.Header = make(http.Header)
			// 添加修改后的响应头
			modifiedHeaders.ForEach(func(k lua.LValue, v lua.LValue) {
				key := k.String()
				value := v.String()
				resp.Header.Add(key, value)
			})
		}

		return nil
	})
}

// setupHTMLInjection 设置HTML注入
func (lm *LuaMITM) setupHTMLInjection() {
	lm.mitm.InsertHTMLToHTMLBody(func(resp *http.Response) string {
		// 从对象池获取Lua状态
		vm := lm.getLuaVM()
		defer lm.putLuaVM(vm)

		// 加载HTML注入脚本
		if err := lm.loadScript(vm, lm.config.htmlInjectScript); err != nil {
			fmt.Printf("Error loading HTML inject script: %v\n", err)
			return ""
		}

		// 将响应信息传递给Lua
		vm.SetGlobal("url", lua.LString(resp.Request.URL.String()))
		vm.SetGlobal("status", lua.LNumber(resp.StatusCode))
		vm.SetGlobal("protocol", lua.LString(resp.Request.URL.Scheme))
		vm.SetGlobal("host", lua.LString(resp.Request.URL.Host))
		vm.SetGlobal("path", lua.LString(resp.Request.URL.Path))

		// 调用Lua处理函数
		if err := vm.CallByParam(lua.P{Fn: vm.GetGlobal("inject_html"), NRet: 1}); err != nil {
			fmt.Printf("Error calling inject_html: %v\n", err)
			return ""
		}

		// 获取返回结果
		result := vm.Get(-1)
		vm.Pop(1)

		if html, ok := result.(lua.LString); ok {
			return string(html)
		}

		return ""
	})
}

// setupDynamicProxy 设置动态代理
func (lm *LuaMITM) setupDynamicProxy() {
	lm.mitm.SetProxy(func(req *http.Request) core.ProxyArray {
		// 从对象池获取Lua状态
		vm := lm.getLuaVM()
		defer lm.putLuaVM(vm)

		// 加载代理设置脚本
		if err := lm.loadScript(vm, lm.config.proxyScript); err != nil {
			fmt.Printf("Error loading proxy script: %v\n", err)
			return core.ProxyArray{}
		}

		// 将请求信息传递给Lua
		vm.SetGlobal("url", lua.LString(req.URL.String()))
		vm.SetGlobal("method", lua.LString(req.Method))

		// 调用Lua处理函数
		if err := vm.CallByParam(lua.P{Fn: vm.GetGlobal("get_proxy"), NRet: 1}); err != nil {
			fmt.Printf("Error calling get_proxy: %v\n", err)
			return core.ProxyArray{}
		}

		// 获取返回结果
		result := vm.Get(-1)
		vm.Pop(1)

		if tbl, ok := result.(*lua.LTable); ok {
			protocol, _ := tbl.RawGetString("protocol").(lua.LString)
			address, _ := tbl.RawGetString("address").(lua.LString)
			username, _ := tbl.RawGetString("username").(lua.LString)
			password, _ := tbl.RawGetString("password").(lua.LString)

			return core.ProxyArray{
				string(protocol),
				string(address),
				string(username),
				string(password),
			}
		}

		return core.ProxyArray{}
	})
}

// Start 启动代理服务
func (lm *LuaMITM) Start() {
	// 设置各种Lua脚本处理
	lm.setupRequestProcessing()
	lm.setupResponseProcessing()
	lm.setupHTMLInjection()
	lm.setupDynamicProxy()

	// 启动MITM代理
	fmt.Printf("Lua MITM Proxy started on port %s\n", lm.config.port)
	lm.mitm.Start(":" + lm.config.port)
}

func main() {
	// 创建Lua MITM代理
	proxy := NewLuaMITM()

	// 启动代理服务
	proxy.Start()
}
