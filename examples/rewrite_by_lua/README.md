# MITM Proxy 示例使用说明

这是一个基于Go语言的MITM（中间人）代理程序示例，支持请求重写、代理设置和HTML注入功能。

## 功能特性

- 支持HTTP/HTTPS协议代理
- 通过Lua脚本灵活配置请求重写规则
- 支持代理服务器配置
- 支持HTML页面注入
- 配置文件热重载
- 自定义证书支持

## 安装和运行

### 编译运行

1. 进入示例目录
```bash
cd /Users/admin/code/go/mitm-proxy/examples
```

2. 编译程序
```bash
go build -o main main.go
```

3. 运行代理服务
```bash
./main
```

### 自定义监听地址

可以通过`-addr`参数自定义代理服务地址：

```bash
./main -addr 0.0.0.0:8888
```

## 证书配置

### 生成证书（可选）

如果需要使用自定义证书，可以运行cert目录下的脚本生成：

```bash
cd cert
./generateCA.sh
```

### 安装证书

为了让浏览器信任MITM代理，需要将生成的证书安装到系统或浏览器中：

1. 找到生成的证书文件：`cert/cert.pem`
2. 按照操作系统或浏览器的要求安装证书
3. 在浏览器中设置证书信任

## 配置文件说明

配置文件`config.lua`支持以下功能：

### 1. 请求重写 (`GoRequest`)

修改请求的协议、主机和路径：

```lua
function GoRequest(protocol, host, path)
    -- 根据条件修改请求
    if host == "example.com" then
        host = "test.example.com"
    end
    return protocol, host, path
end
```

### 2. 代理设置 (`GoProxy`)

为不同主机配置代理服务器：

```lua
function GoProxy(host)
    -- 为特定主机设置代理
    if host == "google.com" then
        return "http://proxy.example.com:8080"
    end
    return ''  -- 直连
end
```

### 3. HTML注入 (`GoInject`)

向HTML页面注入自定义内容：

```lua
function GoInject(host)
    -- 为特定主机注入HTML
    if host == "example.com" then
        return "./inject.html"
    end
    return ""
end
```

## 浏览器代理设置

### 手动设置代理

#### Chrome浏览器

1. 打开Chrome浏览器
2. 点击右上角菜单 → 设置
3. 点击左侧菜单 → 系统 → 打开您计算机的代理设置
4. 在弹出的系统代理设置中：
   - Windows：选择"手动设置代理"，设置地址为代理服务器IP（如127.0.0.1），端口为代理服务端口（如8003）
   - macOS：选择"Web代理(HTTP)"和"安全Web代理(HTTPS)"，设置地址和端口
   - Linux：根据使用的桌面环境设置代理

#### Firefox浏览器

1. 打开Firefox浏览器
2. 点击右上角菜单 → 设置
3. 在搜索栏输入"代理"，点击"设置"按钮
4. 选择"手动配置代理"
5. 设置HTTP代理和SSL代理的地址（如127.0.0.1）和端口（如8003）
6. 点击"确定"保存设置

### 使用代理插件

推荐使用以下浏览器插件来方便地切换代理：

#### Chrome/Firefox插件

1. **SwitchyOmega**
   - 支持多代理配置切换
   - 支持PAC脚本
   - 支持快捷键切换
   - 下载地址：
     - Chrome: [Chrome Web Store](https://chrome.google.com/webstore/detail/proxy-switchyomega/padekgcemlokbadohgkifijomclgjgif)
     - Firefox: [Firefox Add-ons](https://addons.mozilla.org/zh-CN/firefox/addon/switchyomega/)

2. **Proxy Switcher and Manager**
   - 简单易用的代理切换工具
   - 支持快速切换
   - 支持代理自动配置
   - 下载地址：
     - Chrome: [Chrome Web Store](https://chrome.google.com/webstore/detail/proxy-switcher-and-manage/onnfghpihccifgojkpnnncpagjcdbjod)
     - Firefox: [Firefox Add-ons](https://addons.mozilla.org/zh-CN/firefox/addon/proxy-switcher-and-manager/)

#### 插件设置步骤

以SwitchyOmega为例：

1. 安装插件并打开插件设置
2. 点击"新建情景模式"，选择"代理服务器"
3. 命名为"MITM Proxy"
4. 协议选择"HTTP"
5. 服务器地址输入代理服务器IP（如127.0.0.1）
6. 端口输入代理服务端口（如8003）
7. 点击"应用选项"保存设置
8. 在浏览器右上角的插件图标中选择"MITM Proxy"即可切换到该代理

## 示例使用场景

### 环境切换

将生产环境的请求转发到测试环境：

```lua
function GoRequest(protocol, host, path)
    if host == "prod.example.com" then
        host = "test.example.com"
    end
    return protocol, host, path
end
```

### 响应Mock

直接返回本地文件作为响应：

```lua
function GoRequest(protocol, host, path)
    if host == "api.example.com" and path == "/user/info" then
        return protocol, host, path, "./mock/user_info.json"
    end
    return protocol, host, path
end
```

### HTML注入

向页面注入自定义脚本：

```lua
function GoInject(host)
    if host == "example.com" then
        return "./inject.html"
    end
    return ""
end
```

## 注意事项

1. 使用MITM代理时，浏览器可能会显示安全警告，需要信任代理证书
2. 不要在生产环境中使用未授权的MITM代理
3. 定期更新证书以确保安全性
4. 配置文件修改后会自动重载，无需重启代理服务

## 问题排查

1. 代理服务无法启动：检查端口是否被占用
2. 浏览器无法连接：检查代理设置是否正确
3. HTTPS网站无法访问：检查证书是否正确安装
4. 配置不生效：检查Lua脚本语法是否正确

## 联系方式

如有问题或建议，欢迎提交Issue或Pull Request。