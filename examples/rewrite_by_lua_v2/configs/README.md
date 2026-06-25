# 多配置文件使用说明

## 快速开始

```bash
# 使用默认配置 (configs/default.lua)
./main

# 使用 custom 配置
./main -config custom

# 指定监听端口和配置
./main -addr :9000 -config custom

# 使用完整路径
./main -config /path/to/config.lua
```

## 配置文件位置

所有配置文件统一放在 `configs/` 目录下：

- 不指定 `-config` 参数时，默认使用 `configs/default.lua`
- 指定 `-config custom` 时，查找 `configs/custom` 或 `configs/custom.lua`
- 指定完整路径时，直接使用该路径

## 创建新配置

1. 在 `configs/` 目录下创建新的 `.lua` 文件
2. 实现必需的三个函数：`GoRequest`、`GoProxy`、`GoInject`
3. 使用 `-config <配置名>` 参数启动程序

示例：
```bash
# 创建 staging 环境配置
cp configs/default.lua configs/staging.lua
# 编辑配置...
vim configs/staging.lua

# 使用新配置
./main -config staging
```

## 配置热重载

修改配置文件后，程序会自动检测并重启，无需手动重启。

## 常用的请求类别判断 lua 代码
### 是否是Api请求
```lua
local secFetchDest = headers["Sec-Fetch-Dest"] or headers["sec-fetch-dest"]
return secFetchDest == "empty"
```
### 是否是图片请求
```lua
local secFetchDest = headers["Sec-Fetch-Dest"] or headers["sec-fetch-dest"]
return secFetchDest == "image"
```

### 是否是css请求
```lua
local secFetchDest = headers["Sec-Fetch-Dest"] or headers["sec-fetch-dest"]
return secFetchDest == "style"
```
### 是否是js请求
```lua
local secFetchDest = headers["Sec-Fetch-Dest"] or headers["sec-fetch-dest"]
return secFetchDest == "script"
```

## 注意事项

- 配置文件必须是有效的 Lua 脚本
- 三个函数（GoRequest、GoProxy、GoInject）必须实现
- bodyFilePath 返回空字符串表示正常转发请求
- 返回 bodyFilePath 时会直接返回文件内容，不转发请求