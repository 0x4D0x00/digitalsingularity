package websocket

import (
	"crypto/rsa"
	"crypto/x509"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"digitalsingularity/backend/common/auth/tokenmanage"
	"digitalsingularity/backend/common/utils/datahandle"
	"digitalsingularity/backend/silicoid/interceptor"
	"digitalsingularity/backend/main/accountmanagement/apikeymanage"
	speechinterceptor "digitalsingularity/backend/speechsystem/interceptor"
)

// 全局日志器和服务
var logger *log.Logger
var authTokenService *tokenmanage.CommonAuthTokenService
var websocketInterceptorService *interceptor.SilicoIDInterceptor
var apiKeyManageService *apikeymanage.ApiKeyManageService
var readWrite *datahandle.CommonReadWriteService

// ConnectionManager WebSocket连接管理器
type ConnectionManager struct {
	connections map[*websocket.Conn]bool
	register    chan *websocket.Conn
	unregister  chan *websocket.Conn
	broadcast   chan []byte
	mutex       sync.Mutex
}

var connectionManager *ConnectionManager

// NewConnectionManager 创建连接管理器
func NewConnectionManager() *ConnectionManager {
	return &ConnectionManager{
		connections: make(map[*websocket.Conn]bool),
		register:    make(chan *websocket.Conn),
		unregister:  make(chan *websocket.Conn),
		broadcast:   make(chan []byte, 256),
	}
}

// Start 启动连接管理器
func (cm *ConnectionManager) Start() {
	for {
		select {
		case conn := <-cm.register:
			cm.mutex.Lock()
			cm.connections[conn] = true
			cm.mutex.Unlock()
			logger.Printf("WebSocket连接已注册，当前连接数: %d", len(cm.connections))
		case conn := <-cm.unregister:
			cm.mutex.Lock()
			if _, ok := cm.connections[conn]; ok {
				delete(cm.connections, conn)
				conn.Close()
			}
			cm.mutex.Unlock()
			logger.Printf("WebSocket连接已注销，当前连接数: %d", len(cm.connections))
		case message := <-cm.broadcast:
			cm.mutex.Lock()
			for conn := range cm.connections {
				if err := conn.WriteMessage(websocket.TextMessage, message); err != nil {
					logger.Printf("广播消息失败: %v", err)
					delete(cm.connections, conn)
					conn.Close()
				}
			}
			cm.mutex.Unlock()
		}
	}
}

// BroadcastMessage 广播消息给所有连接的客户端
func (cm *ConnectionManager) BroadcastMessage(messageType string, data interface{}) {
	message := map[string]interface{}{
		"type": messageType,
		"data": data,
	}
	messageBytes, err := json.Marshal(message)
	if err != nil {
		logger.Printf("序列化广播消息失败: %v", err)
		return
	}
	cm.broadcast <- messageBytes
}

// RegisterConnection 注册连接
func (cm *ConnectionManager) RegisterConnection(conn *websocket.Conn) {
	cm.register <- conn
}

// UnregisterConnection 注销连接
func (cm *ConnectionManager) UnregisterConnection(conn *websocket.Conn) {
	cm.unregister <- conn
}

// Server Call 调用配置
const maxServerCallIterations = 50 // 最大服务端调用次数，防止无限循环

// ServerCall 表示一个服务端调用（MCP 或 Executor）
type ServerCall struct {
	Type      string                 `json:"type"`      // 调用类型标识（用于序列化）；实际是否由服务端执行由工具注册表的 execution_type 决定
	Name      string                 `json:"name"`      // 工具名称或执行器名称
	Arguments map[string]interface{} `json:"arguments"` // 参数
}

// 工具的执行位置（server/client）应通过工具表中的 execution_type 字段判断，模型侧应使用标准 `function_call`/`tool_calls` 格式

// parsePublicKey 解析公钥字符串，支持多种格式
// 支持: 16进制编码的PEM格式 和 直接的PEM格式
func parsePublicKey(publicKeyStr string) (*rsa.PublicKey, error) {
	var block *pem.Block
	var publicKeyBytes []byte
	
	// 先尝试直接PEM格式
	block, _ = pem.Decode([]byte(publicKeyStr))
	if block != nil {
		logger.Printf("公钥是直接PEM格式")
		publicKeyBytes = block.Bytes
	} else {
		// 尝试16进制解码（可能是16进制编码的PEM）
		logger.Printf("公钥不是直接PEM格式，尝试16进制解码")
		decodedBytes, err := hex.DecodeString(publicKeyStr)
		if err != nil {
			return nil, fmt.Errorf("16进制解码失败: %v", err)
		}
		
		// 16进制解码后，再尝试PEM解析
		block, _ = pem.Decode(decodedBytes)
		if block != nil {
			logger.Printf("16进制解码后得到PEM格式公钥")
			publicKeyBytes = block.Bytes
		} else {
			logger.Printf("16进制解码后不是PEM格式，直接使用DER字节")
			publicKeyBytes = decodedBytes
		}
	}
	
	// 解析公钥
	parsedKey, err := x509.ParsePKIXPublicKey(publicKeyBytes)
	if err != nil {
		return nil, fmt.Errorf("解析公钥失败: %v", err)
	}
	
	// 确保是RSA公钥
	rsaPublicKey, ok := parsedKey.(*rsa.PublicKey)
	if !ok {
		return nil, fmt.Errorf("公钥不是RSA类型")
	}
	
	logger.Printf("公钥解析成功")
	return rsaPublicKey, nil
}

// rwAdapter 适配器，用于将datahandle.ReadWriteService转换为auth/tokenmanage所需的接口
type rwAdapter struct {
	rw *datahandle.CommonReadWriteService
}

func (a *rwAdapter) GetRedis(key string) string {
	result := a.rw.GetRedis(key)
	if !result.IsSuccess() {
		return ""
	}
	if val, ok := result.Data.(string); ok {
		return val
	}
	return ""
}

func (a *rwAdapter) SetRedis(key string, value string, expire int) error {
	result := a.rw.SetRedis(key, value, time.Duration(expire)*time.Second)
	if !result.IsSuccess() {
		return result.Error
	}
	return nil
}

func (a *rwAdapter) DeleteRedis(key string) error {
	result := a.rw.DeleteRedis(key)
	if !result.IsSuccess() {
		return result.Error
	}
	return nil
}

// 初始化日志器和服务
func init() {
	// 初始化日志器
	logger = log.New(log.Writer(), "[WebSocket] ", log.LstdFlags)
	
	// 初始化数据服务
	var err error
	readWrite, err = datahandle.NewCommonReadWriteService("database")
	if err != nil {
		logger.Printf("初始化数据服务失败: %v，部分功能可能不可用", err)
		// 不直接返回，继续初始化其他服务（如语音服务有自己的错误处理）
	} else {
		// 创建服务适配器并初始化authTokenService
		rwAdapterInst := &rwAdapter{rw: readWrite}
		authTokenService = tokenmanage.NewCommonAuthTokenService(rwAdapterInst)
		
		// 初始化API密钥管理服务
		apiKeyManageService = apikeymanage.NewApiKeyManageService(authTokenService)
	}
	
	// 初始化 WebSocket 专用的 Interceptor 服务（与HTTP服务隔离）
	websocketInterceptorService = interceptor.CreateInterceptor()
	
	// 初始化语音服务（即使数据库连接失败，也尝试初始化，因为它有自己的错误处理）
	if err := initSpeechService(); err != nil {
		logger.Printf("语音服务初始化失败: %v", err)
	}
	
	// 初始化连接管理器
	connectionManager = NewConnectionManager()
	go connectionManager.Start()
	logger.Printf("WebSocket连接管理器已启动")
	
	// 初始化通知服务（在线和离线通知）
	InitNotificationService(logger)
	logger.Printf("通知服务已初始化（在线和离线）")
	
	// 注册角色更新广播函数（避免循环导入）
	registerRolesBroadcastFunction()
	
	logger.Printf("WebSocket服务初始化完成")
}

// initSpeechService 初始化语音服务
// 验证语音拦截器服务是否可用，并记录初始化状态
func initSpeechService() error {
	// 创建语音拦截器服务实例进行测试（注意：这是speechsystem的拦截器，不是silicoid的拦截器）
	interceptorService := speechinterceptor.NewInterceptorService()
	testInterceptor, err := interceptorService.CreateInterceptor()
	if err != nil {
		return fmt.Errorf("无法创建语音拦截器: %v", err)
	}
	
	// 记录成功信息
	logger.Printf("语音服务初始化成功，提供商: %s", testInterceptor.GetProvider())
	
	// 注意：这里创建的是测试实例，实际使用时会在每个会话中创建新的拦截器实例
	return nil
}

// WebSocket连接升级器
var upgrader = websocket.Upgrader{
	ReadBufferSize:  4096,  // 增加读缓冲区以支持更大的消息
	WriteBufferSize: 4096,  // 增加写缓冲区以支持流式传输
	CheckOrigin: func(r *http.Request) bool {
		// 允许所有来源的WebSocket连接
		// 注意：实际生产环境应该限制来源
		return true
	},
	// 不设置 HandshakeTimeout，允许慢速连接
	// 不设置读写超时，支持长时间任务
}

// 处理WebSocket连接
func handleConnection(w http.ResponseWriter, r *http.Request) {
	// 升级HTTP连接为WebSocket
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		logger.Printf("升级为WebSocket连接失败: %v", err)
		return
	}
	defer conn.Close()

	// 客户端地址
	clientAddr := conn.RemoteAddr().String()
	logger.Printf("新的WebSocket连接: %s", clientAddr)

	// 处理客户端消息
	for {
		messageType, message, err := conn.ReadMessage()
		if err != nil {
			logger.Printf("读取消息失败: %v", err)
			break
		}

		// 记录收到的消息
		logger.Printf("收到消息(%s): %s", clientAddr, string(message))

		// 处理消息并回复
		response := fmt.Sprintf("服务器已收到消息: %s", string(message))
		
		// 发送回复
		if err := conn.WriteMessage(messageType, []byte(response)); err != nil {
			logger.Printf("发送消息失败: %v", err)
			break
		}
	}

	logger.Printf("WebSocket连接已关闭: %s", clientAddr)
}


// 获取当前时间戳
func getCurrentTimestamp() int64 {
	return time.Now().Unix()
}

// generateRandomString 生成指定长度的随机字符串
func generateRandomString(length int) string {
	const charset = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	b := make([]byte, length)
	for i := range b {
		b[i] = charset[time.Now().UnixNano()%int64(len(charset))]
	}
	return string(b)
}

// min 辅助函数
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}


// getUserActiveToken 从Redis中获取用户的活跃token
func getUserActiveToken(userID string, readWrite *datahandle.CommonReadWriteService) string {
	// 获取用户的活跃token列表
	userTokensKey := fmt.Sprintf("user:tokens:%s", userID)
	opResult := readWrite.GetRedis(userTokensKey)
	
	if !opResult.IsSuccess() {
		logger.Printf("获取用户 %s 的token列表失败: %v", userID, opResult.Error)
		return ""
	}
	
	userTokensStr, ok := opResult.Data.(string)
	if !ok || userTokensStr == "" {
		logger.Printf("用户 %s 没有活跃的token", userID)
		return ""
	}
	
	// 解析token列表
	var tokensList []string
	err := json.Unmarshal([]byte(userTokensStr), &tokensList)
	if err != nil || len(tokensList) == 0 {
		logger.Printf("解析用户 %s 的token列表失败: %v", userID, err)
		return ""
	}
	
	// 获取第一个（最新的）token的JTI
	jti := tokensList[0]
	tokenKey := fmt.Sprintf("token:%s", jti)
	
	// 获取token详细信息
	tokenOpResult := readWrite.GetRedis(tokenKey)
	if !tokenOpResult.IsSuccess() {
		logger.Printf("获取token %s 详细信息失败: %v", jti, tokenOpResult.Error)
		return ""
	}
	
	tokenDataStr, ok := tokenOpResult.Data.(string)
	if !ok || tokenDataStr == "" {
		logger.Printf("token %s 不存在或已过期", jti)
		return ""
	}
	
	// 解析token数据
	var tokenData map[string]interface{}
	err = json.Unmarshal([]byte(tokenDataStr), &tokenData)
	if err != nil {
		logger.Printf("解析token %s 数据失败: %v", jti, err)
		return ""
	}
	
	// 检查token状态
	status, ok := tokenData["status"].(string)
	if !ok || status != "active" {
		logger.Printf("token %s 状态不是active: %s", jti, status)
		return ""
	}
	
	// 返回完整的token
	token, ok := tokenData["token"].(string)
	if !ok {
		logger.Printf("token %s 中没有token字段", jti)
		return ""
	}
	
	logger.Printf("成功获取用户 %s 的活跃token", userID)
	return token
}

// registerRolesBroadcastFunction 注册角色更新广播函数
// 这个函数用于设置http包中的函数指针，以便http包可以调用websocket的广播功能
func registerRolesBroadcastFunction() {
	// 广播功能通过BroadcastRolesUpdate函数实现
	// http包可以直接导入websocket包来调用该函数
}

// HandleConnection 启动WebSocket服务
func HandleConnection(host string, port int, debug bool) {
	// 设置WebSocket路由
	http.HandleFunc("/ws", handleConnection)
	http.HandleFunc("/api/mainservice", handleMainServiceConnection)
	http.HandleFunc("/api/speechinteractive", handleMainServiceConnection) // 语音交互使用相同的处理函数
	
	// 创建服务地址
	addr := fmt.Sprintf("%s:%d", host, port)
	
	// 打印启动信息
	logger.Printf("WebSocket服务器启动于 %s", addr)
	logger.Printf("WebSocket路由: /ws, /api/mainservice, /api/speechinteractive")
	if debug {
		logger.Printf("调试模式已启用")
	}
	
	// 启动服务器
	if err := http.ListenAndServe(addr, nil); err != nil {
		logger.Fatalf("WebSocket服务器启动失败: %v", err)
	}
} 