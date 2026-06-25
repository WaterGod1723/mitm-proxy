package internal

import (
	"fmt"
	"log"
	"net/http"
	"sync"

	lua "github.com/yuin/gopher-lua"
)

// Pool 维护一组可复用的 Lua 状态，用于并发执行配置脚本中的回调函数。
type Pool struct {
	config  *Config
	maxSize int
	mu      sync.Mutex
	states  []*lua.LState
}

// NewPool 创建 Lua 状态池，maxSize 控制池中最大缓存数量。
func NewPool(cfg *Config, maxSize int) *Pool {
	if maxSize <= 0 {
		maxSize = 8
	}
	return &Pool{config: cfg, maxSize: maxSize}
}

// Get 从池中获取一个已加载当前脚本的 Lua 状态；若池为空则新建。
func (p *Pool) Get() (*lua.LState, error) {
	p.mu.Lock()
	script := p.config.Script()
	if n := len(p.states); n > 0 {
		L := p.states[n-1]
		p.states = p.states[:n-1]
		p.mu.Unlock()
		return L, nil
	}
	p.mu.Unlock()

	L := lua.NewState()
	if err := L.DoString(script); err != nil {
		L.Close()
		return nil, fmt.Errorf("load lua script: %w", err)
	}
	return L, nil
}

// Put 将 Lua 状态归还池中；超过容量则关闭。
func (p *Pool) Put(L *lua.LState) {
	if L == nil {
		return
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if len(p.states) < p.maxSize {
		p.states = append(p.states, L)
		return
	}
	L.Close()
}

// Recreate 在配置变更后清空旧状态池，使后续 Get 创建的新状态使用新脚本。
func (p *Pool) Recreate() {
	p.mu.Lock()
	old := p.states
	p.states = p.states[:0]
	p.mu.Unlock()

	for _, L := range old {
		L.Close()
	}
}

// GoHeadersToLua 将 Go 的 http.Header 转为 Lua 表。
// 单值 header 转为字符串，多值 header 转为字符串数组。
func GoHeadersToLua(L *lua.LState, h http.Header) *lua.LTable {
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

// LuaTableToGoHeaders 将 Lua 表转回 http.Header。
// 字符串值 -> 单元素切片；表值 -> 多元素切片；nil/false -> 删除该 header。
func LuaTableToGoHeaders(tbl *lua.LTable) http.Header {
	h := http.Header{}
	tbl.ForEach(func(k, v lua.LValue) {
		keyStr, ok := k.(lua.LString)
		if !ok {
			return
		}
		key := string(keyStr)
		switch v.Type() {
		case lua.LTNil, lua.LTBool:
			if v.Type() == lua.LTBool && lua.LVAsBool(v) {
				return
			}
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

// CallLuaString 调用返回 string 的 Lua 全局函数。
func CallLuaString(L *lua.LState, name string, args ...lua.LValue) (string, error) {
	fn := L.GetGlobal(name)
	if fn == lua.LNil {
		return "", fmt.Errorf("lua function %q not found", name)
	}
	if err := L.CallByParam(lua.P{Fn: fn, NRet: 1, Protect: true}, args...); err != nil {
		return "", err
	}
	result := L.Get(-1)
	L.Pop(1)
	if s, ok := result.(lua.LString); ok {
		return string(s), nil
	}
	return "", nil
}

// SafePut 是 Put 的安全包装，带日志。
func SafePut(pool *Pool, L *lua.LState, logger *log.Logger) {
	if L == nil {
		return
	}
	pool.Put(L)
}
