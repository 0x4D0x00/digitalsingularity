package interceptor

import (
	"fmt"
	"io"
	"net/http"
	"github.com/gin-gonic/gin"
)

// CreateHTTPStreamResponse 创建HTTP流式AI响应
func (s *SilicoIDInterceptor) CreateHTTPStreamResponse(c *gin.Context, requestID string, userID string, data map[string]interface{}) (chan string, error) {
	// 获取模型名称
	modelName, ok := data["model"].(string)
	if !ok {
		return nil, fmt.Errorf("缺少模型参数")
	}

	// 根据model_name查询模型配置（获取base_url和endpoint）
	modelConfig, err := s.modelManager.GetModelConfig(modelName)
	if err != nil {
		logger.Printf("[%s] ⚠️ 获取模型配置失败: %v", requestID, err)
		return nil, fmt.Errorf("获取模型配置失败: %v", err)
	}

	// 将获取到的配置存储到requestData中
	modelCode := modelConfig.ModelCode
	baseURL := modelConfig.BaseURL
	endpoint := modelConfig.Endpoint
	data["model_code"] = modelCode
	data["_base_url"] = baseURL
	data["_endpoint"] = endpoint
	logger.Printf("[%s] ✅ 成功获取模型配置: model_code=%s, base_url=%s, endpoint=%s",
		requestID, modelCode, baseURL, endpoint)

	// 判断是否是 Claude 模型
	isClaudeModel := s.isClaudeModel(modelName)
	logger.Printf("[%s] 创建HTTP流式响应，模型: %s, Claude模型: %v", requestID, modelName, isClaudeModel)

	// 将 userId 添加到请求数据中，供 formatconverter 使用
	data["_user_id"] = userID

	// 根据模型类型选择服务
	var streamChan chan string

	if isClaudeModel {
		// 处理Claude流式请求
		logger.Printf("[%s] 处理Claude流式请求", requestID)

		// 检查并纠正模型名称
		if modelName == "claude-3-7-sonnet-20250222" {
			data["model"] = "claude-3-7-sonnet-20250219"
			logger.Printf("[%s] 模型名称更正: claude-3-7-sonnet-20250222 -> claude-3-7-sonnet-20250219", requestID)
		}

		// 处理思考模式参数
		thinkingEnabled, _ := data["thinking_enabled"].(bool)
		if thinkingEnabled {
			thinkingBudget := 16000
			if budget, ok := data["thinking_budget"].(float64); ok {
				thinkingBudget = int(budget)
			}
			logger.Printf("[%s] 已启用Claude思考模式，预算令牌数: %d", requestID, thinkingBudget)
		}

		// 将OpenAI格式转换为Claude格式
		claudeData, err := s.formatConverter.RequestOpenAIToClaude(data)
		if err != nil {
			logger.Printf("[%s] OpenAI转Claude格式转换失败: %v", requestID, err)
			return nil, fmt.Errorf("模型格式转换失败: %v", err)
		}
		logger.Printf("[%s] 请求已转换为Claude格式", requestID)

		// 使用通道来接收流式响应
		claudeStream := s.claudeService.CreateChatCompletionStream(c.Request.Context(), claudeData)

		// 将Claude流式响应转换为OpenAI格式
		openaiStream, err := s.formatConverter.HandleResponseClaudeStream(claudeStream)
		if err != nil {
			logger.Printf("[%s] Claude流转OpenAI流格式转换失败: %v", requestID, err)
			return nil, fmt.Errorf("流格式转换失败: %v", err)
		}

		streamChan = openaiStream
	} else {
		// 处理OpenAI流式请求
		logger.Printf("[%s] 处理OpenAI流式请求", requestID)

		// 使用格式转换器规范化 OpenAI 请求
		// 这会处理：1) system prompt 的注入和拼接  2) 将数组格式的 content 转换为字符串
		normalizedData, err := s.formatConverter.NormalizeOpenAIRequest(data)
		if err != nil {
			logger.Printf("[%s] OpenAI 请求规范化失败: %v", requestID, err)
			return nil, fmt.Errorf("请求格式规范化失败: %v", err)
		}
		logger.Printf("[%s] OpenAI 请求已规范化", requestID)

		// 使用规范化后的数据
		data = normalizedData

		// 使用通道来接收流式响应
		streamChan = s.openaiService.CreateChatCompletionStream(c.Request.Context(), data)
	}

	// 包装流式响应 channel，检测并处理错误（特别是 401 认证错误）
	wrappedChan := wrapStreamResponseWithErrorDetection(streamChan, modelName, requestID)

	return wrappedChan, nil
}


// HandleHTTPRequestStream 处理所有模型的HTTP流式请求
func (s *SilicoIDInterceptor) HandleHTTPRequestStream(c *gin.Context, requestID string, userID string, data map[string]interface{}) {
	logger.Printf("[%s] 处理HTTP流式请求", requestID)

	// 创建AI流式响应
	streamChan, err := s.CreateHTTPStreamResponse(c, requestID, userID, data)
	if err != nil {
		logger.Printf("[%s] 创建HTTP流式响应失败: %v", requestID, err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": gin.H{
				"message": "创建流式响应失败: " + err.Error(),
				"type":    "internal_error",
				"code":    "stream_response_creation_failed",
			},
		})
		return
	}

	// 设置HTTP头并处理流式响应
	err = s.processHTTPStreamResponse(c, streamChan, requestID)
	if err != nil {
		logger.Printf("[%s] 处理HTTP流式响应失败: %v", requestID, err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": gin.H{
				"message": "处理流式响应失败: " + err.Error(),
				"type":    "internal_error",
				"code":    "stream_response_processing_failed",
			},
		})
		return
	}
}

// processHTTPStreamResponse 处理HTTP流式响应（内部方法）
func (s *SilicoIDInterceptor) processHTTPStreamResponse(c *gin.Context, streamChan chan string, requestID string) error {
	logger.Printf("[%s] 开始处理HTTP流式响应", requestID)

	// 设置HTTP头
	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")

	// 流式输出响应
	logger.Printf("[%s] 开始流式输出", requestID)
	c.Stream(func(w io.Writer) bool {
		chunk, ok := <-streamChan
		if !ok {
			return false
		}
		// 直接写入SSE数据
		c.Writer.WriteString(chunk)
		c.Writer.Flush()
		return true
	})
	logger.Printf("[%s] HTTP流式请求处理完成", requestID)

	return nil
}
