# Shermie-Proxy 代码说明文档

## 1. 项目概述

Shermie-Proxy 是一个基于 Go 语言编写的多功能代理服务器，支持 HTTP、HTTPS、WebSocket（WS/WSS）、TCP 透传和 SOCKS5 协议的统一代理入口。

**核心特性：**

- **协议自动识别**：通过 Peek 读取连接首字节自动判断协议类型，无需预先配置端口协议
- **TLS 中间人代理**：动态生成子证书进行 HTTPS 解密，实现对加密流量的拦截和修改
- **事件回调机制**：提供 10 个可注册的事件回调函数，允许用户在请求/响应各阶段拦截和修改数据
- **DNS 缓存**：内置 5 分钟 TTL 的 DNS 缓存，减少重复解析开销
- **多端口多网卡**：支持同时监听多个端口，每个端口可绑定不同出口网卡
- **上级代理链**：支持通过 `--proxy` 参数设置上级代理，实现代理链

**技术栈：**

| 技术 | 说明 |
|------|------|
| Go 1.16 | 运行时版本 |
| `crypto/tls` | TLS 中间人证书生成与握手 |
| `crypto/x509` | X.509 证书解析与生成 |
| `net/http` | HTTP 请求解析与转发 |
| gorilla/websocket (fork) | WebSocket 协议处理（内嵌于 `Core/Websocket/`） |
| `viki-org/dnscache` | DNS 结果缓存 |
| `golang.org/x/sys` | Windows 系统调用（证书安装、代理设置） |

---

## 2. 目录结构与模块职责

```
shermie-proxy/
├── Main.go                    # 程序入口，CLI 参数解析，事件回调注册示例
├── Core/                      # 核心代理逻辑
│   ├── ProxyServer.go         # 服务器启动、连接接受、协议识别与分发
│   ├── ProxyHttp.go           # HTTP/HTTPS/WS/WSS 协议处理
│   ├── ProxySocks5.go         # SOCKS5 协议处理
│   ├── ProxyTcp.go            # TCP 透传代理处理
│   ├── ConnPeer.go            # 连接上下文封装（conn + reader + writer + server）
│   ├── Certificate.go         # TLS 证书生成（根证书 + 叶子证书）
│   ├── Cache.go               # 证书缓存存储（并发安全）
│   └── Websocket/             # gorilla/websocket fork，WebSocket 协议实现
│       ├── Client.go          # WebSocket 客户端拨号
│       ├── Conn.go            # WebSocket 连接管理
│       ├── Server.go          # WebSocket 服务端升级
│       └── ...                # 其他 WebSocket 内部实现
├── Contract/                  # 接口定义
│   └── IServerProcesser.go    # 协议处理器接口（Handle() 方法）
├── Constant/                  # 常量定义（当前为空）
│   └── Certificate.go
├── Log/                       # 日志模块
│   └── Logger.go              # 标准日志封装
├── Utils/                     # 工具函数 + 平台适配
│   ├── Utils.go               # 通用工具（文件检测、端口检测、反射读取 TLS 内部数据）
│   ├── Windows.go             # Windows 平台：证书安装 + 系统代理设置
│   ├── Macos.go               # macOS 平台：返回不支持
│   └── Linux.go               # Linux 平台：返回不支持
├── cert.crt                   # 运行时生成的根证书文件
├── cert.key                   # 运行时生成的根证书私钥文件
├── go.mod                     # Go 模块定义
└── go.sum                     # 依赖校验
```

### 2.1 模块依赖关系

```
Main.go
  ├── Core.ProxyServer     (服务器核心)
  │     ├── Core.ProxyHttp      (HTTP 处理器)
  │     │     ├── Core.Websocket    (WebSocket 子包)
  │     │     ├── Core.Cache        (证书缓存)
  │     │     └── Core.Certificate  (证书生成)
  │     ├── Core.ProxySocks5    (SOCKS5 处理器)
  │     └── Core.ProxyTcp       (TCP 处理器)
  ├── Log.Logger            (日志)
  └── Utils                 (工具函数)
```

---

## 3. 核心架构

### 3.1 服务器启动流程

```
main()
  ├── init()                      # 初始化日志 + 根证书
  ├── flag.Parse()                # 解析 CLI 参数
  ├── ListenBranch() (per port)   # 每个端口启动一个 goroutine
  │     └── ProxyServer.Start()
  │           ├── beforeStart()   # 打印 Logo，一次性初始化
  │           ├── ListenTCP()     # 绑定端口
  │           └── MultiListen()   # 启动 5 个 Accept goroutine
  │                 └── handle(conn)  # 每个连接一个 goroutine
  └── select {}                   # 主 goroutine 永久阻塞
```

### 3.2 协议识别机制

当新连接到达时，`handle()` 函数执行协议识别：

```go
// ProxyServer.go - handle()
reader := bufio.NewReader(conn)
peek, err := reader.Peek(9)       // 窥探前 9 字节，不消费

if isHttpMethod(peek) {
    process = &ProxyHttp{...}     // HTTP/HTTPS/WS/WSS
} else if peek[0] == 0x05 {
    process = &ProxySocks5{...}   // SOCKS5
} else {
    process = &ProxyTcp{...}      // 纯 TCP
}
process.Handle()
```

**HTTP 方法识别**：使用完整的 HTTP 方法字符串前缀匹配（`"GET "`, `"POST "`, `"PUT "` 等），而非单字节比较，避免 POST 和 PUT 首字母冲突。

**识别优先级**：
1. HTTP 方法前缀匹配 -> `ProxyHttp`
2. 首字节为 `0x05` -> `ProxySocks5`
3. 其他 -> `ProxyTcp`

### 3.3 连接上下文 (`ConnPeer`)

`ConnPeer` 是所有协议处理器的共享基座，通过 Go 的嵌入机制（embedding）被 `ProxyHttp`、`ProxySocks5`、`ProxyTcp` 继承：

```go
type ConnPeer struct {
    conn   net.Conn          // 客户端 TCP 连接
    writer *bufio.Writer     // 带缓冲的写入器
    reader *bufio.Reader     // 带缓冲的读取器
    server *ProxyServer      // 所属服务器实例（用于访问回调函数和配置）
}
```

### 3.4 协议处理器接口

所有处理器实现 `IServerProcesser` 接口：

```go
// Contract/IServerProcesser.go
type IServerProcesser interface {
    Handle()
}
```

---

## 4. 协议处理器详解

### 4.1 ProxyHttp — HTTP/HTTPS/WS/WSS 处理器

**文件**：`Core/ProxyHttp.go`（491 行）

**结构体**：

```go
type ProxyHttp struct {
    ConnPeer                         // 嵌入连接上下文
    request  *http.Request           // 当前请求
    response *http.Response          // 当前响应
    upgrade  *Websocket.Upgrader     // WebSocket 升级器（延迟初始化）
    target   net.Conn                // 目标服务器连接（HTTPS 隧道）
    tls      bool                    // 是否为 TLS 连接
    port     string                  // 目标端口
}
```

#### 4.1.1 处理入口 `Handle()`

```
Handle()
  ├── 解析请求：http.ReadRequest(reader)
  ├── 提取端口号：从 request.Host 解析
  ├── 判断请求类型：
  │     ├── CONNECT 方法 -> handleSslRequest()   [HTTPS/WSS]
  │     └── 其他方法    -> handleRequest()        [普通 HTTP]
```

#### 4.1.2 HTTPS 隧道建立 `handleSslRequest()`

处理 HTTPS CONNECT 请求的核心流程：

```
handleSslRequest()
  ├── 连接目标服务器：
  │     ├── 有上级代理 -> net.Dial("tcp", proxy)
  │     ├── 端口 443  -> tls.Dial("tcp", host)    [直连 TLS]
  │     └── 其他端口  -> net.Dial("tcp", host)     [普通 TCP]
  ├── 向客户端返回 "HTTP/1.1 200 Connection Established"
  └── SslReceiveSend()    [进入 TLS 中间人流程]
```

#### 4.1.3 TLS 中间人握手 `SslReceiveSend()`

```
SslReceiveSend()
  ├── Cache.GetCertificate(host, port)  -> 获取/生成子证书
  ├── tls.Server(conn, cert)            -> 创建 TLS 服务端连接
  ├── sslConn.Handshake()               -> 与客户端完成 TLS 握手
  │     ├── 成功 -> 替换 conn 为 sslConn，继续处理
  │     └── 失败 -> handleWsHandshakeErr() [尝试解析原始 WS 请求]
  ├── 读取解密后的 HTTP 请求
  ├── 判断协议：
  │     ├── Upgrade: websocket -> handleWsRequest()  [WebSocket]
  │     └── 普通请求           -> handleRequest()    [HTTPS]
```

#### 4.1.4 HTTP 请求转发 `handleRequest()`

```
handleRequest()
  ├── 特殊路径 "/tls" -> 返回根证书文件供客户端下载
  ├── 读取请求体：ReadRequestBody()
  ├── 触发 OnHttpRequestEvent 回调 -> 用户可修改请求数据
  ├── Transport(request)             -> 转发请求到目标服务器
  ├── 读取响应体：ReadResponseBody() -> 自动处理 gzip 解码
  ├── 触发 OnHttpResponseEvent 回调 -> 用户可修改响应数据
  └── response.Write(conn)           -> 将响应写回客户端
```

#### 4.1.5 WebSocket 双向桥接 `handleWsRequest()`

```
handleWsRequest()
  ├── Upgrader.Upgrade()       -> 升级客户端连接为 WebSocket
  ├── Dialer.Dial()            -> 连接远程 WebSocket 服务器
  │     └── 使用 DialContext() 进行 DNS 缓存拨号
  ├── goroutine 1: targetWsConn.ReadMessage() -> OnWsResponseEvent -> clientWsConn.WriteMessage()
  ├── goroutine 2: clientWsConn.ReadMessage() -> OnWsRequestEvent  -> targetWsConn.WriteMessage()
  └── 任一方向出错 -> 停止桥接
```

#### 4.1.6 HTTP 转发器 `Transport()`

```go
func (i *ProxyHttp) Transport(request *http.Request) (*http.Response, error) {
    transport := &http.Transport{
        DisableKeepAlives:     true,
        TLSHandshakeTimeout:   15 * time.Second,
        ResponseHeaderTimeout: 30 * time.Second,
        DialContext:           i.DialContext(),
        TLSClientConfig:       &tls.Config{InsecureSkipVerify: true},
    }
    // 如果配置了上级代理
    if i.server.proxy != "" {
        transport.Proxy = http.ProxyURL(&url.URL{Host: i.server.proxy})
    }
    response, err := transport.RoundTrip(request)
    // 移除 hop-by-hop 头
    i.RemoveHeader(response.Header)
    return response, err
}
```

#### 4.1.7 DNS 缓存拨号 `DialContext()`

```go
func (i *ProxyHttp) DialContext() func(ctx context.Context, network, addr string) (net.Conn, error) {
    return func(ctx context.Context, network, addr string) (net.Conn, error) {
        // 1. 从 DNS 缓存获取 IP（5 分钟 TTL）
        ipList, _ := i.server.dns.Fetch(hostname)
        // 2. 优先使用 IPv4 地址
        // 3. 指定网卡绑定（如果配置了 --network）
        // 4. 使用 net.Dialer 拨号
        // 5. 设置 TCP NoDelay（Nagle 算法控制）
    }
}
```

#### 4.1.8 请求头清理 `RemoveHeader()`

转发前移除以下 hop-by-hop 头部，防止代理链中出现连接状态混乱：

```
Keep-Alive, Transfer-Encoding, TE, Connection,
Trailer, Upgrade, Proxy-Authorization,
Proxy-Authenticate, Accept-Encoding
```

---

### 4.2 ProxySocks5 — SOCKS5 协议处理器

**文件**：`Core/ProxySocks5.go`（300 行）

**结构体**：

```go
type ProxySocks5 struct {
    ConnPeer
    target net.Conn    // 目标连接
    port   string      // 目标端口
}
```

#### 4.2.1 SOCKS5 握手流程

```
Handle()
  ├── 1. 读取版本号（0x05）
  ├── 2. 读取认证方法数量 + 方法列表
  ├── 3. 回复认证结果（无认证：0x00）
  ├── 4. 读取命令请求：
  │     ├── 版本号（0x05）
  │     ├── 命令类型：CONNECT(0x01) / BIND(0x02) / UDP(0x03)
  │     ├── 保留位（0x00）
  │     └── 目标地址：
  │           ├── IPv4（0x01）-> 读 4 字节
  │           ├── IPv6（0x04）-> 读 16 字节
  │           └── 域名（0x03）-> 读长度字节 + 域名数据
  ├── 5. 读取端口号（2 字节大端）
  ├── 6. 连接目标服务器：
  │     ├── UDP -> net.DialTimeout("udp")
  │     ├── 端口 443 -> tls.DialWithDialer("tcp")
  │     └── 其他 -> net.DialTimeout("tcp")
  ├── 7. 回复连接结果（成功/失败 + 绑定地址）
  └── 8. 启动双向数据转发
        ├── goroutine 1: conn -> target (SocketClient)
        └── goroutine 2: target -> conn (SocketServer)
```

#### 4.2.2 辅助函数

| 函数 | 说明 |
|------|------|
| `ByteToInt(input []byte) int32` | 2 字节大端转整型（用于端口解析） |
| `IpV4(ipAddr string) bool` | 判断地址是否为 IPv4 |
| `IpV6(ipAddr string) bool` | 判断地址是否为 IPv6 |

---

### 4.3 ProxyTcp — TCP 透传代理处理器

**文件**：`Core/ProxyTcp.go`（112 行）

**结构体**：

```go
type ProxyTcp struct {
    ConnPeer
    target net.Conn    // 目标连接
    port   string      // 目标端口
}
```

#### 4.3.1 处理流程

```
Handle()
  ├── 解析目标地址：--to 参数指定的地址
  ├── net.DialTCP() 连接目标
  ├── 获取证书 -> Cache.GetCertificate(host, port)
  ├── 可选 TLS 握手：sslConn.Handshake()
  │     ├── 成功 -> 替换 conn 为 sslConn
  │     └── 失败 -> 保持原始连接（明文转发）
  ├── 设置 Nagle 算法
  └── 启动双向数据转发
        ├── goroutine 1: conn -> target (TcpClient)
        └── goroutine 2: target -> conn (TcpServer)
```

#### 4.3.2 数据转发 `Transport()`

```go
func (i *ProxyTcp) Transport(out chan<- error, originConn, targetConn net.Conn, role string) {
    buff := make([]byte, 4096)    // 4KB 缓冲区
    for {
        readLen, err := originConn.Read(buff)
        if readLen > 0 {
            // 根据角色触发对应的回调
            if role == TcpServer {
                // OnTcpServerStreamEvent
            } else {
                // OnTcpClientStreamEvent
            }
            // 写入目标连接并校验写入长度
        }
        if err != nil { break }
    }
}
```

---

## 5. 事件回调系统

事件回调是 Shermie-Proxy 的核心扩展机制，允许用户在数据流经代理时进行拦截、检查和修改。所有回调注册在 `ProxyServer` 实例上，在 `Main.go` 的 `ListenBranch()` 函数中设置。

### 5.1 回调类型定义

```go
// ProxyServer.go
type HttpRequestEvent   func(message []byte, request *http.Request, resolve ResolveHttpRequest, conn net.Conn) bool
type HttpResponseEvent  func(message []byte, response *http.Response, resolve ResolveHttpResponse, conn net.Conn) bool
type Socks5ResponseEvent func(message []byte, resolve ResolveSocks5, conn net.Conn) (int, error)
type Socks5RequestEvent  func(message []byte, resolve ResolveSocks5, conn net.Conn) (int, error)
type WsRequestEvent     func(msgType int, message []byte, resolve ResolveWs, conn net.Conn) error
type WsResponseEvent    func(msgType int, message []byte, resolve ResolveWs, conn net.Conn) error
type TcpConnectEvent    func(conn net.Conn)
type TcpClosetEvent     func(conn net.Conn)
type TcpServerStreamEvent func(message []byte, resolve ResolveTcp, conn net.Conn) (int, error)
type TcpClientStreamEvent func(message []byte, resolve ResolveTcp, conn net.Conn) (int, error)
```

### 5.2 Resolve 函数签名

```go
type ResolveHttpRequest  func(message []byte, request *http.Request)     // 修改请求体
type ResolveHttpResponse func(message []byte, response *http.Response)   // 修改响应体
type ResolveWs          func(msgType int, message []byte) error          // 转发 WS 消息
type ResolveSocks5      func(buff []byte) (int, error)                   // 转发 SOCKS5 数据
type ResolveTcp         func(buff []byte) (int, error)                   // 转发 TCP 数据
```

### 5.3 回调详细列表

| 回调 | 触发时机 | 返回值语义 |
|------|----------|------------|
| `OnTcpConnectEvent(conn)` | 新 TCP 连接建立时 | 无 |
| `OnTcpCloseEvent(conn)` | TCP 连接关闭时（defer 中触发） | 无 |
| `OnHttpRequestEvent(body, req, resolve, conn)` | HTTP 请求到达，转发前 | `true` 继续转发，`false` 跳过 |
| `OnHttpResponseEvent(body, resp, resolve, conn)` | HTTP 响应到达，返回客户端前 | `true` 继续返回，`false` 跳过 |
| `OnSocks5RequestEvent(data, resolve, conn)` | SOCKS5 客户端发送数据 | 返回 `(写入字节数, error)` |
| `OnSocks5ResponseEvent(data, resolve, conn)` | SOCKS5 服务端返回数据 | 返回 `(写入字节数, error)` |
| `OnWsRequestEvent(type, data, resolve, conn)` | WebSocket 客户端发送消息 | 返回 `error` |
| `OnWsResponseEvent(type, data, resolve, conn)` | WebSocket 服务端返回消息 | 返回 `error` |
| `OnTcpClientStreamEvent(data, resolve, conn)` | TCP 客户端 -> 服务端方向 | 返回 `(写入字节数, error)` |
| `OnTcpServerStreamEvent(data, resolve, conn)` | TCP 服务端 -> 客户端方向 | 返回 `(写入字节数, error)` |

### 5.4 回调使用模式

每个回调都提供一个 `resolve` 函数，用户通过调用它来完成默认行为，或在调用前修改数据：

```go
// 示例：修改 HTTP 请求体
s.OnHttpRequestEvent = func(message []byte, request *http.Request, resolve Core.ResolveHttpRequest, conn net.Conn) bool {
    // 修改 message...
    modifiedMessage := append(message, []byte("injected")...)
    resolve(modifiedMessage, request)  // 调用 resolve 完成转发
    return true                        // 返回 true 继续处理
}

// 示例：拦截并自定义处理
s.OnHttpResponseEvent = func(body []byte, response *http.Response, resolve Core.ResolveHttpResponse, conn net.Conn) bool {
    conn.Write([]byte("custom response"))  // 自己写回客户端
    return false                            // 返回 false 跳过默认写回
}
```

---

## 6. 证书系统

### 6.1 根证书生命周期

```
程序启动 init()
  ├── NewCertificate().Init()
  │     ├── 检查 "./cert.crt" 是否存在
  │     ├── 不存在 -> GenerateRootPemFile("Shermie")
  │     │     ├── 生成 RSA 2048 密钥对
  │     │     ├── 创建 X.509 自签名证书（有效期 2 年）
  │     │     ├── 写入 cert.key（私钥 PEM）
  │     │     └── 写入 cert.crt（证书 PEM）
  │     ├── 已存在 -> 从文件读取并解析
  │     ├── 解析证书：x509.ParseCertificate()
  │     ├── 解析私钥：x509.ParsePKCS1PrivateKey()
  │     └── 设置全局变量 Cert = i
```

**根证书属性**：

| 属性 | 值 |
|------|-----|
| CommonName | "Shermie" |
| Country | CN |
| IsCA | true |
| KeyUsage | KeyEncipherment, DigitalSignature, CertSign |
| 有效期 | 前 1 年 ~ 后 1 年（共 2 年） |
| 密钥长度 | RSA 2048 |

### 6.2 叶子证书动态生成

当客户端请求 HTTPS 站点时，代理动态生成一张以目标域名为 CN 的子证书：

```go
// Certificate.go - GeneratePem(host)
template := x509.Certificate{
    SerialNumber: rand.Int(rand.Reader, max),  // 128 位随机序列号
    Subject: pkix.Name{
        CommonName: host,     // 目标域名或 IP
    },
    NotBefore: time.Now().AddDate(-1, 0, 0),   // 有效起始
    NotAfter:  time.Now().AddDate(1, 0, 0),     // 有效结束
    KeyUsage:  KeyEncipherment | DigitalSignature | CertSign,
    IsCA:      true,
}
// 根据 host 类型设置 SAN
if ip := net.ParseIP(host); ip != nil {
    template.IPAddresses = []net.IP{ip}       // IP 地址
} else {
    template.DNSNames = []string{host}         // 域名
}
// 使用根证书签署
cert, _ := x509.CreateCertificate(rand.Reader, &template, rootCa, &priKey.PublicKey, rootKey)
```

### 6.3 证书缓存 (`Cache.go`)

**全局单例**：`var Cache = NewStorage()`

**数据结构**：

```go
type Storage struct {
    lock    *sync.Mutex
    mapping map[string]*action    // key = 域名（不含端口）
}

type action struct {
    wg     *sync.WaitGroup        // 用于同域名并发等待
    fn     func() (interface{}, error)
    cert   interface{}            // 缓存的 tls.Certificate
    err    error
}
```

**`GetCertificate()` 并发控制流程**：

```
GetCertificate(hostname, port)
  ├── lock.Lock()
  ├── 标准化 hostname（去除端口）
  ├── 检查 mapping 中是否已存在该域名
  │     ├── 存在 -> lock.Unlock()
  │     │           act.wg.Wait()     [等待正在生成的证书完成]
  │     │           return act.cert, act.err
  │     └── 不存在 -> 创建新 action
  │           ├── act.wg.Add(1)
  │           ├── mapping[host] = act
  │           └── lock.Unlock()
  ├── do(act, act.fn)
  │     └── GetAction(host)()
  │           ├── Cert.GeneratePem(host)     -> 生成证书 + 私钥
  │           └── tls.X509KeyPair()          -> 组合为 tls.Certificate
  ├── act.wg.Done()
  └── return act.cert, act.err
```

**并发特性**：
- **同一域名**的多个请求：第一个请求生成证书，后续请求通过 WaitGroup 等待复用
- **不同域名**的请求：在锁保护下串行创建 action 对象，但证书生成（`do()`）在锁外并行执行
- 证书一旦生成即永久缓存（无淘汰策略）

---

## 7. CLI 参数说明

程序通过 `flag` 包解析命令行参数：

| 参数 | 类型 | 默认值 | 说明 |
|------|------|--------|------|
| `--port` | string | `"9090"` | 监听端口。支持逗号分隔多端口，如 `"9090,9091"` |
| `--nagle` | bool | `true` | 是否对远程连接启用 Nagle 算法（`SetNoDelay`） |
| `--proxy` | string | `""` | 上级代理地址，格式 `host:port` |
| `--to` | string | `""` | TCP 透传代理的目标地址（仅 TCP 协议生效） |
| `--network` | string | `""` | 强制使用的出口网卡 IP 地址。多端口时需与 `--port` 数量一致 |

**多端口示例**：

```bash
# 两个端口分别绑定不同网卡
./shermie-proxy --port 9090,9091 --network 192.168.1.100,10.0.0.5
```

**Nagle 算法说明**：`--nagle true` 表示启用 Nagle 算法，底层调用 `SetNoDelay(true)` 实际上是**禁用** Nagle（即低延迟模式）。默认值 `true` 意味着默认以低延迟模式运行。

---

## 8. 平台适配

### 8.1 Windows (`Utils/Windows.go`)

**证书安装 `InstallCert()`**：
- 读取 PEM 文件 -> 解码 -> 调用 `CertCreateCertificateContext()`
- 打开系统 Root 证书存储区 -> `CertAddCertificateContextToStore()`

**系统代理设置 `SetSystemProxy()`**：
- 动态加载 `Wininet.dll`
- 通过 `InternetSetOptionW` 设置/清除代理
- 内置旁路规则：`localhost;127.*;10.*;172.16-31.*;192.168.*`

### 8.2 macOS (`Utils/Macos.go`)

`InstallCert()` 和 `SetSystemProxy()` 均返回 `"不支持Macos系统"` 错误。

### 8.3 Linux (`Utils/Linux.go`)

`InstallCert()` 和 `SetSystemProxy()` 均返回 `"不支持Linux系统"` 错误。

### 8.4 通用工具 (`Utils/Utils.go`)

| 函数 | 签名 | 说明 |
|------|------|------|
| `FileExist` | `func FileExist(file string) bool` | 检查文件是否存在 |
| `GetAvailablePort` | `func GetAvailablePort() (int, error)` | 获取系统分配的可用端口（绑定 `:0`） |
| `IsPortAvailable` | `func IsPortAvailable(port int) bool` | 检查指定端口是否可用 |
| `GetLastTimeFrame` | `func GetLastTimeFrame(conn *tls.Conn, property string) []byte` | 通过 unsafe 反射读取 `tls.Conn` 内部未导出字段（如 `rawInput`），用于 TLS 握手失败时解析原始数据 |

---

## 9. 数据流图

### 9.1 HTTP 请求流

```
客户端 --[HTTP Request]--> ProxyServer
  ├── Peek(9) -> 匹配 HTTP 方法
  ├── ProxyHttp.Handle()
  │     ├── http.ReadRequest() 解析请求
  │     ├── OnHttpRequestEvent 回调（用户可修改请求体）
  │     ├── Transport() -> http.Transport.RoundTrip() -> 目标服务器
  │     ├── ReadResponseBody() 解析响应（自动解压 gzip）
  │     ├── OnHttpResponseEvent 回调（用户可修改响应体）
  │     └── response.Write(conn) 返回客户端
```

### 9.2 HTTPS CONNECT 隧道流

```
客户端 --[CONNECT host:443]--> ProxyServer
  ├── ProxyHttp.Handle() -> handleSslRequest()
  │     ├── 连接目标服务器（tls.Dial 或 net.Dial）
  │     ├── 返回 "200 Connection Established"
  │     └── SslReceiveSend()
  │           ├── Cache.GetCertificate(host) -> 生成子证书
  │           ├── tls.Server(conn, cert) -> TLS 握手
  │           ├── http.ReadRequest() 解密读取真实请求
  │           ├── Transport() 转发到目标
  │           └── response.Write(sslConn) 加密返回
```

### 9.3 WebSocket 桥接流

```
浏览器 <==[WebSocket Frames]==> ProxyServer <==[WebSocket Frames]==> WS 服务器

客户端连接 -> CONNECT -> TLS 握手 -> 检测 Upgrade: websocket
  ├── Upgrader.Upgrade(客户端连接)
  ├── Dialer.Dial(目标 WS 地址)
  ├── goroutine A: 服务端 -> 读取 -> OnWsResponseEvent -> 写入 -> 客户端
  └── goroutine B: 客户端 -> 读取 -> OnWsRequestEvent  -> 写入 -> 服务端
```

### 9.4 SOCKS5 握手+转发流

```
客户端                     ProxyServer                  目标服务器
  |                             |                             |
  |-- 版本号 + 方法列表 -------->|                             |
  |<-- 认证结果 (无认证) --------|                             |
  |-- 命令 + 地址 + 端口 ------->|                             |
  |                             |-- TCP/UDP 连接 ------------>|
  |<-- 连接结果 + 绑定地址 ------|                             |
  |                             |                             |
  |--[数据]-------------------->|---- OnSocks5Request ------>|
  |<--- OnSocks5Response <------|<-----------[数据]-----------|
```

### 9.5 TCP 透传流

```
客户端                ProxyServer              目标服务器(--to)
  |                        |                        |
  |--[TCP 数据]----------->|                        |
  |                        |-- net.DialTCP -------->|
  |                        |-- TLS 握手(可选) ------>|
  |                        |                        |
  |                        |<-- TcpServerStream ----|
  |-- TcpClientStream ---->|                        |
  |                        |                        |
  |                        |== 双向数据转发 =========|
```

---

## 10. 关键设计决策

### 10.1 协议识别：Peek 前缀匹配

**决策**：使用 `bufio.Reader.Peek(9)` 窥探连接前 9 字节，不消费数据流，然后通过 `isHttpMethod()` 进行 HTTP 方法字符串前缀匹配。

**原因**：所有协议共用同一端口，必须在连接层判断协议类型。Peek 不消费数据，保证后续处理器可以完整读取。选择 9 字节是因为最长的 HTTP 方法 `"OPTIONS"` 加空格正好 8 字节。

### 10.2 DNS 缓存：5 分钟 TTL

**决策**：使用 `viki-org/dnscache` 库，TTL 固定为 5 分钟。

**原因**：代理场景下频繁的 DNS 查询是性能瓶颈，5 分钟 TTL 在准确性和性能之间取得平衡。缓存结果在 `DialContext()` 中被使用。

### 10.3 多端口启动：每端口独立 goroutine

**决策**：`--port` 支持逗号分隔多端口，每个端口在独立 goroutine 中启动完整的 `ProxyServer`。

**原因**：允许多网卡环境下的灵活部署。每个端口可绑定不同的出口 IP（`--network`），实现流量分流。端口数量与网卡数量必须一致。

### 10.4 并发证书生成：WaitGroup + Mutex

**决策**：`Storage` 使用 Mutex 保护 mapping 的并发写入，WaitGroup 确保同一域名只生成一次证书。

**原因**：高并发场景下多个 goroutine 可能同时请求同一域名的证书。第一个请求负责生成，后续请求通过 WaitGroup 等待复用，避免重复的 RSA 密钥生成开销。不同域名之间互不阻塞。

### 10.5 TLS 中间人：动态子证书

**决策**：代理持有自签名根证书，对每个 HTTPS 目标动态生成以目标域名为 CN 的子证书。

**原因**：实现对 HTTPS 流量的透明拦截。客户端需信任根证书（可通过 `http://127.0.0.1/tls` 下载）。子证书使用 128 位随机序列号，有效期 2 年，包含 SAN 扩展（DNS 或 IP）。

### 10.6 多 Accept goroutine

**决策**：`MultiListen()` 启动 5 个 goroutine 同时调用 `Accept()`。

**原因**：单个 Accept goroutine 在高并发下可能成为瓶颈。5 个并发 Accept 可以提高连接接受吞吐量，每个连接仍由独立 goroutine 处理。

---

## 附录：类型速查表

| 类型 | 文件 | 说明 |
|------|------|------|
| `ProxyServer` | ProxyServer.go | 服务器核心，管理监听、协议分发、回调注册 |
| `ProxyHttp` | ProxyHttp.go | HTTP/HTTPS/WS/WSS 协议处理器 |
| `ProxySocks5` | ProxySocks5.go | SOCKS5 协议处理器 |
| `ProxyTcp` | ProxyTcp.go | TCP 透传处理器 |
| `ConnPeer` | ConnPeer.go | 连接上下文基座 |
| `Certificate` | Certificate.go | 证书管理器（根证书 + 子证书生成） |
| `Storage` | Cache.go | 证书缓存存储 |
| `IServerProcesser` | IServerProcesser.go | 协议处理器接口 |
| `HttpRequestEvent` | ProxyServer.go | HTTP 请求回调类型 |
| `HttpResponseEvent` | ProxyServer.go | HTTP 响应回调类型 |
| `Socks5RequestEvent` | ProxyServer.go | SOCKS5 请求回调类型 |
| `Socks5ResponseEvent` | ProxyServer.go | SOCKS5 响应回调类型 |
| `WsRequestEvent` | ProxyServer.go | WS 请求回调类型 |
| `WsResponseEvent` | ProxyServer.go | WS 响应回调类型 |
| `TcpConnectEvent` | ProxyServer.go | TCP 连接回调类型 |
| `TcpClosetEvent` | ProxyServer.go | TCP 关闭回调类型 |
| `TcpServerStreamEvent` | ProxyServer.go | TCP 服务端流回调类型 |
| `TcpClientStreamEvent` | ProxyServer.go | TCP 客户端流回调类型 |
| `ResolveHttpRequest` | ProxyHttp.go | HTTP 请求 resolve 函数类型 |
| `ResolveHttpResponse` | ProxyHttp.go | HTTP 响应 resolve 函数类型 |
| `ResolveWs` | ProxyHttp.go | WS resolve 函数类型 |
| `ResolveSocks5` | ProxySocks5.go | SOCKS5 resolve 函数类型 |
| `ResolveTcp` | ProxyTcp.go | TCP resolve 函数类型 |
