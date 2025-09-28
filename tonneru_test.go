package tonneru

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"testing"
	"time"
)

func TestClientConnect(t *testing.T) {
	// 启动一个简单的HTTP服务器作为本地服务
	go startLocalServer()

	// 等待本地服务器启动
	time.Sleep(2 * time.Second)

	// 创建客户端配置
	config := ClientConfig{
		LocalHost:  "localhost",    // 本地服务地址
		LocalPort:  8080,           // 本地服务端口
		RemoteHost: "sssh.hpc.pub", // 远程服务器地址
		RemotePort: 0,              // 0表示让服务器分配端口
		Secret:     "hpc.pub",      // 认证密钥（可选）
		BindIP:     "0.0.0.0",      // 服务器绑定IP（可选）
	}

	// 创建客户端
	client, err := NewClient(config)
	if err != nil {
		log.Fatalf("创建客户端失败: %v", err)
	}
	defer client.Close()

	// 连接到服务器
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	fmt.Println("正在连接到bore服务器...")
	if err := client.Connect(ctx); err != nil {
		log.Fatalf("连接服务器失败: %v", err)
	}

	// 获取分配的远程端口
	remotePort := client.GetRemotePort()
	fmt.Printf("✅ 连接成功！\n")
	fmt.Printf("📡 本地服务: %s:%d\n", config.LocalHost, config.LocalPort)
	fmt.Printf("🌐 远程地址: %s:%d\n", config.RemoteHost, remotePort)
	fmt.Printf("🔗 绑定IP: %s\n", config.BindIP)
	fmt.Printf("🔐 使用认证: %t\n", config.Secret != "")
	fmt.Println("\n🚀 开始监听连接...")
	fmt.Println("💡 现在可以通过远程地址访问你的本地服务了！")
	fmt.Println("⏹️  按 Ctrl+C 停止服务")

	// 开始监听连接
	if err := client.Listen(ctx); err != nil {
		log.Printf("监听连接时出错: %v", err)
	}
}

// startLocalServer 启动一个简单的本地HTTP服务器用于测试
func startLocalServer() {
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, `
<!DOCTYPE html>
<html>
<head>
    <title>Bore Go Client Test</title>
    <style>
        body { font-family: Arial, sans-serif; margin: 40px; background: #f0f0f0; }
        .container { background: white; padding: 30px; border-radius: 10px; box-shadow: 0 2px 10px rgba(0,0,0,0.1); }
        h1 { color: #333; }
        .info { background: #e8f4fd; padding: 15px; border-radius: 5px; margin: 20px 0; }
        .success { color: #28a745; font-weight: bold; }
    </style>
</head>
<body>
    <div class="container">
        <h1>🎉 Bore Go Client 测试成功！</h1>
        <div class="info">
            <p class="success">✅ 你的本地服务已经通过bore成功暴露到公网！</p>
            <p><strong>请求时间:</strong> %s</p>
            <p><strong>请求路径:</strong> %s</p>
            <p><strong>用户代理:</strong> %s</p>
        </div>
        <p>这个页面是通过Go实现的bore客户端代理的本地HTTP服务。</p>
        <p>如果你能看到这个页面，说明bore隧道工作正常！</p>
    </div>
</body>
</html>
		`, time.Now().Format("2006-01-02 15:04:05"), r.URL.Path, r.UserAgent())
	})

	server := &http.Server{
		Addr:         ":8080",
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
	}

	log.Println("🌐 本地HTTP服务器启动在 :8080")
	if err := server.ListenAndServe(); err != nil {
		log.Printf("本地服务器启动失败: %v", err)
	}
}
