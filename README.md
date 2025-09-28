# Bore Go Client SDK

这是一个用Go语言实现的bore客户端SDK，可以让你轻松地将本地服务暴露到公网。

## 特性

- 🚀 简单易用的API设计
- 🔐 支持HMAC-SHA256认证
- 🌐 自动端口分配或指定端口
- 📡 双向数据代理
- ⚡ 基于Go的高性能实现
- 🔄 与Rust版本完全兼容的协议

## 快速开始

### 1. 基本使用

```go
package main

import (
    "context"
    "log"
    "time"
    "bore"
)

func main() {
    // 创建客户端配置
    config := bore.ClientConfig{
        LocalHost:  "localhost", // 本地服务地址
        LocalPort:  8080,        // 本地服务端口
        RemoteHost: "sssh.hpc.pub", // 远程服务器地址
        RemotePort: 0,           // 0表示让服务器分配端口
        Secret:     "hpc.pub",   // 认证密钥（可选）
        BindIP:     "0.0.0.0",   // 服务器绑定IP（可选）
    }

    // 创建客户端
    client, err := bore.NewClient(config)
    if err != nil {
        log.Fatalf("创建客户端失败: %v", err)
    }
    defer client.Close()

    // 连接到服务器
    ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
    defer cancel()

    if err := client.Connect(ctx); err != nil {
        log.Fatalf("连接服务器失败: %v", err)
    }

    // 获取分配的远程端口
    remotePort := client.GetRemotePort()
    log.Printf("远程端口: %d", remotePort)

    // 开始监听连接
    if err := client.Listen(ctx); err != nil {
        log.Printf("监听连接时出错: %v", err)
    }
}
```

### 2. 不使用认证

```go
config := bore.ClientConfig{
    LocalHost:  "localhost",
    LocalPort:  3000,
    RemoteHost: "sssh.hpc.pub",
    RemotePort: 0,
    // 不设置Secret，表示不使用认证
    BindIP: "127.0.0.1",
}
```

### 3. 指定远程端口

```go
config := bore.ClientConfig{
    LocalHost:  "localhost",
    LocalPort:  9000,
    RemoteHost: "sssh.hpc.pub",
    RemotePort: 12345, // 指定远程端口
    Secret:     "hpc.pub",
    BindIP:     "0.0.0.0",
}
```

## API 参考

### ClientConfig

客户端配置结构体：

```go
type ClientConfig struct {
    LocalHost  string // 本地服务地址
    LocalPort  int    // 本地服务端口
    RemoteHost string // 远程服务器地址
    RemotePort int    // 0表示让服务器分配端口
    Secret     string // 认证密钥（可选）
    BindIP     string // 服务器绑定IP（可选）
}
```

### Client

客户端结构体：

```go
// NewClient 创建新的客户端
func NewClient(config ClientConfig) (*Client, error)

// Connect 连接到服务器
func (c *Client) Connect(ctx context.Context) error

// Listen 开始监听连接
func (c *Client) Listen(ctx context.Context) error

// GetRemotePort 获取远程端口
func (c *Client) GetRemotePort() int

// Close 关闭连接
func (c *Client) Close() error
```

## 运行测试

1. 确保你的本地有服务运行在指定端口（如8080）
2. 运行测试文件：

```bash
cd go-src
go run testfile.go
```

3. 或者运行示例：

```bash
go test -run ExampleNewClient -v
```

## 协议兼容性

这个Go实现与原始的Rust bore客户端完全兼容，使用相同的：

- 控制端口：7837
- 消息协议：JSON over null-delimited frames
- 认证机制：HMAC-SHA256
- 代理协议：TCP流代理

## 依赖

- Go 1.21+
- github.com/google/uuid

## 许可证

与主项目相同的许可证。
