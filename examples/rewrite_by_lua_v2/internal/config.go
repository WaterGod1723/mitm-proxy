package internal

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// Config 封装 Lua 配置脚本的加载与热重载。
type Config struct {
	path     string
	script   string
	modTime  time.Time
	mu       sync.RWMutex
	onChange func(script string)
}

// LoadConfig 从指定路径加载 Lua 配置脚本。
func LoadConfig(path string) (*Config, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return nil, fmt.Errorf("resolve config path: %w", err)
	}

	c := &Config{path: abs}
	if err := c.Reload(); err != nil {
		return nil, err
	}
	return c, nil
}

// ResolveConfigPath 解析用户通过 -config 传入的配置名称或路径。
func ResolveConfigPath(name string) string {
	if name == "" {
		return "configs/default.lua"
	}
	if filepath.IsAbs(name) || strings.ContainsAny(name, `/\`) {
		if !strings.HasSuffix(name, ".lua") {
			name += ".lua"
		}
		return name
	}
	name = strings.TrimSuffix(name, ".lua")
	return filepath.Join("configs", name+".lua")
}

// Script 返回当前生效的 Lua 脚本内容。
func (c *Config) Script() string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.script
}

// Path 返回配置文件的绝对路径。
func (c *Config) Path() string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.path
}

// SetOnChange 设置配置变更回调；每次热重载成功后会调用。
func (c *Config) SetOnChange(fn func(script string)) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.onChange = fn
}

// Reload 立即重新读取配置文件。
func (c *Config) Reload() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	info, err := os.Stat(c.path)
	if err != nil {
		return fmt.Errorf("stat config %q: %w", c.path, err)
	}

	data, err := os.ReadFile(c.path)
	if err != nil {
		return fmt.Errorf("read config %q: %w", c.path, err)
	}

	c.script = string(data)
	c.modTime = info.ModTime()

	if c.onChange != nil {
		c.onChange(c.script)
	}
	return nil
}

// ModTime 返回配置文件的最后修改时间。
func (c *Config) ModTime() time.Time {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.modTime
}

// Watch 以轮询方式监听配置文件变更，变更时调用 Reload。
// 调用方应通过 ctx 或 stop 通道控制退出。
func (c *Config) Watch(interval time.Duration, stop <-chan struct{}) {
	if interval <= 0 {
		interval = 2 * time.Second
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-stop:
			return
		case <-ticker.C:
			info, err := os.Stat(c.Path())
			if err != nil {
				continue
			}
			if info.ModTime().After(c.ModTime()) {
				if err := c.Reload(); err != nil {
					// 仅记录错误，不中断监听
					continue
				}
			}
		}
	}
}
