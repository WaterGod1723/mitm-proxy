### 中间人代理与证书配置
```mermaid
sequenceDiagram
   client->>connector:request
   client->>connector:request1
   connector->>connector: handle for requests
   connector-)Server:request
   connector-)Server:request1
   Server--)connector:response
   Server--)connector:response1
   connector->>connector: handle for responses
   connector-->>client:response
   connector-->>client:response1
```

1. **根证书生成与安装**：
   - 中间人代理需要生成并安装根证书，以确保跨域请求不被浏览器拦截。
   - 在Linux或macOS环境下，可以通过运行`cert`目录中的Shell脚本来生成根证书。
   - 在Windows环境下，将生成的证书文件复制一份，并将后缀改为`.crt`，然后双击即可安装。
   - 如果不安装根证书，所有跨域请求将被浏览器拦截，并显示安全警告。

2. **中间人代理与前端开发**：
   - 中间人代理（MITM）通常用于前端开发中，将本地请求代理到线上环境，方便调试和测试。
   - 在`main.go`文件中，提供了一个前端使用本地静态资源请求线上接口的示例。该示例需要结合浏览器的代理插件使用。
   - 中间人代理的核心代码位于`core/mitm`目录中，开发者可以参考`main.go`中的示例进行使用。

3. **动态配置**：
   - 配置文件使用Lua脚本实现，允许开发者动态修改请求的协议、域名、地址和代理设置，提供了更高的灵活性和可配置性。

### 总结：
- 中间人代理在本地开发中非常有用，尤其是在前端开发中代理线上请求时。
- 通过生成和安装根证书，可以避免浏览器拦截跨域请求。
- 配置文件使用Lua脚本，支持动态调整请求参数，便于开发和调试。
