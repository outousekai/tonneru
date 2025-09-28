# Bore Go Client 使用说明

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
        RemoteHost: "server.dns", // 远程服务器地址
        RemotePort: 0,           // 0表示让服务器分配端口
        Secret:     "secret.str",   // 认证密钥
        BindIP:     "0.0.0.0",   // 服务器绑定IP
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

### 2. 运行测试

```bash
# 进入examples目录
cd examples

# 运行测试文件
go run testfile.go
```

### 3. 在你的项目中使用

```bash
# 在你的项目中初始化go模块
go mod init your-project

# 添加bore依赖
go get ./path/to/bore/go-src
```

## 配置选项

- `LocalHost`: 本地服务地址（默认: "localhost"）
- `LocalPort`: 本地服务端口（必需）
- `RemoteHost`: 远程服务器地址（默认: "server.dns"）
- `RemotePort`: 远程端口，0表示让服务器自动分配
- `Secret`: 认证密钥（可选，默认: "secret.str"）
- `BindIP`: 服务器绑定IP（可选，默认: "0.0.0.0"）

## 注意事项

1. 确保本地服务已经启动并监听指定端口
2. 服务器地址和密钥已经预设为 `server.dns` 和 `secret.str`
3. 客户端会自动处理认证和连接管理
4. 支持优雅关闭，使用 `defer client.Close()`
