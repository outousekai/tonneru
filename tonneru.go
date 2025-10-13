package tonneru

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log/slog"
	"net"
	"sync"
	"time"

	"github.com/google/uuid"
)

// 常量定义
const (
	ControlPort    = 7837
	MaxFrameLength = 512
	NetworkTimeout = 10 * time.Second
)

// ClientConfig 客户端配置
type ClientConfig struct {
	LocalHost  string        // 本地服务地址
	LocalPort  int           // 本地服务端口
	RemoteHost string        // 远程服务器地址
	RemotePort int           // 0表示让服务器分配端口
	Secret     string        // 认证密钥（可选）
	BindIP     string        // 服务器绑定IP（可选）
	RetryCount int           // 重连次数，0表示无限重连
	RetryDelay time.Duration // 重连延迟时间，最低1秒
	Logger     *slog.Logger  // 日志记录器（可选）
	// 缓冲区配置
	BufferSize    int           // 缓冲区大小，默认64KB
	ReadTimeout   time.Duration // 读取超时时间
	WriteTimeout  time.Duration // 写入超时时间
	KeepAlive     bool          // 是否启用TCP Keep-Alive
	KeepAliveTime time.Duration // Keep-Alive间隔时间
}

// Client 客户端结构体
type Client struct {
	config     ClientConfig
	conn       net.Conn
	remotePort int
	auth       *Authenticator
	mu         sync.RWMutex
	retryCount int // 当前重连次数
}

// NewClient 创建新的客户端
func NewClient(config ClientConfig) (*Client, error) {
	// 设置默认值
	if config.RetryDelay == 0 {
		config.RetryDelay = 1 * time.Second
	} else if config.RetryDelay < 1*time.Second {
		config.RetryDelay = 1 * time.Second
	}

	// 设置缓冲区默认值
	if config.BufferSize == 0 {
		config.BufferSize = 256 * 1024 // 64KB
	}
	if config.ReadTimeout == 0 {
		config.ReadTimeout = 30 * time.Second
	}
	if config.WriteTimeout == 0 {
		config.WriteTimeout = 30 * time.Second
	}
	if config.KeepAliveTime == 0 {
		config.KeepAliveTime = 30 * time.Second
	}

	client := &Client{
		config: config,
	}

	if config.Secret != "" {
		client.auth = NewAuthenticator(config.Secret)
	}

	return client, nil
}

// logInfo 记录信息日志
func (c *Client) logInfo(msg string, args ...any) {
	if c.config.Logger != nil {
		c.config.Logger.Info(msg, args...)
	}
}

// logError 记录错误日志
func (c *Client) logError(msg string, args ...any) {
	if c.config.Logger != nil {
		c.config.Logger.Error(msg, args...)
	}
}

// logDebug 记录调试日志
func (c *Client) logDebug(msg string, args ...any) {
	if c.config.Logger != nil {
		c.config.Logger.Debug(msg, args...)
	}
}

// Connect 连接到服务器（带重连机制）
func (c *Client) Connect(ctx context.Context) error {
	for {
		// 检查重连次数限制
		if c.config.RetryCount > 0 && c.retryCount >= c.config.RetryCount {
			return fmt.Errorf("已达到最大重连次数 %d", c.config.RetryCount)
		}

		// 尝试连接
		err := c.connectOnce()
		if err == nil {
			// 连接成功，重置重连计数
			c.mu.Lock()
			c.retryCount = 0
			c.mu.Unlock()
			return nil
		}

		// 连接失败，增加重连计数
		c.mu.Lock()
		c.retryCount++
		c.mu.Unlock()

		c.logError("连接失败", "attempt", c.retryCount, "error", err)

		// 等待重连延迟
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(c.config.RetryDelay):
			continue
		}
	}
}

// connectOnce 执行单次连接
func (c *Client) connectOnce() error {
	// 连接到控制端口
	conn, err := net.DialTimeout("tcp", fmt.Sprintf("%s:%d", c.config.RemoteHost, ControlPort), NetworkTimeout)
	if err != nil {
		return fmt.Errorf("无法连接到服务器: %w", err)
	}

	c.conn = conn

	// 进行认证握手
	if c.auth != nil {
		if err := c.auth.ClientHandshake(c.conn); err != nil {
			conn.Close()
			return fmt.Errorf("认证失败: %w", err)
		}
	}

	// 发送Hello消息
	var helloData HelloMsg
	if c.config.BindIP != "" {
		helloData = HelloMsg{c.config.BindIP, c.config.RemotePort}
	} else {
		helloData = HelloMsg{nil, c.config.RemotePort}
	}
	helloMsg := ClientMessage{
		Hello: &helloData,
	}

	if err := c.sendMessage(helloMsg); err != nil {
		conn.Close()
		return fmt.Errorf("发送Hello消息失败: %w", err)
	}

	// 接收服务器响应
	var serverMsg ServerMessage
	if err := c.recvMessage(&serverMsg); err != nil {
		conn.Close()
		return fmt.Errorf("接收服务器响应失败: %w", err)
	}

	if serverMsg.Hello != nil {
		c.remotePort = *serverMsg.Hello
		c.logInfo("连接成功", "local", fmt.Sprintf("%s:%d", c.config.LocalHost, c.config.LocalPort), "remote", fmt.Sprintf("%s:%d", c.config.RemoteHost, c.remotePort), "bind_ip", c.config.BindIP)
	} else if serverMsg.Error != nil {
		conn.Close()
		return fmt.Errorf("服务器错误: %s", *serverMsg.Error)
	} else if serverMsg.Challenge != nil {
		conn.Close()
		return fmt.Errorf("服务器需要认证，但未提供密钥")
	} else {
		conn.Close()
		return fmt.Errorf("意外的服务器消息")
	}

	return nil
}

// Listen 开始监听连接
func (c *Client) Listen(ctx context.Context) error {
	defer c.Close()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
			// 设置读取超时
			c.conn.SetReadDeadline(time.Now().Add(NetworkTimeout))

			var serverMsg ServerMessage
			if err := c.recvMessage(&serverMsg); err != nil {
				if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
					continue
				}

				// 连接断开，尝试重连
				c.logError("连接断开，尝试重连", "error", err)
				if err := c.reconnect(ctx); err != nil {
					return fmt.Errorf("重连失败: %w", err)
				}
				continue
			}

			if serverMsg.Heartbeat != nil {
				// 心跳消息，继续监听
				continue
			} else if serverMsg.Connection != nil {
				// 新连接请求
				go c.handleConnection(*serverMsg.Connection)
			} else if serverMsg.Error != nil {
				// 服务器错误，尝试重连
				c.logError("服务器错误，尝试重连", "error", *serverMsg.Error)
				if err := c.reconnect(ctx); err != nil {
					return fmt.Errorf("重连失败: %w", err)
				}
				continue
			} else {
				c.logDebug("意外的服务器消息", "message", serverMsg)
			}
		}
	}
}

// reconnect 重连到服务器
func (c *Client) reconnect(ctx context.Context) error {
	// 关闭当前连接
	if c.conn != nil {
		c.conn.Close()
		c.conn = nil
	}

	// 使用Connect方法重连（会自动处理重连逻辑）
	return c.Connect(ctx)
}

// handleConnection 处理单个代理连接
func (c *Client) handleConnection(connectionID uuid.UUID) {
	// 连接到控制端口
	conn, err := net.DialTimeout("tcp", fmt.Sprintf("%s:%d", c.config.RemoteHost, ControlPort), NetworkTimeout)
	if err != nil {
		c.logError("无法连接到服务器进行代理", "error", err)
		return
	}
	defer conn.Close()

	// 进行认证握手
	if c.auth != nil {
		if err := c.auth.ClientHandshake(conn); err != nil {
			c.logError("代理连接认证失败", "error", err)
			return
		}
	}

	// 发送Accept消息
	acceptMsg := ClientMessage{
		Accept: &connectionID,
	}

	if err := c.sendMessageToConn(conn, acceptMsg); err != nil {
		c.logError("发送Accept消息失败", "error", err)
		return
	}

	// 连接到本地服务
	localConn, err := net.DialTimeout("tcp", fmt.Sprintf("%s:%d", c.config.LocalHost, c.config.LocalPort), NetworkTimeout)
	if err != nil {
		c.logError("无法连接到本地服务", "error", err)
		return
	}
	defer localConn.Close()

	c.logDebug("新连接建立，开始代理数据")

	// 开始代理数据
	c.proxy(localConn, conn)
}

// proxy 代理数据流（改进版本，支持缓冲区管理）
func (c *Client) proxy(conn1, conn2 net.Conn) {
	var wg sync.WaitGroup
	wg.Add(2)

	// 设置TCP Keep-Alive
	if c.config.KeepAlive {
		if tcpConn1, ok := conn1.(*net.TCPConn); ok {
			tcpConn1.SetKeepAlive(true)
			tcpConn1.SetKeepAlivePeriod(c.config.KeepAliveTime)
		}
		if tcpConn2, ok := conn2.(*net.TCPConn); ok {
			tcpConn2.SetKeepAlive(true)
			tcpConn2.SetKeepAlivePeriod(c.config.KeepAliveTime)
		}
	}

	// 从conn1复制到conn2（带缓冲区管理）
	go func() {
		defer wg.Done()
		defer conn2.Close()

		buffer := make([]byte, c.config.BufferSize)
		for {
			// 设置读取超时
			conn1.SetReadDeadline(time.Now().Add(c.config.ReadTimeout))

			n, err := conn1.Read(buffer)
			if err != nil {
				if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
					// 超时，继续读取
					continue
				}
				c.logDebug("连接1读取结束", "error", err)
				return
			}

			// 设置写入超时
			conn2.SetWriteDeadline(time.Now().Add(c.config.WriteTimeout))

			_, err = conn2.Write(buffer[:n])
			if err != nil {
				c.logError("连接2写入失败", "error", err)
				return
			}
		}
	}()

	// 从conn2复制到conn1（带缓冲区管理）
	go func() {
		defer wg.Done()
		defer conn1.Close()

		buffer := make([]byte, c.config.BufferSize)
		for {
			// 设置读取超时
			conn2.SetReadDeadline(time.Now().Add(c.config.ReadTimeout))

			n, err := conn2.Read(buffer)
			if err != nil {
				if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
					// 超时，继续读取
					continue
				}
				c.logDebug("连接2读取结束", "error", err)
				return
			}

			// 设置写入超时
			conn1.SetWriteDeadline(time.Now().Add(c.config.WriteTimeout))

			_, err = conn1.Write(buffer[:n])
			if err != nil {
				c.logError("连接1写入失败", "error", err)
				return
			}
		}
	}()

	wg.Wait()
	c.logDebug("代理连接结束")
}

// GetRemotePort 获取远程端口
func (c *Client) GetRemotePort() int {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.remotePort
}

// Close 关闭连接
func (c *Client) Close() error {
	if c.conn != nil {
		return c.conn.Close()
	}
	return nil
}

// sendMessage 发送消息到控制连接
func (c *Client) sendMessage(msg interface{}) error {
	return c.sendMessageToConn(c.conn, msg)
}

// sendMessageToConn 发送消息到指定连接
func (c *Client) sendMessageToConn(conn net.Conn, msg interface{}) error {
	data, err := json.Marshal(msg)
	if err != nil {
		return err
	}

	// 添加null分隔符
	data = append(data, 0)

	_, err = conn.Write(data)
	return err
}

// recvMessage 接收消息
func (c *Client) recvMessage(msg interface{}) error {
	// 读取直到遇到null分隔符
	var data []byte
	buffer := make([]byte, 1)

	for {
		_, err := c.conn.Read(buffer)
		if err != nil {
			return err
		}

		if buffer[0] == 0 {
			break
		}

		data = append(data, buffer[0])
	}

	// 调试信息已移除

	return json.Unmarshal(data, msg)
}

// ClientMessage 客户端消息 - 使用联合类型来匹配Rust的枚举
type ClientMessage struct {
	Authenticate *string    `json:"Authenticate,omitempty"`
	Hello        *HelloMsg  `json:"Hello,omitempty"`
	Accept       *uuid.UUID `json:"Accept,omitempty"`
}

// HelloMsg Hello消息的内容 - 匹配Rust的元组结构
type HelloMsg []interface{}

// ServerMessage 服务器消息 - 使用联合类型来匹配Rust的枚举
type ServerMessage struct {
	Challenge  *uuid.UUID `json:"Challenge,omitempty"`
	Hello      *int       `json:"Hello,omitempty"`
	Heartbeat  *string    `json:"Heartbeat,omitempty"`
	Connection *uuid.UUID `json:"Connection,omitempty"`
	Error      *string    `json:"Error,omitempty"`
}

// UnmarshalJSON 自定义JSON解析
func (s *ServerMessage) UnmarshalJSON(data []byte) error {
	// 首先尝试解析为字符串（用于Heartbeat）
	var str string
	if err := json.Unmarshal(data, &str); err == nil {
		if str == "Heartbeat" {
			s.Heartbeat = &str
			return nil
		}
	}

	// 尝试解析为对象
	var obj map[string]interface{}
	if err := json.Unmarshal(data, &obj); err != nil {
		return err
	}

	// 检查各个字段
	if challenge, ok := obj["Challenge"].(string); ok {
		if uuid, err := uuid.Parse(challenge); err == nil {
			s.Challenge = &uuid
		}
	}
	if hello, ok := obj["Hello"].(float64); ok {
		port := int(hello)
		s.Hello = &port
	}
	if connection, ok := obj["Connection"].(string); ok {
		if uuid, err := uuid.Parse(connection); err == nil {
			s.Connection = &uuid
		}
	}
	if error, ok := obj["Error"].(string); ok {
		s.Error = &error
	}

	return nil
}

// Authenticator 认证器
type Authenticator struct {
	secret []byte
}

// NewAuthenticator 创建认证器
func NewAuthenticator(secret string) *Authenticator {
	hash := sha256.Sum256([]byte(secret))
	return &Authenticator{
		secret: hash[:],
	}
}

// ClientHandshake 客户端认证握手
func (a *Authenticator) ClientHandshake(conn net.Conn) error {
	// 接受连接 - 使用自定义的recvMessage方法
	var serverMsg ServerMessage
	if err := a.recvMessage(conn, &serverMsg); err != nil {
		return fmt.Errorf("接收连接失败: %w", err)
	}

	// 调试信息已移除

	if serverMsg.Challenge == nil {
		return fmt.Errorf("期望连接消息，但收到其他消息")
	}

	// 生成响应
	response := a.answerChallenge(*serverMsg.Challenge)

	// 发送认证响应
	authMsg := ClientMessage{
		Authenticate: &response,
	}

	data, err := json.Marshal(authMsg)
	if err != nil {
		return err
	}

	data = append(data, 0)
	_, err = conn.Write(data)
	return err
}

// recvMessage 接收消息的辅助方法
func (a *Authenticator) recvMessage(conn net.Conn, msg interface{}) error {
	// 设置读取超时
	conn.SetReadDeadline(time.Now().Add(NetworkTimeout))

	// 读取直到遇到null分隔符
	var data []byte
	buffer := make([]byte, 1)

	for {
		_, err := conn.Read(buffer)
		if err != nil {
			return err
		}

		if buffer[0] == 0 {
			break
		}

		data = append(data, buffer[0])
	}

	return json.Unmarshal(data, msg)
}

// answerChallenge 回答连接
func (a *Authenticator) answerChallenge(challenge uuid.UUID) string {
	h := hmac.New(sha256.New, a.secret)
	h.Write(challenge[:])
	return hex.EncodeToString(h.Sum(nil))
}

// WithBufferConfig 设置缓冲区配置
func (c *ClientConfig) WithBufferConfig(bufferSize int, readTimeout, writeTimeout time.Duration) *ClientConfig {
	c.BufferSize = bufferSize
	c.ReadTimeout = readTimeout
	c.WriteTimeout = writeTimeout
	return c
}

// WithKeepAlive 设置TCP Keep-Alive
func (c *ClientConfig) WithKeepAlive(keepAliveTime time.Duration) *ClientConfig {
	c.KeepAlive = true
	c.KeepAliveTime = keepAliveTime
	return c
}

// WithLogger 设置日志记录器
func (c *ClientConfig) WithLogger(logger *slog.Logger) *ClientConfig {
	c.Logger = logger
	return c
}
