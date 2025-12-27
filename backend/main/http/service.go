package http

import (
	"bufio"
	"crypto/rsa"
	"crypto/x509"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"time"

	"github.com/gin-gonic/gin"

	aibasicplatformdatabase "digitalsingularity/backend/aibasicplatform/database"
	"digitalsingularity/backend/common/auth/nonce"
	"digitalsingularity/backend/common/auth/smsverify"
	asymmetricdecrypt "digitalsingularity/backend/common/security/asymmetricencryption/decrypt"
	asymmetricencrypt "digitalsingularity/backend/common/security/asymmetricencryption/encrypt"
	symmetricdecrypt "digitalsingularity/backend/common/security/symmetricencryption/decrypt"
	symmetricencrypt "digitalsingularity/backend/common/security/symmetricencryption/encrypt"
	"digitalsingularity/backend/common/auth/tokenmanage"
	"digitalsingularity/backend/common/userinfostorage"
	"digitalsingularity/backend/common/utils/datahandle"
	
	"digitalsingularity/backend/main/accountmanagement/apikeymanage"
	"digitalsingularity/backend/main/accountmanagement/login"
	
	
	"digitalsingularity/backend/silicoid"
	silicoiddatabase "digitalsingularity/backend/silicoid/database"
	silicoidtrainingdata "digitalsingularity/backend/silicoid/trainingdata"
	
)

// 全局日志器
var logger *log.Logger

// 速率限制相关
type rateLimitInfo struct {
	requests  int
	lastReset time.Time
}

var (
	rateLimits = make(map[string]rateLimitInfo)
	limiterMux sync.Mutex
)

// 服务实例
var (
	readWrite                        *datahandle.CommonReadWriteService
	loginService                     *login.LoginService
	nonceService                     *nonce.NonceService
	httpInterceptor                  *silicoid.Interceptor
	
	
	
	apiKeyManageService              *apikeymanage.ApiKeyManageService
	authTokenService                 *tokenmanage.CommonAuthTokenService
	silicoidService                  *silicoiddatabase.SilicoidDataService
	aiBasicPlatformDataService       *aibasicplatformdatabase.AIBasicPlatformDataService
	trainingDataService              *silicoidtrainingdata.TrainingDataService
)

// 创建服务适配器以匹配tokenmanage和userinfostorage所需的接口
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
	// 初始化日志器 - 同时输出到标准输出和http_requests.log文件
	var logWriter io.Writer = log.Writer()

	// 获取日志目录（优先使用环境变量）
	var logDir string
	if envLogDir := os.Getenv("DIGITALSINGULARITY_LOG_DIR"); envLogDir != "" {
		logDir = envLogDir
	} else {
		// 尝试获取用户主目录
		if homeDir, err := os.UserHomeDir(); err == nil {
			if homeDir == "/root" {
				logDir = "/var/log/digitalsingularity_logs"
			} else {
				logDir = filepath.Join(homeDir, "digitalsingularity_logs")
			}
		} else {
			logDir = "/var/log/digitalsingularity_logs"
		}
	}

	// 创建日志目录
	os.MkdirAll(logDir, 0755)

	// 尝试打开http_requests.log文件
	httpLogFile, err := os.OpenFile(
		filepath.Join(logDir, "http_requests.log"),
		os.O_CREATE|os.O_APPEND|os.O_WRONLY,
		0644,
	)
	if err == nil {
		// 同时输出到标准输出和文件
		logWriter = io.MultiWriter(log.Writer(), httpLogFile)
	}

	// 使用带时间戳和文件位置的日志格式
	logFlags := log.Ldate | log.Ltime | log.Lmicroseconds | log.Lshortfile
	logger = log.New(logWriter, "[HTTP] ", logFlags)

	// 初始化服务
	readWrite, err = datahandle.NewCommonReadWriteService("database")
	if err != nil {
		logger.Printf("初始化数据服务失败: %v", err)
	}

	// 创建服务适配器以匹配auth/tokenmanage和userinfostorage所需的接口
	rwAdapterInst := &rwAdapter{rw: readWrite}
	authTokenService = tokenmanage.NewCommonAuthTokenService(rwAdapterInst)

	// 为对称加密/解密服务创建包装函数
	encryptFunc := func(plainText string) (string, error) {
		return symmetricencrypt.SymmetricEncryptService(plainText, "", "")
	}
	decryptFunc := func(cipherText string) (string, error) {
		return symmetricdecrypt.SymmetricDecryptService(cipherText, "", "")
	}

	userCacheService := userinfostorage.NewCommonUserCacheService(rwAdapterInst, encryptFunc, decryptFunc)
	smsVerifyService, _ := smsverify.NewSmsVerifyService()
	silicoidService = silicoiddatabase.NewSilicoidDataService(readWrite)
	aiBasicPlatformDataService = aibasicplatformdatabase.NewAIBasicPlatformDataService(readWrite)
	aiBasicPlatformService := aibasicplatformdatabase.NewAiBasicPlatformLoginService()

	loginService = login.NewLoginService(
		readWrite,
		authTokenService,
		userCacheService,
		smsVerifyService,
		nil,
		nil,
		aiBasicPlatformService,
	)
	nonceService, _ = nonce.NewNonceService()
	httpInterceptor = silicoid.CreateInterceptor()

	// 初始化API密钥管理服务（apikey/service.go 会自己处理数据库连接）
	apiKeyManageService = apikeymanage.NewApiKeyManageService(authTokenService)
	logger.Printf("API密钥管理服务初始化成功")

	// 清空Redis中所有system_prompt相关键（在加载前先清空）
	if err := readWrite.ClearSystemPromptKeys(); err != nil {
		logger.Printf("清空system_prompt相关键失败: %v", err)
	} else {
		logger.Printf("已清空Redis中所有system_prompt相关键")
	}

	// 初始化系统提示词数据并加载到Redis（来自AiBasicPlatform库）
	if aiBasicPlatformDataService == nil {
		logger.Printf("AIBasicPlatform数据服务未初始化，无法加载系统提示词到Redis")
	} else if err := aiBasicPlatformDataService.LoadPromptsToRedis(); err != nil {
		logger.Printf("加载AIBasicPlatform系统提示词到Redis失败: %v", err)
	} else {
		logger.Printf("AIBasicPlatform系统提示词已成功加载到Redis")
	}

	// 在启动时预加载常用翻译到 Redis，便于登录时快速返回给前端
	// 包含 StorageBox、综合安全测试、Chrome DevTools、扫描配置和系统提示的角色名称/类型（中英文）
	preloadTranslations := []struct {
		App      string
		Category string
		Locale   string
	}{
		// system_prompt
		{"system_prompt", "role_name", "zh-CN"},
		{"system_prompt", "role_name", "en-US"},
		{"system_prompt", "role_type", "zh-CN"},
		{"system_prompt", "role_type", "en-US"},
	}

	for _, cfg := range preloadTranslations {
		if aiBasicPlatformDataService == nil {
			logger.Printf("AIBasicPlatform数据服务未初始化，无法预加载翻译 app=%s category=%s locale=%s",
				cfg.App, cfg.Category, cfg.Locale)
			continue
		}

		if err := aiBasicPlatformDataService.LoadTranslationsToRedis(cfg.App, cfg.Category, cfg.Locale); err != nil {
			logger.Printf("预加载翻译到 Redis 失败 app=%s category=%s locale=%s: %v",
				cfg.App, cfg.Category, cfg.Locale, err)
		} else {
			logger.Printf("已预加载翻译到 Redis app=%s category=%s locale=%s",
				cfg.App, cfg.Category, cfg.Locale)
		}
	}

	// 初始化训练数据服务
	trainingDataService = silicoidtrainingdata.NewTrainingDataService(silicoidService)
	
	logger.Printf("训练数据服务初始化成功")
}

// 速率限制中间件
func rateLimit(next http.Handler, limit int, per int) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ip := r.RemoteAddr

		limiterMux.Lock()
		defer limiterMux.Unlock()

		now := time.Now()

		// 获取当前IP的速率限制数据
		info, exists := rateLimits[ip]
		if !exists || now.Sub(info.lastReset).Seconds() > float64(per) {
			info = rateLimitInfo{
				requests:  0,
				lastReset: now,
			}
		}

		// 更新请求次数
		info.requests++
		rateLimits[ip] = info

		// 检查是否超过限制
		if info.requests > limit {
			logger.Printf("请求限制: IP %s 在 %d秒内发送了 %d 个请求，超过限制 %d", ip, per, info.requests, limit)
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusTooManyRequests)
			json.NewEncoder(w).Encode(map[string]string{
				"status":  "error",
				"message": "请求过于频繁",
			})
			return
		}

		next.ServeHTTP(w, r)
	})
}

// 处理加密请求
func handleEncryptedRequest(w http.ResponseWriter, r *http.Request) {
	// 记录请求开始
	requestID := strconv.FormatInt(time.Now().UnixNano()/1e6, 10)
	clientIP := r.RemoteAddr
	logger.Printf("[%s] 收到来自 %s 的加密请求", requestID, clientIP)

	if r.Method == "OPTIONS" {
		logger.Printf("[%s] 处理OPTIONS预检请求", requestID)
		buildCorsPreflightResponse(w)
		return
	}

	// 设置响应头
	w.Header().Set("Content-Type", "application/json")

	// 解析请求体
	var requestData struct {
		Ciphertext string `json:"ciphertext"`
	}

	if err := json.NewDecoder(r.Body).Decode(&requestData); err != nil {
		logger.Printf("[%s] 解析请求体失败: %v", requestID, err)
		respondWithError(w, "无效的请求数据", http.StatusBadRequest)
		return
	}

	if requestData.Ciphertext == "" {
		logger.Printf("[%s] 无效的请求数据: 缺少ciphertext字段", requestID)
		respondWithError(w, "无效的请求数据", http.StatusBadRequest)
		return
	}

	// 使用统一的解密函数
	decryptedData, err := decryptRequestData(requestData.Ciphertext, requestID)
	if err != nil {
		respondWithError(w, err.Error(), http.StatusBadRequest)
		return
	}

	// 解析解密后的JSON数据
	var data map[string]interface{}
	if err := json.Unmarshal([]byte(decryptedData), &data); err != nil {
		logger.Printf("[%s] 解析解密后的JSON数据失败: %v", requestID, err)
		respondWithError(w, "无效的请求数据", http.StatusBadRequest)
		return
	}

	// 处理请求
	result := processRequest(requestID, data, false, w, r)
	if result == nil {
		// result 为 nil 表示已经直接写入响应（如流式响应），不需要再处理
		return
	}

	// 获取用户公钥（优先使用 userPublicKeyHex，向后兼容 userPublicKeyBase64）
	userPublicKey, _ := data["userPublicKeyHex"].(string)
	if userPublicKey == "" {
		userPublicKey, _ = data["userPublicKeyBase64"].(string)
	}

	// 根据状态和是否有用户公钥来处理响应
	if result["status"] == "success" {
		logger.Printf("[%s] 处理成功响应，准备返回数据", requestID)

		if userPublicKey != "" {
			// 有公钥，使用统一的加密函数
			encryptedResult, err := encryptResponseData(result, userPublicKey, requestID)
			if err != nil {
				logger.Printf("[%s] 加密响应数据失败: %v，返回未加密结果", requestID, err)
				// 加密失败时，返回未加密的结果
				json.NewEncoder(w).Encode(result)
				return
			}

			json.NewEncoder(w).Encode(map[string]string{
				"ciphertext": encryptedResult,
			})
		} else {
			// 没有公钥，返回未加密结果
			logger.Printf("[%s] 未提供用户公钥，返回未加密响应", requestID)
			json.NewEncoder(w).Encode(result)
		}
	} else {
		// 处理失败响应
		logger.Printf("[%s] 处理失败响应", requestID)
		statusCode := http.StatusBadRequest

		if userPublicKey != "" {
			// 有公钥，使用统一的加密函数
			encryptedResult, err := encryptResponseData(result, userPublicKey, requestID)
			if err != nil {
				logger.Printf("[%s] 加密失败响应数据失败: %v，返回未加密结果", requestID, err)
				// 加密失败时，返回未加密的结果
				w.WriteHeader(statusCode)
				json.NewEncoder(w).Encode(result)
				return
			}

			w.WriteHeader(statusCode)
			json.NewEncoder(w).Encode(map[string]string{
				"ciphertext": encryptedResult,
			})
		} else {
			// 没有公钥，返回未加密结果
			w.WriteHeader(statusCode)
			json.NewEncoder(w).Encode(result)
		}
	}
}

// 处理非加密请求
func handlePlainRequest(w http.ResponseWriter, r *http.Request) {
	// 记录请求开始
	requestID := strconv.FormatInt(time.Now().UnixNano()/1e6, 10)
	clientIP := r.RemoteAddr
	logger.Printf("[%s] 收到来自 %s 的普通请求，路径: %s", requestID, clientIP, r.URL.Path)

	if r.Method == "OPTIONS" {
		logger.Printf("[%s] 处理OPTIONS预检请求", requestID)
		buildCorsPreflightResponse(w)
		return
	}

	// 设置响应头
	w.Header().Set("Content-Type", "application/json")

	// 根据路径判断操作类型（白名单机制：只允许 getServerPublicKey 和 getNonce）
	path := r.URL.Path
	var data map[string]interface{}
	var operation string

	// getNonce 是特殊情况：不需要请求体，不需要 nonce 验证
	if path == "/api/getNonce" {
		nonceStr := nonceService.GenerateNonce()
		logger.Printf("[%s] 生成nonce成功: %s", requestID, nonceStr)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"status": "success",
			"data": map[string]string{
				"nonce": nonceStr,
			},
		})
		return
	}

	// 其他所有 plainRequest 都需要解析请求体
	if err := json.NewDecoder(r.Body).Decode(&data); err != nil {
		logger.Printf("[%s] 解析请求体失败: %v", requestID, err)
		respondWithError(w, "无效的请求数据", http.StatusBadRequest)
		return
	}

	// 统一验证 nonce（getNonce 已经在上面返回了）
	if err := VerifyNonce(requestID, data); err != nil {
		logger.Printf("[%s] nonce 验证失败", requestID)
		respondWithError(w, "无效请求", http.StatusBadRequest)
		return
	}

	// 根据路径处理不同的请求
	switch path {
	case "/api/getServerPublicKey":
		operation = "getServerPublicKey"
		// 获取服务器公钥
		result := readWrite.GetServerPublicKey()
		if !result.IsSuccess() || result.Data == nil {
			logger.Printf("[%s] 服务器公钥读取失败: %v", requestID, result.Error)
			respondWithError(w, "服务器公钥不可用", http.StatusInternalServerError)
			return
		}

		serverPublicKey, ok := result.Data.(string)
		if !ok || serverPublicKey == "" {
			logger.Printf("[%s] 服务器公钥类型错误", requestID)
			respondWithError(w, "服务器公钥不可用", http.StatusInternalServerError)
			return
		}

		logger.Printf("[%s] 服务器公钥读取成功，长度: %d", requestID, len(serverPublicKey))
		json.NewEncoder(w).Encode(map[string]string{
			"status":          "success",
			"serverPublicKey": serverPublicKey,
		})
		return
	case "/api/plainRequest":
		// 对于统一接口，data 已经在前面解析和验证过了
		if len(data) == 0 {
			logger.Printf("[%s] 无效的请求数据: 为空", requestID)
			respondWithError(w, "无效的请求数据", http.StatusBadRequest)
			return
		}

		requestType, _ := data["type"].(string)
		op, _ := data["operation"].(string)

		// 白名单检查：只允许 getServerPublicKey（getNonce 不应该走这里，应该走 /api/getNonce）
		if requestType != "query" {
			logger.Printf("[%s] 非法的plainRequest访问: type=%s", requestID, requestType)
			respondWithError(w, "plainRequest仅支持type为query的请求", http.StatusForbidden)
			return
		}

		if op != "getServerPublicKey" {
			logger.Printf("[%s] 非法的plainRequest访问: operation=%s (仅允许getServerPublicKey)", requestID, op)
			respondWithError(w, "plainRequest仅支持getServerPublicKey操作，getNonce请使用/api/getNonce", http.StatusForbidden)
			return
		}

		operation = op
	default:
		logger.Printf("[%s] 非法的plainRequest路径: %s", requestID, path)
		respondWithError(w, "非法的请求路径", http.StatusForbidden)
		return
	}

	// 处理 getServerPublicKey 操作（需要验证 nonce）
	if operation == "getServerPublicKey" {
		if len(data) == 0 {
			logger.Printf("[%s] 无效的请求数据: 为空", requestID)
			respondWithError(w, "无效的请求数据", http.StatusBadRequest)
			return
		}

		requestType, _ := data["type"].(string)
		op, _ := data["operation"].(string)

		// 白名单检查
		if requestType != "query" || op != "getServerPublicKey" {
			logger.Printf("[%s] 非法的getServerPublicKey访问: type=%s operation=%s", requestID, requestType, op)
			respondWithError(w, "getServerPublicKey仅支持type=query和operation=getServerPublicKey", http.StatusForbidden)
			return
		}

		nonce, ok := data["nonce"].(string)
		if !ok || nonce == "" {
			logger.Printf("[%s] getServerPublicKey缺少nonce参数", requestID)
			respondWithError(w, "无效请求：缺少nonce", http.StatusBadRequest)
			return
		}

		valid, nonceMessage := nonceService.VerifyNonce(nonce)
		if !valid {
			logger.Printf("[%s] getServerPublicKey nonce验证失败: %s", requestID, nonceMessage)
			respondWithError(w, fmt.Sprintf("无效请求：%s", nonceMessage), http.StatusUnauthorized)
			return
		}

		logger.Printf("[%s] getServerPublicKey nonce验证通过", requestID)

		// 处理请求
		result := processRequest(requestID, data, true, w, r)
		if result == nil {
			return
		}

		if result["status"] == "success" {
			logger.Printf("[%s] 处理成功响应", requestID)
			json.NewEncoder(w).Encode(result)
		} else {
			logger.Printf("[%s] 处理失败响应", requestID)
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(result)
		}
		return
	}
}

// 创建一个辅助函数来包装http.ResponseWriter和http.Request为gin.Context
func createGinContext(w http.ResponseWriter, r *http.Request) *gin.Context {
	// 创建一个新的上下文
	c, _ := gin.CreateTestContext(w)
	// 设置请求和响应
	c.Request = r
	c.Writer = &responseWriterWrapper{
		ResponseWriter: w,
		StatusCode:     http.StatusOK,
	}
	return c
}

// 响应写入器包装器，实现gin.ResponseWriter接口
type responseWriterWrapper struct {
	http.ResponseWriter
	StatusCode int
	size       int
}

func (w *responseWriterWrapper) Status() int {
	return w.StatusCode
}

func (w *responseWriterWrapper) WriteHeader(statusCode int) {
	w.StatusCode = statusCode
	w.ResponseWriter.WriteHeader(statusCode)
}

func (w *responseWriterWrapper) WriteHeaderNow() {
	if w.StatusCode == 0 {
		w.StatusCode = http.StatusOK
	}
	w.ResponseWriter.WriteHeader(w.StatusCode)
}

func (w *responseWriterWrapper) Write(data []byte) (int, error) {
	size, err := w.ResponseWriter.Write(data)
	w.size += size
	return size, err
}

func (w *responseWriterWrapper) WriteString(s string) (int, error) {
	size, err := w.ResponseWriter.Write([]byte(s))
	w.size += size
	return size, err
}

func (w *responseWriterWrapper) Flush() {
	if f, ok := w.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

func (w *responseWriterWrapper) CloseNotify() <-chan bool {
	if cn, ok := w.ResponseWriter.(http.CloseNotifier); ok {
		return cn.CloseNotify()
	}
	return make(<-chan bool)
}

func (w *responseWriterWrapper) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	if hj, ok := w.ResponseWriter.(http.Hijacker); ok {
		return hj.Hijack()
	}
	return nil, nil, fmt.Errorf("http.Hijacker接口未实现")
}

func (w *responseWriterWrapper) Pusher() http.Pusher {
	if pusher, ok := w.ResponseWriter.(http.Pusher); ok {
		return pusher
	}
	return nil
}

func (w *responseWriterWrapper) Size() int {
	return w.size
}

func (w *responseWriterWrapper) Written() bool {
	return w.size > 0
}

// 处理错误响应
func respondWithError(w http.ResponseWriter, message string, statusCode int) {
	w.WriteHeader(statusCode)
	json.NewEncoder(w).Encode(map[string]string{
		"status":  "fail",
		"message": message,
	})
}

// 统一的加密/解密辅助函数

// decryptRequestData 解密请求数据（统一的解密函数）
// 参数：
//   - ciphertext: 加密的密文（16进制编码）
//   - requestID: 请求ID（用于日志）
//
// 返回：
//   - decryptedData: 解密后的JSON字符串
//   - err: 错误信息
func decryptRequestData(ciphertext string, requestID string) (string, error) {
	if ciphertext == "" {
		return "", fmt.Errorf("缺少加密数据")
	}

	logger.Printf("[%s] 接收到加密数据，16进制字符串长度: %d", requestID, len(ciphertext))

	// 解密数据（使用RSA+AES混合加密，数据格式为16进制编码）
	info := map[string]string{
		"filePath": "config",
		"userName": "server",
	}

	decryptedData, err := asymmetricdecrypt.AsymmetricDecryptService(ciphertext, info)
	if err != nil {
		logger.Printf("[%s] 数据解密失败: %v", requestID, err)
		return "", fmt.Errorf("数据解密失败: %v", err)
	}

	logger.Printf("[%s] 数据解密成功，解密后长度: %d", requestID, len(decryptedData))
	return decryptedData, nil
}

// parsePublicKey 解析用户公钥（16进制编码的PEM格式）
// 参数：
//   - userPublicKey: 用户公钥（16进制编码的PEM格式）
//
// 返回：
//   - *rsa.PublicKey: 解析后的RSA公钥，失败返回nil
func parsePublicKey(userPublicKey string) *rsa.PublicKey {
	if userPublicKey == "" {
		return nil
	}

	// 先进行16进制解码
	publicKeyBytes, err := hex.DecodeString(userPublicKey)
	if err != nil {
		return nil
	}

	block, _ := pem.Decode(publicKeyBytes)
	if block == nil {
		return nil
	}

	parsedKey, err := x509.ParsePKIXPublicKey(block.Bytes)
	if err != nil {
		return nil
	}

	rsaPublicKey, ok := parsedKey.(*rsa.PublicKey)
	if !ok {
		return nil
	}

	return rsaPublicKey
}

// encryptResponseData 加密响应数据（统一的加密函数）
// 参数：
//   - data: 要加密的数据（map或可序列化为JSON的数据）
//   - userPublicKey: 用户公钥（16进制编码的PEM格式，优先使用 userPublicKeyHex，向后兼容 userPublicKeyBase64）
//   - requestID: 请求ID（用于日志）
//
// 返回：
//   - encryptedResult: 加密后的密文（16进制编码），失败返回空字符串
//   - err: 错误信息
func encryptResponseData(data interface{}, userPublicKey string, requestID string) (string, error) {
	if userPublicKey == "" {
		return "", fmt.Errorf("缺少用户公钥")
	}

	// 序列化数据
	resultJSON, err := json.Marshal(data)
	if err != nil {
		logger.Printf("[%s] 序列化响应数据失败: %v", requestID, err)
		return "", fmt.Errorf("序列化响应数据失败: %v", err)
	}

	// 解析用户公钥
	rsaPublicKey := parsePublicKey(userPublicKey)
	if rsaPublicKey == nil {
		logger.Printf("[%s] 解析用户公钥失败", requestID)
		return "", fmt.Errorf("解析用户公钥失败")
	}

	// 加密数据
	logger.Printf("[%s] 开始加密响应数据", requestID)
	encryptedResult, err := asymmetricencrypt.AsymmetricEncryptService(string(resultJSON), rsaPublicKey)
	if err != nil {
		logger.Printf("[%s] 加密响应数据失败: %v", requestID, err)
		return "", fmt.Errorf("加密响应数据失败: %v", err)
	}

	logger.Printf("[%s] 响应数据加密成功，长度: %d", requestID, len(encryptedResult))
	return encryptedResult, nil
}

// decryptAndVerifyRequest 通用的解密和验证函数（包含nonce验证）
// 参数：
//   - w: HTTP响应写入器
//   - r: HTTP请求
//   - requestID: 请求ID
//
// 返回：
//   - decryptedJSON: 解密后的JSON数据
//   - err: 错误信息
func decryptAndVerifyRequest(w http.ResponseWriter, r *http.Request, requestID string) (map[string]interface{}, error) {
	// 读取请求体
	bodyBytes, err := io.ReadAll(r.Body)
	if err != nil {
		logger.Printf("[%s] 读取请求体失败: %v", requestID, err)
		respondWithError(w, "读取请求失败", http.StatusBadRequest)
		return nil, err
	}
	r.Body.Close()

	// 解析加密请求
	var requestData struct {
		Ciphertext string `json:"ciphertext"`
	}

	if err := json.Unmarshal(bodyBytes, &requestData); err != nil {
		logger.Printf("[%s] 请求格式错误: %v", requestID, err)
		respondWithError(w, "请求格式错误：必须提供加密数据", http.StatusBadRequest)
		return nil, err
	}

	if requestData.Ciphertext == "" {
		logger.Printf("[%s] 缺少加密数据", requestID)
		respondWithError(w, "安全错误：必须使用加密请求", http.StatusBadRequest)
		return nil, fmt.Errorf("缺少加密数据")
	}

	// 使用统一的解密函数
	decryptedData, err := decryptRequestData(requestData.Ciphertext, requestID)
	if err != nil {
		respondWithError(w, err.Error(), http.StatusUnauthorized)
		return nil, err
	}

	// 验证解密后的数据并验证nonce
	var decryptedJSON map[string]interface{}
	if err := json.Unmarshal([]byte(decryptedData), &decryptedJSON); err != nil {
		logger.Printf("[%s] 解析解密后的JSON失败: %v", requestID, err)
		respondWithError(w, "无效的请求数据", http.StatusBadRequest)
		return nil, err
	}

	// 验证nonce（防重放攻击）
	nonce, ok := decryptedJSON["nonce"].(string)
	if !ok {
		logger.Printf("[%s] 请求缺少nonce参数", requestID)
		respondWithError(w, "无效请求：缺少nonce", http.StatusBadRequest)
		return nil, fmt.Errorf("缺少nonce参数")
	}

	valid, nonceMessage := nonceService.VerifyNonce(nonce)
	if !valid {
		logger.Printf("[%s] nonce验证失败: %s", requestID, nonceMessage)
		respondWithError(w, fmt.Sprintf("无效请求：%s", nonceMessage), http.StatusUnauthorized)
		return nil, fmt.Errorf("nonce验证失败: %s", nonceMessage)
	}

	logger.Printf("[%s] nonce验证通过", requestID)
	return decryptedJSON, nil
}

// 构建CORS预检响应
func buildCorsPreflightResponse(w http.ResponseWriter) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
	w.Header().Set("Access-Control-Allow-Methods", "GET,POST,OPTIONS")
	w.WriteHeader(http.StatusOK)
}

// 主页处理函数
func handleRoot(w http.ResponseWriter, r *http.Request) {
	logger.Printf("接收到请求: %s %s", r.Method, r.URL.Path)
	fmt.Fprintf(w, "数字奇点 HTTP 服务")
}

// loggingMiddleware 记录所有HTTP请求的中间件
func loggingMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// 记录请求开始
		requestID := strconv.FormatInt(time.Now().UnixNano()/1e6, 10)
		clientIP := r.RemoteAddr

		// 获取真实IP（如果使用了反向代理）
		if forwarded := r.Header.Get("X-Forwarded-For"); forwarded != "" {
			clientIP = forwarded
		} else if realIP := r.Header.Get("X-Real-IP"); realIP != "" {
			clientIP = realIP
		}

		// 记录请求信息
		logger.Printf("[%s] %s %s 来自 %s", requestID, r.Method, r.URL.Path, clientIP)

		// 调用下一个处理器
		next.ServeHTTP(w, r)
	})
}

// HandleConnection 启动HTTP服务
func HandleConnection(host string, port int, debug bool) {
	// 设置路由
	handler := setupRoutes()

	// 添加日志中间件
	handler = loggingMiddleware(handler)

	// 创建服务地址
	addr := fmt.Sprintf("%s:%d", host, port)

	// 创建服务器
	server := &http.Server{
		Addr:    addr,
		Handler: handler,
	}

	// 打印启动信息
	logger.Printf("HTTP服务器启动于 %s", addr)
	if debug {
		logger.Printf("调试模式已启用")
	}

	// 启动服务器
	if err := server.ListenAndServe(); err != nil {
		logger.Fatalf("HTTP服务器启动失败: %v", err)
	}
}
