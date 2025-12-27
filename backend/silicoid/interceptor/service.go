// 拦截器：处理不同AI模型的请求
// 将使用Claude模型的请求转发至格式转换器

package interceptor

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"digitalsingularity/backend/common/auth/tokenmanage"
	"digitalsingularity/backend/common/userfiles"
	"digitalsingularity/backend/common/utils/datahandle"
	aibasicplatformdatabase "digitalsingularity/backend/aibasicplatform/database"
	"digitalsingularity/backend/main/accountmanagement/apikeymanage"


	"digitalsingularity/backend/silicoid/database"
	"digitalsingularity/backend/silicoid/formatconverter"
	"digitalsingularity/backend/silicoid/mcp"
	"digitalsingularity/backend/silicoid/models/claude"
	"digitalsingularity/backend/silicoid/models/manager"
	"digitalsingularity/backend/silicoid/models/openai"

	speechsysteminterceptor "digitalsingularity/backend/speechsystem/interceptor"
)

// checkAndLogResponseError 统一的响应错误检测和日志记录函数
func checkAndLogResponseError(response map[string]interface{}, requestID string, serviceName string) bool {
	if errObj, exists := response["error"]; exists && errObj != nil {
		logger.Printf("[%s] %s API 返回错误: %v", requestID, serviceName, errObj)
		return true // 有错误
	}
	return false // 无错误
}

// wrapStreamResponseWithErrorDetection 统一的流式响应错误检测包装器
func wrapStreamResponseWithErrorDetection(streamChan chan string, modelName string, requestID string) chan string {
	wrappedChan := make(chan string)

	go func() {
		defer close(wrappedChan)

		for chunk := range streamChan {
			// 检测是否是错误消息
			if IsErrorChunk(chunk) {
				// 尝试解析错误信息
				errorInfo := ExtractErrorInfo(chunk)

				// 检测是否是 401 认证错误
				if errorInfo != nil && errorInfo.IsAuthError() {
					logger.Printf("[%s] ⚠️ 检测到 API Key 认证错误（401），模型: %s", requestID, modelName)

					// 生成友好的错误消息，建议用户切换模型
					friendlyError := GenerateAuthErrorResponse(modelName)
					wrappedChan <- friendlyError
					return
				}
			}

			// 非错误消息或非认证错误，直接转发
			wrappedChan <- chunk
		}
	}()

	return wrappedChan
}

// 获取logger
var logger = log.New(log.Writer(), "silicoid_interceptor: ", log.LstdFlags)

// generateMessageID 生成消息ID
func generateMessageID() string {
	return uuid.New().String()
}

// ensureMessagesHaveID 确保所有消息都有 id 字段
// 如果消息没有 id，则自动生成一个
func ensureMessagesHaveID(messages []interface{}) []interface{} {
	result := make([]interface{}, 0, len(messages))
	for _, msg := range messages {
		msgMap, ok := msg.(map[string]interface{})
		if !ok {
			// 如果不是 map，直接添加
			result = append(result, msg)
			continue
		}
		
		// 检查是否已有 id
		if _, hasID := msgMap["id"]; !hasID {
			// 如果没有 id，生成一个
			msgMap["id"] = generateMessageID()
		}
		
		result = append(result, msgMap)
	}
	return result
}

// 共享的工具列表常量
var (
	// ToolsExecutionType 存储工具的 execution_type（server_executor 或 client_executor）
	ToolsExecutionType = map[string]string{}
)

// SynthesisResultCallback TTS 合成结果回调接口
// 此接口定义在 silicoid/interceptor 中，避免 WebSocket 层直接依赖 speechsystem/interceptor
type SynthesisResultCallback interface {
	// OnAudioChunk 流式音频数据块回调（识别过程中实时返回）
	OnAudioChunk(audioData []byte)
	// OnSynthesisComplete 合成完成回调（合成完成时返回完整音频）
	OnSynthesisComplete(audioData []byte)
	// OnError 合成错误回调
	OnError(err error)
}

// truncateString 截断字符串到指定长度
func truncateString(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}

// ReadWriteAdapter 适配器实现auth/tokenmanage.ReadWriteService接口
type ReadWriteAdapter struct {
	rw *datahandle.CommonReadWriteService
}

// GetRedis 适配器方法
func (a *ReadWriteAdapter) GetRedis(key string) string {
	result := a.rw.GetRedis(key)
	if !result.IsSuccess() {
		return ""
	}
	if str, ok := result.Data.(string); ok {
		return str
	}
	return ""
}

// SetRedis 适配器方法
func (a *ReadWriteAdapter) SetRedis(key string, value string, expire int) error {
	// 将int类型的过期时间转换为time.Duration
	expireDuration := time.Duration(expire) * time.Second
	result := a.rw.SetRedis(key, value, expireDuration)
	if !result.IsSuccess() {
		return result.Error
	}
	return nil
}

// DeleteRedis 适配器方法
func (a *ReadWriteAdapter) DeleteRedis(key string) error {
	result := a.rw.DeleteRedis(key)
	if !result.IsSuccess() {
		return result.Error
	}
	return nil
}


// SilicoIDInterceptor 拦截器结构体
type SilicoIDInterceptor struct {
	apiKeyManageService        *apikeymanage.ApiKeyManageService
	authTokenService           *tokenmanage.CommonAuthTokenService
	formatConverter            *formatconverter.SilicoidFormatConverterService
	openaiService              *openai.OpenAIService
	claudeService              *claude.ClaudeService
	readWrite                  *datahandle.CommonReadWriteService
	dataService                *database.SilicoidDataService              // 数据库服务
	aiPlatformDataService      *aibasicplatformdatabase.AIBasicPlatformDataService
	modelManager               *manager.ModelManager                     // 模型管理器 (从数据库/Redis动态加载模型配置)
	ttsService                 *speechsysteminterceptor.TTSService       // TTS 服务
	fileService                *userfiles.FileService                    // 文件服务
	mcpClientManager           *mcp.MCPClientManager                     // MCP 客户端管理器
}

// 创建拦截器实例
func CreateInterceptor() *SilicoIDInterceptor {
	readWrite, _ := datahandle.NewCommonReadWriteService("database")
	adapter := &ReadWriteAdapter{rw: readWrite}
	
	// 初始化模型管理器
	modelManager := manager.NewModelManager()
	logger.Printf("✅ 模型管理器已成功初始化")
	
	// 初始化API密钥管理服务
	apiKeyManageService := apikeymanage.NewApiKeyManageService(tokenmanage.NewCommonAuthTokenService(adapter))
	
	// 初始化 TTS 服务
	ttsService := speechsysteminterceptor.NewTTSService()
	
	// 初始化数据库服务
	dataService := database.NewSilicoidDataService(readWrite)
	aiPlatformDataService := aibasicplatformdatabase.NewAIBasicPlatformDataService(readWrite)
	
	// 初始化文件服务
	fileService := userfiles.NewFileService()

	// 从工具表加载执行类型映射（execution_type）
	if aiPlatformDataService != nil {
		if tools, err := aiPlatformDataService.GetAllTools(); err == nil {
			logger.Printf("从数据库加载了 %d 个工具", len(tools))
			for _, t := range tools {
				if t.ToolName != "" && t.ExecutionType != "" {
					ToolsExecutionType[t.ToolName] = t.ExecutionType
					logger.Printf("工具执行类型加载: %s -> %s", t.ToolName, t.ExecutionType)
				} else {
					logger.Printf("跳过无效工具: name='%s', execution_type='%s'", t.ToolName, t.ExecutionType)
				}
			}
			logger.Printf("ToolsExecutionType 映射大小: %d", len(ToolsExecutionType))
			// 打印所有加载的工具执行类型
			for name, execType := range ToolsExecutionType {
				logger.Printf("已加载工具映射: %s -> %s", name, execType)
			}
		} else {
			logger.Printf("警告: 加载工具执行类型失败: %v", err)
		}
	} else {
		logger.Printf("警告: aiPlatformDataService 未初始化，无法加载工具执行类型")
	}
	
	// 创建格式转换器
	formatConverter := formatconverter.NewSilicoidFormatConverterService()
	
	// 创建 Claude 服务
	claudeService := claude.NewClaudeService()
	
	// 设置 Claude 文件上传器到格式转换器（支持 Claude Files API）
	formatConverter.SetClaudeFileUploader(claudeService)

	// 初始化 MCP 客户端管理器
	mcpClientManager := mcp.NewMCPClientManager()
	logger.Printf("✅ MCP 客户端管理器已成功初始化")

	return &SilicoIDInterceptor{
		apiKeyManageService:        apiKeyManageService,
		authTokenService:           tokenmanage.NewCommonAuthTokenService(adapter),
		formatConverter:            formatConverter,
		openaiService:              openai.NewOpenAIService(),
		claudeService:              claudeService,
		readWrite:                  readWrite,
		dataService:                 dataService,
		aiPlatformDataService:       aiPlatformDataService,
		modelManager:                  modelManager,
		ttsService:                    ttsService,
		fileService:                   fileService,
		mcpClientManager:             mcpClientManager,
	}
}


// processFilesInRequest 处理请求中的文件：检测新文件（base64数据）并上传到 userfiles，获取 file_id
func (s *SilicoIDInterceptor) processFilesInRequest(data map[string]interface{}, userId string, requestID string) (map[string]interface{}, error) {
	messages, ok := data["messages"].([]interface{})
	if !ok {
		return data, nil
	}

	// 遍历所有消息
	for _, msg := range messages {
		msgMap, ok := msg.(map[string]interface{})
		if !ok {
			continue
		}

		content, ok := msgMap["content"]
		if !ok {
			continue
		}

		// 检查 content 是否为数组格式
		contentArray, ok := content.([]interface{})
		if !ok {
			continue
		}

		// 处理数组中的每个元素
		for i, part := range contentArray {
			partMap, ok := part.(map[string]interface{})
			if !ok {
				continue
			}

			partType, _ := partMap["type"].(string)

			// 检查是否有 base64 文件数据（新文件）
			if partType == "image" {
				source, ok := partMap["source"].(map[string]interface{})
				if !ok {
					continue
				}
				sourceType, _ := source["type"].(string)
				if sourceType == "base64" {
					// 检测到 base64 图片数据，需要上传
					fileData, _ := source["data"].(string)
					mediaType, _ := source["media_type"].(string)
					if fileData != "" {
						fileId, err := s.uploadFileToUserfiles(userId, fileData, "image", mediaType, requestID)
						if err != nil {
							logger.Printf("[%s] 上传图片文件失败: %v", requestID, err)
							return nil, fmt.Errorf("上传图片文件失败: %v", err)
						}
						// 替换为 file_id
						partMap["type"] = "file_id"
						partMap["file_id"] = fileId
						delete(partMap, "source")
						contentArray[i] = partMap
						logger.Printf("[%s] 图片文件已上传，file_id: %s", requestID, fileId)
					}
				}
			} else if partType == "document" {
				source, ok := partMap["source"].(map[string]interface{})
				if !ok {
					continue
				}
				sourceType, _ := source["type"].(string)
				if sourceType == "base64" {
					// 检测到 base64 文档数据，需要上传
					fileData, _ := source["data"].(string)
					mediaType, _ := source["media_type"].(string)
					if fileData != "" {
						fileType := "document"
						if strings.Contains(mediaType, "pdf") {
							fileType = "pdf"
						}
						fileId, err := s.uploadFileToUserfiles(userId, fileData, fileType, mediaType, requestID)
						if err != nil {
							logger.Printf("[%s] 上传文档文件失败: %v", requestID, err)
							return nil, fmt.Errorf("上传文档文件失败: %v", err)
						}
						// 替换为 file_id
						partMap["type"] = "file_id"
						partMap["file_id"] = fileId
						delete(partMap, "source")
						contentArray[i] = partMap
						logger.Printf("[%s] 文档文件已上传，file_id: %s", requestID, fileId)
					}
				}
			}
		}

		// 更新消息的 content
		msgMap["content"] = contentArray
	}

	return data, nil
}

// uploadFileToUserfiles 上传文件到 userfiles 服务
func (s *SilicoIDInterceptor) uploadFileToUserfiles(userId string, base64Data string, fileType string, mimeType string, requestID string) (string, error) {
	// userfiles 服务现在支持 base64，直接传递，无需转换
	// 计算文件大小（解码 base64 以获取实际大小）
	fileBytes, err := base64.StdEncoding.DecodeString(base64Data)
	if err != nil {
		return "", fmt.Errorf("base64 解码失败: %v", err)
	}

	// 生成文件名
	fileName := fmt.Sprintf("upload_%s", requestID)
	if fileType == "image" {
		if strings.Contains(mimeType, "jpeg") || strings.Contains(mimeType, "jpg") {
			fileName += ".jpg"
		} else if strings.Contains(mimeType, "png") {
			fileName += ".png"
		} else if strings.Contains(mimeType, "gif") {
			fileName += ".gif"
		} else if strings.Contains(mimeType, "webp") {
			fileName += ".webp"
		}
	} else if fileType == "pdf" {
		fileName += ".pdf"
	}

	// 调用 userfiles 服务上传文件（直接传递 base64）
	req := &userfiles.UploadFileRequest{
		FileData: base64Data,
		FileName: fileName,
		FileType: fileType,
		MimeType: mimeType,
		FileSize: int64(len(fileBytes)),
	}

	result, err := s.fileService.UploadFile(userId, req)
	if err != nil {
		return "", fmt.Errorf("上传文件失败: %v", err)
	}

	if !result.Success {
		return "", fmt.Errorf("上传文件失败: %s", result.Message)
	}

	return result.FileId, nil
}

// isClaudeModel 判断模型是否为Claude模型
// 通过查询数据库/Redis中的模型配置，根据provider字段判断
func (s *SilicoIDInterceptor) isClaudeModel(modelName string) bool {
	// 尝试从模型管理器获取模型配置
	// 首先尝试直接匹配 model_code
	modelConfig, err := s.modelManager.GetModelConfig(modelName)
	if err == nil && modelConfig != nil {
		// 根据 provider 字段判断
		provider := strings.ToLower(modelConfig.Provider)
		isClaude := provider == "anthropic" || strings.Contains(provider, "claude")
		if isClaude {
			logger.Printf("✅ 模型 %s 识别为 Claude 模型 (provider: %s)", modelName, modelConfig.Provider)
		} else {
			logger.Printf("✅ 模型 %s 识别为非 Claude 模型 (provider: %s)", modelName, modelConfig.Provider)
		}
		return isClaude
	}
	
	// 如果数据库中没有找到，尝试模糊匹配常见的模型名称前缀
	// 这是降级方案，当数据库不可用或模型未配置时使用
	modelLower := strings.ToLower(modelName)
	
	// Claude 模型特征：包含 "claude" 或以 "sk-ant-" 开头的API Key使用的模型
	if strings.Contains(modelLower, "claude") {
		logger.Printf("⚠️  通过模型名称模糊匹配识别为 Claude 模型: %s (数据库查询失败: %v)", modelName, err)
		return true
	}
	
	logger.Printf("⚠️  通过模型名称模糊匹配识别为非 Claude 模型: %s (数据库查询失败: %v)", modelName, err)
	return false
}

// extractApiKey 从请求中提取API密钥
func (s *SilicoIDInterceptor) extractApiKey(c *gin.Context) string {
	// 从Authorization头中提取
	authHeader := c.GetHeader("Authorization")
	if strings.HasPrefix(authHeader, "Bearer ") {
		return authHeader[7:]
	}
	
	// 如果没有在Authorization头中找到，尝试从查询参数中提取
	apiKey := c.Query("api_key")
	if apiKey != "" {
		return apiKey
	}
	
	// 如果仍然没有找到，尝试从请求体中提取
	var data map[string]interface{}
	if err := c.ShouldBindJSON(&data); err == nil {
		if apiKey, ok := data["api_key"].(string); ok {
			return apiKey
		}
	}
	
	return ""
}

// extractTextToolCalls 从文本响应中提取工具调用
func extractTextToolCalls(response string) []map[string]interface{} {
	var calls []map[string]interface{}

	// 查找TOOL_CALL格式的调用
	lines := strings.Split(response, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "TOOL_CALL:") {
			// 提取JSON部分
			jsonStr := strings.TrimPrefix(line, "TOOL_CALL:")
			jsonStr = strings.TrimSpace(jsonStr)

			var call map[string]interface{}
			if err := json.Unmarshal([]byte(jsonStr), &call); err == nil {
				calls = append(calls, call)
			}
		}
	}

	return calls
}

// convertTextCallsToStructured 将文本工具调用转换为结构化格式
func convertTextCallsToStructured(textCalls []map[string]interface{}) []interface{} {
	var structuredCalls []interface{}

	for _, textCall := range textCalls {
		if name, ok := textCall["name"].(string); ok {
			if args, ok := textCall["arguments"].(map[string]interface{}); ok {
				// 转换为OpenAI工具调用格式
				structuredCall := map[string]interface{}{
					"id":       fmt.Sprintf("text_call_%d", len(structuredCalls)+1),
					"type":     "function",
					"function": map[string]interface{}{
						"name":      name,
						"arguments": args,
					},
				}
				structuredCalls = append(structuredCalls, structuredCall)
			}
		}
	}
	return structuredCalls
}

// AuthenticatedRequestData HTTP请求认证和预处理结果
type AuthenticatedRequestData struct {
	UserID         string
	UserOwnOpenAIKey string
	UserOwnClaudeKey string
	Data           map[string]interface{}
	RequestID      string
}

// authenticateAndPreprocessRequest HTTP请求的通用认证和预处理逻辑
func (s *SilicoIDInterceptor) authenticateAndPreprocessRequest(c *gin.Context) (*AuthenticatedRequestData, error) {
	requestID := fmt.Sprintf("%d", time.Now().UnixNano()/int64(time.Millisecond))
	logger.Printf("[%s] 开始认证和预处理HTTP请求 (IP: %s)", requestID, c.ClientIP())
	
	// 获取数据
	var data map[string]interface{}
	if err := c.ShouldBindJSON(&data); err != nil {
		logger.Printf("[%s] 解析请求数据失败: %v", requestID, err)
		return nil, fmt.Errorf("无效的请求数据: %v", err)
	}
	
	logger.Printf("[%s] 从请求中获取数据", requestID)

	// 在对话发起时保存初始上下文到 Redis，便于后续从工具调用或格式转换器中查找
	func() {
		svc, err := datahandle.NewCommonReadWriteService("database")
		if err != nil {
			logger.Printf("[%s] ⚠️ 无法初始化 CommonReadWriteService 保存初始上下文: %v", requestID, err)
			return
		}
		defer func() {
			// svc 可能没有需要关闭的方法，这里是占位以便未来扩展
		}()

		modelVal, _ := data["model"].(string)
		roleNameVal, _ := data["role_name"].(string)
		userIdVal, _ := data["user_id"].(string)

		storeObj := map[string]interface{}{
			"request_id": requestID,
			"model":      modelVal,
			"role_name":  roleNameVal,
			"user_id":    userIdVal,
			"created_at": time.Now().Unix(),
		}

		key := fmt.Sprintf("tools_call_context:%s", requestID)
		res := svc.RedisWrite(key, storeObj, 5*time.Minute)
		if res == nil || res.Status != datahandle.StatusSuccess {
			var errDetail interface{}
			if res != nil {
				errDetail = res.Error
			} else {
				errDetail = "nil result"
			}
			logger.Printf("[%s] ⚠️ 保存初始 tools_call_context 到 Redis 失败: %v", requestID, errDetail)
		} else {
			logger.Printf("[%s] ✅ 已将对话初始上下文存入 Redis key=%s", requestID, key)
		}
	}()
	
	// 提取认证信息
	apiKey := s.extractApiKey(c)
	authToken, authTokenExists := data["auth_token"].(string)
	if !authTokenExists {
		authToken = s.extractAuthToken(c)
	}
	
	var userId string
	var userOwnOpenAIKey string // 用户自己的 OpenAI Key
	var userOwnClaudeKey string // 用户自己的 Claude Key
	
	// 验证用户身份
	switch {
	case apiKey != "":
		// 根据 API Key 前缀判断类型
		switch {
		case strings.HasPrefix(apiKey, "sk-potagi-"):
			// 场景1: 平台认证 Key (sk-potagi-xxx)
			// ✅ 需要验证用户身份和余额
			valid, id, errorMessage := s.apiKeyManageService.VerifyApiKey(apiKey)
			if !valid {
				logger.Printf("[%s] 平台API密钥验证失败: %s", requestID, errorMessage)
				return nil, fmt.Errorf("API密钥验证失败: %s", errorMessage)
			}
			
			userId = id
			logger.Printf("[%s] 平台API密钥验证成功，用户ID: %s (需扣费)", requestID, userId)
			
		case strings.HasPrefix(apiKey, "sk-ant-"):
			// 场景2: 用户自己的 Claude Key (sk-ant-xxx)
			// ✅ 不验证，不扣费，直接使用
			userOwnClaudeKey = apiKey
			userId = "user-own-key" // 标记为使用自己的 Key，不需要真实 userId
			logger.Printf("[%s] 使用用户自己的 Claude Key (不扣费)", requestID)
			
			// 标记为使用用户自己的 key
			data["_use_user_key"] = true
			data["_user_claude_key"] = userOwnClaudeKey
			
		case strings.HasPrefix(apiKey, "sk-"):
			// 场景3: 用户自己的 OpenAI Key (sk-xxx，但不是 sk-potagi- 或 sk-ant-)
			// ✅ 不验证，不扣费，直接使用
			userOwnOpenAIKey = apiKey
			userId = "user-own-key" // 标记为使用自己的 Key，不需要真实 userId
			logger.Printf("[%s] 使用用户自己的 OpenAI Key (不扣费)", requestID)
			
			// 标记为使用用户自己的 key
			data["_use_user_key"] = true
			data["_user_openai_key"] = userOwnOpenAIKey
			
		default:
			// 场景4: 未知格式的 key，作为平台认证 key 验证
			valid, id, errorMessage := s.apiKeyManageService.VerifyApiKey(apiKey)
			if !valid {
				logger.Printf("[%s] API密钥验证失败: %s", requestID, errorMessage)
				return nil, fmt.Errorf("API密钥验证失败: %s", errorMessage)
			}
			
			userId = id
			logger.Printf("[%s] API密钥验证成功，用户ID: %s", requestID, userId)
		}
		
	case authToken != "":
		// 使用AuthToken验证
		valid, payload := s.authTokenService.VerifyAuthToken(authToken)
		if !valid {
			errorMsg := "无效的AuthToken"
			if errStr, ok := payload.(string); ok {
				errorMsg = errStr
			}
			logger.Printf("[%s] AuthToken验证失败: %v", requestID, errorMsg)
			return nil, fmt.Errorf("AuthToken验证失败: %s", errorMsg)
		}
		
		payloadMap := payload.(map[string]interface{})
		userId = payloadMap["userId"].(string)
		logger.Printf("[%s] AuthToken验证成功，用户ID: %s", requestID, userId)
		
	default:
		// 无认证信息
		logger.Printf("[%s] 未提供认证信息", requestID)
		return nil, fmt.Errorf("请提供有效的API密钥或Token")
	}
	
	// 如果不是使用用户自己的 Key，才检查令牌余额
	if userOwnOpenAIKey == "" && userOwnClaudeKey == "" {
		hasTokens, _, err, _ := s.apiKeyManageService.CheckUserTokens(userId)
		if !hasTokens {
			errorMessage := ""
			if err != nil {
				errorMessage = err.Error()
			}
			logger.Printf("[%s] 令牌余额不足: %s", requestID, errorMessage)
			return nil, fmt.Errorf("令牌余额不足: %s", errorMessage)
		}
		
		logger.Printf("[%s] 用户令牌余额充足，可以继续处理请求", requestID)
	} else {
		logger.Printf("[%s] 使用用户自己的 OpenAI Key，跳过余额检查", requestID)
	}
	
	// 处理模型配置
	modelInterface, exists := data["model"]
	if !exists {
		return nil, fmt.Errorf("缺少模型参数")
	}
	
	model, _ := modelInterface.(string)
	logger.Printf("[%s] 请求模型: %s (前端传入的可能是 model_name)", requestID, model)
	
	// 如果使用平台 API Key，需要根据 model 获取对应的 model_code 和 API Key
	useUserKey, _ := data["_use_user_key"].(bool)
	if !useUserKey {
		// 使用平台 Key，需要从数据库获取对应的 model_code 和 API Key
		modelConfig, err := s.modelManager.GetModelConfig(model)
		if err != nil {
			logger.Printf("[%s] ❌ 获取模型配置失败: %v，模型: %s (可能是 model_name 或 model_code)", requestID, err, model)
			return nil, fmt.Errorf("模型配置不存在: %s", model)
		}

			// 将 model_code、base_url 和 endpoint 存储到 data 中，供后续服务使用
			data["model_code"] = modelConfig.ModelCode
			data["_base_url"] = modelConfig.BaseURL
			data["_endpoint"] = modelConfig.Endpoint
			logger.Printf("[%s] ✅ 获取到模型配置: 前端传入=%s, model_code=%s, model_name=%s, base_url=%s, endpoint=%s", 
				requestID, model, modelConfig.ModelCode, modelConfig.ModelName, modelConfig.BaseURL, modelConfig.Endpoint)
	} else {
		logger.Printf("[%s] 使用用户自己的 Key，无需获取平台 API Key", requestID)
	}
	
	// 处理文件上传
	data, err := s.processFilesInRequest(data, userId, requestID)
	if err != nil {
		logger.Printf("[%s] 处理文件失败: %v", requestID, err)
		return nil, fmt.Errorf("处理文件失败: %v", err)
	}

	return &AuthenticatedRequestData{
		UserID:           userId,
		UserOwnOpenAIKey: userOwnOpenAIKey,
		UserOwnClaudeKey: userOwnClaudeKey,
		Data:             data,
		RequestID:        requestID,
	}, nil
}


