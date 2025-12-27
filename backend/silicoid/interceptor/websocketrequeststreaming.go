package interceptor

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
)
// CreateWebSocketStreamResponse 创建WebSocket流式 AI 响应
// 返回一个 channel（通过它可以接收流式数据）和 session_id
func (s *SilicoIDInterceptor) CreateWebSocketStreamResponse(
	ctx context.Context,
	requestData map[string]interface{},
	requestID string,
) (chan string, string, error) {
	// 生成 session_id
	sessionID := uuid.New().String()
	logger.Printf("[%s] 📝 生成 session_id: %s", requestID, sessionID)
	
	// 获取模型名称
	modelName, ok := requestData["model"].(string)
	if !ok {
		return nil, "", fmt.Errorf("缺少模型参数")
	}
	
	// 获取用户ID和API Key
	userID, _ := requestData["user_id"].(string)
	apiKey, _ := requestData["api_key"].(string)
	
	// 检查用户资产
	hasAssets, err := s.checkUserAssets(userID, apiKey)
	if err != nil {
		return nil, "", fmt.Errorf("检查用户资产失败: %v", err)
	}
	if !hasAssets {
		return nil, "", fmt.Errorf("用户资产不足")
	}
	
	// 根据model_name查询模型配置（获取base_url和endpoint）
	modelConfig, err := s.modelManager.GetModelConfig(modelName)
	if err != nil {
		logger.Printf("[%s] ⚠️ 获取模型配置失败: %v", requestID, err)
		return nil, "", fmt.Errorf("获取模型配置失败: %v", err)
	}
	
	// 将获取到的配置存储到requestData中
	modelCode := modelConfig.ModelCode
	baseURL := modelConfig.BaseURL
	endpoint := modelConfig.Endpoint
	requestData["model_code"] = modelCode
	requestData["_base_url"] = baseURL
	requestData["_endpoint"] = endpoint
	logger.Printf("[%s] ✅ 成功获取模型配置: model_code=%s, base_url=%s, endpoint=%s", 
		requestID, modelCode, baseURL, endpoint)
	
	// 判断是否是 Claude 模型
	isClaudeModel := s.isClaudeModel(modelName)
	
	logger.Printf("[%s] 📡 创建流式响应，模型: %s, Claude模型: %v", requestID, modelName, isClaudeModel)
	
	// 工具添加由 formatConverter 自动处理
	
	// 根据模型类型选择服务
	var streamChan chan string
	
	if isClaudeModel {
		// 使用 Claude 服务
		logger.Printf("[%s] 使用 Claude 服务处理流式请求", requestID)
		
		// 先将 OpenAI 格式转换为 Claude 格式
		claudeData, err := s.formatConverter.RequestOpenAIToClaude(requestData)
		if err != nil {
			return nil, "", fmt.Errorf("格式转换失败: %v", err)
		}
		
		streamChan = s.claudeService.CreateChatCompletionStream(ctx, claudeData)
	} else {
		// 使用 OpenAI 服务（支持 DeepSeek、OpenAI 等兼容 OpenAI API 的模型）
		logger.Printf("[%s] 使用 OpenAI 兼容服务处理流式请求", requestID)
		
		// 直接传递请求数据，让 OpenAI 服务自己处理规范化
		streamChan = s.openaiService.CreateChatCompletionStream(ctx, requestData)
	}
	
	// 包装流式响应 channel，检测并处理错误（特别是 401 认证错误）
	wrappedChan := wrapStreamResponseWithErrorDetection(streamChan, modelName, requestID)
	
	return wrappedChan, sessionID, nil
}
// HandleWebSocketRequestStream 处理所有模型的WebSocket流式请求
// ProcessStreamChat 处理流式AI聊天（WebSocket接口）
func (s *SilicoIDInterceptor) ProcessStreamChat(ctx context.Context, requestID string, userID string, requestData map[string]interface{}, sendMessage func(messageType string, data map[string]interface{}) error, sendChunk func(chunk string) error) error {
	return s.HandleWebSocketRequestStream(ctx, requestID, userID, requestData, sendMessage, sendChunk)
}

func (s *SilicoIDInterceptor) HandleWebSocketRequestStream(ctx context.Context, requestID string, userID string, requestData map[string]interface{}, sendMessage func(messageType string, data map[string]interface{}) error, sendChunk func(chunk string) error) error {
	logger.Printf("[%s] 处理流式AI聊天会话管理 (user=%s)", requestID, userID)

	// 发起流式AI请求
	streamChan, sessionID, err := s.CreateWebSocketStreamResponse(ctx, requestData, requestID)
	if err != nil {
		logger.Printf("[%s] 创建流式响应失败: %v", requestID, err)
		return fmt.Errorf("创建流式响应失败: %v", err)
	}

	// 返回流式通道和session_id给WebSocket层处理
	// WebSocket层负责具体的消息发送和流式处理
	return s.processWebSocketStreamResponse(streamChan, sessionID, requestID, sendMessage, sendChunk)
}

// processWebSocketStreamResponse 处理流式响应（内部方法）
func (s *SilicoIDInterceptor) processWebSocketStreamResponse(streamChan chan string, sessionID string, requestID string, sendMessage func(messageType string, data map[string]interface{}) error, sendChunk func(chunk string) error) error {
	logger.Printf("[%s] 开始处理流式响应", requestID)

	// 发送session_id给前端
	sessionData := map[string]interface{}{
		"type":       "session_id",
		"session_id": sessionID,
	}
	if err := sendMessage("session_id", sessionData); err != nil {
		logger.Printf("[%s] 发送session_id失败: %v", requestID, err)
		return fmt.Errorf("发送session_id失败: %v", err)
	}
	logger.Printf("[%s] 已发送session_id: %s", requestID, sessionID)

	// 处理流式响应
	var fullResponse strings.Builder
	for chunk := range streamChan {
		fullResponse.WriteString(chunk)

		// 发送chunk给前端
		if err := sendChunk(chunk); err != nil {
			logger.Printf("[%s] 发送chunk失败: %v", requestID, err)
			return fmt.Errorf("发送chunk失败: %v", err)
		}
	}

	// 发送完成消息
	doneData := map[string]interface{}{
		"type":      "chat_done",
		"timestamp": time.Now().Unix(),
	}
	if err := sendMessage("chat_done", doneData); err != nil {
		logger.Printf("[%s] 发送完成消息失败: %v", requestID, err)
		return fmt.Errorf("发送完成消息失败: %v", err)
	}

	return nil
}

