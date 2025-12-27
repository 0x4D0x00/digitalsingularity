package websocket

import (
	"context"
	"crypto/rsa"
	"crypto/x509"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"regexp"
	"encoding/pem"
	"fmt"
	"strings"
	"time"

	"github.com/gorilla/websocket"
	"digitalsingularity/backend/common/security/asymmetricencryption/encrypt"
)

// ttsCallback TTS 合成结果回调实现
// 将合成结果通过 WebSocket 发送给客户端
type ttsCallback struct {
	conn         *websocket.Conn
	connectionID string
	userPublicKey string
}

// OnAudioChunk 流式音频数据块回调
func (c *ttsCallback) OnAudioChunk(audioData []byte) {
	// Base64 编码音频数据
	audioBase64 := base64.StdEncoding.EncodeToString(audioData)
	
	response := map[string]interface{}{
		"type": "speech_synthesis_audio",
		"data": map[string]interface{}{
			"audio_data": audioBase64,
			"chunk":      true, // 标识这是流式数据块
		},
	}
	
	responseBytes, _ := json.Marshal(response)
	c.conn.WriteMessage(websocket.TextMessage, responseBytes)
}

// OnSynthesisComplete 合成完成回调
func (c *ttsCallback) OnSynthesisComplete(audioData []byte) {
	response := map[string]interface{}{
		"type": "speech_synthesis_complete",
	}
	
	// 如果音频数据不为空，则包含音频数据（用于非流式合成）
	// 如果为空，则只发送完成信号（用于流式合成，避免重复播放）
	if audioData != nil && len(audioData) > 0 {
		// Base64 编码音频数据
		audioBase64 := base64.StdEncoding.EncodeToString(audioData)
		response["data"] = map[string]interface{}{
			"audio_data": audioBase64,
		}
	} else {
		// 流式合成完成，只发送完成信号，不包含音频数据
		response["data"] = map[string]interface{}{
			"complete": true,
		}
	}
	
	responseBytes, _ := json.Marshal(response)
	c.conn.WriteMessage(websocket.TextMessage, responseBytes)
}

// OnError 合成错误回调
func (c *ttsCallback) OnError(err error) {
	logger.Printf("[TTS:%s] 合成错误: %v", c.connectionID, err)
	response := map[string]interface{}{
		"type":    "speech_synthesis_error",
		"message": err.Error(),
	}
	responseBytes, _ := json.Marshal(response)
	c.conn.WriteMessage(websocket.TextMessage, responseBytes)
}

// extractSessionIDFromChunk 从流式数据块中提取 session_id
// 流式响应可能是 JSON 格式，例如：{"id":"...","session_id":"..."}
// 或者可能是 SSE 格式：data: {"id":"...","session_id":"..."}
func extractSessionIDFromChunk(chunk string) string {
	if chunk == "" {
		return ""
	}
	
	// 尝试直接解析 JSON
	var data map[string]interface{}
	if err := json.Unmarshal([]byte(chunk), &data); err == nil {
		if sessionID, ok := data["session_id"].(string); ok && sessionID != "" {
			return sessionID
		}
	}
	
	// 尝试解析 SSE 格式（data: {...}）
	if strings.HasPrefix(chunk, "data: ") {
		jsonPart := strings.TrimPrefix(chunk, "data: ")
		jsonPart = strings.TrimSpace(jsonPart)
		if jsonPart != "[DONE]" {
			var data map[string]interface{}
			if err := json.Unmarshal([]byte(jsonPart), &data); err == nil {
				if sessionID, ok := data["session_id"].(string); ok && sessionID != "" {
					return sessionID
				}
			}
		}
	}
	
	// 尝试在文本中查找 session_id（可能是部分 JSON）
	// 查找 "session_id":"..." 模式
	if idx := strings.Index(chunk, `"session_id"`); idx >= 0 {
		// 提取 session_id 后面的值
		start := idx + len(`"session_id"`)
		// 跳过可能的空格和冒号
		for start < len(chunk) && (chunk[start] == ' ' || chunk[start] == ':') {
			start++
		}
		// 跳过引号
		if start < len(chunk) && chunk[start] == '"' {
			start++
			// 查找结束引号
			end := start
			for end < len(chunk) && chunk[end] != '"' {
				end++
			}
			if end < len(chunk) {
				sessionID := chunk[start:end]
				if sessionID != "" {
					return sessionID
				}
			}
		}
	}
	
	return ""
}

// extractClientExecutorCallsFromText 从文本中提取所有 client_executor 调用（保持原样）
// 匹配包含标准的 function_call 或 tool_calls 的 JSON 对象
func extractClientExecutorCallsFromText(text string) []map[string]interface{} {
	var calls []map[string]interface{}
	if text == "" {
		return calls
	}

	// 匹配可能包含 client_executor 的 JSON 对象（非贪婪）
	re := regexp.MustCompile("(?s)\\{.*?\"type\"\\s*:\\s*\"client_executor(?:_call)?\".*?\\}")
	matches := re.FindAllString(text, -1)
	for _, m := range matches {
		var obj map[string]interface{}
		if err := json.Unmarshal([]byte(m), &obj); err == nil {
			calls = append(calls, obj)
		}
	}
	return calls
}

// prepareChatRequestData 准备聊天请求数据
func prepareChatRequestData(messageData map[string]interface{}, userID string) (map[string]interface{}, error) {
	// 解析data字段
	var chatData map[string]interface{}

	if dataStr, ok := messageData["data"].(string); ok {
		if err := json.Unmarshal([]byte(dataStr), &chatData); err != nil {
			return nil, fmt.Errorf("解析聊天数据失败: %v", err)
		}
	} else if dataMap, ok := messageData["data"].(map[string]interface{}); ok {
		chatData = dataMap
	} else {
		chatData = messageData
	}

	// 提取必要字段
	model, _ := chatData["model"].(string)
	if model == "" {
		model = "deepseek-chat"
	}

	messages, ok := chatData["messages"].([]interface{})
	if !ok {
		return nil, fmt.Errorf("消息格式错误：缺少messages字段")
	}

	// 获取模型配置
	modelConfig, err := websocketInterceptorService.GetModelConfig(model)
	if err != nil || modelConfig == nil {
		return nil, fmt.Errorf("模型配置不存在: %s", model)
	}

	// 构造请求数据
	requestData := map[string]interface{}{
		"model":      model,
		"model_code": modelConfig.ModelCode,
		"messages":   messages,
		"user_id":    userID,
		"_user_id":   userID,
	}

	// 复制其他可选参数
	if roleName, ok := chatData["role_name"].(string); ok {
		requestData["role_name"] = roleName
	}

	if enableDeepThinking, ok := chatData["enable_deep_thinking"].(bool); ok && enableDeepThinking {
		requestData["thinking_enabled"] = true
	}

	return requestData, nil
}

// toAIProcessNonStreamingChat 发送非流式聊天请求给AI处理
func toAIProcessNonStreamingChat(conn *websocket.Conn, userID string, messageData map[string]interface{}, userPublicKey string) {
	// 添加 panic 恢复机制
	defer func() {
		if r := recover(); r != nil {
			logger.Printf("❌ [PANIC] processNonStreamingAIChat 发生 panic: %v", r)
			toClientChatError(conn, fmt.Sprintf("服务器内部错误: %v", r), userPublicKey)
		}
	}()
	
	logger.Printf("开始处理用户 %s 的非流式聊天消息", userID)

	// 检查 userID 是否为空
	if userID == "" {
		logger.Printf("❌ 错误: userID 参数为空")
		toClientChatError(conn, "用户ID为空，无法处理聊天消息", userPublicKey)
		return
	}

	// 解析并构造请求数据
	requestData, err := prepareChatRequestData(messageData, userID)
	if err != nil {
		logger.Printf("准备聊天请求数据失败: %v", err)
		toClientChatError(conn, err.Error(), userPublicKey)
		return
	}

	// 设置为非流式
	requestData["stream"] = false

	// 创建消息发送函数
	sendMessage := func(messageType string, data map[string]interface{}) error {
		return toClientChatResponse(conn, data, userPublicKey)
	}

	// 调用interceptor处理AI聊天
	requestID := fmt.Sprintf("CHAT_%s_%d", userID, time.Now().UnixNano())
	if err := websocketInterceptorService.ProcessNonStreamChat(
		context.Background(),
		requestID,
		userID,
		requestData,
		sendMessage,
	); err != nil {
		logger.Printf("[%s] 处理AI聊天失败: %v", requestID, err)
		toClientChatError(conn, fmt.Sprintf("处理AI聊天失败: %v", err), userPublicKey)
	}
}


// ===== AI 聊天处理函数 =====
// toAIProcessStreamingChat 发送流式聊天请求给AI处理
func toAIProcessStreamingChat(conn *websocket.Conn, userID string, requestData map[string]interface{}, userPublicKey string, enableTTS bool, voiceGender string) {
	// 添加 panic 恢复机制
	defer func() {
		if r := recover(); r != nil {
			logger.Printf("❌ [PANIC] processStreamingAIChat 发生 panic: %v", r)
			toClientChatError(conn, fmt.Sprintf("AI处理过程中发生错误: %v", r), userPublicKey)
		}
	}()

	logger.Printf("开始处理用户 %s 的流式聊天消息", userID)

	// 检查 userID 是否为空
	if userID == "" {
		logger.Printf("❌ 错误: userID 参数为空")
		toClientChatError(conn, "用户ID为空，无法处理聊天消息", userPublicKey)
		return
	}

	// 设置为流式
	requestData["stream"] = true

	// 创建消息发送函数
	sendMessage := func(messageType string, data map[string]interface{}) error {
		return toClientChatResponse(conn, data, userPublicKey)
	}

	// 累积响应变量
	var fullStreamResponse strings.Builder
	var sessionIDExtracted bool
	var sessionID string

	// 创建chunk发送函数
	sendChunk := func(chunk string) error {
		// 累积响应
		fullStreamResponse.WriteString(chunk)

		// 尝试从流式数据中提取 session_id（如果还没有提取到）
		if !sessionIDExtracted {
			if extractedSessionID := extractSessionIDFromChunk(chunk); extractedSessionID != "" {
				logger.Printf("[%s] ✅ 从流式数据中提取到 session_id: %s", fmt.Sprintf("STREAM_%s_%d", userID, time.Now().UnixNano()), extractedSessionID)
				sessionID = extractedSessionID
				sessionIDExtracted = true

				// 更新并重新发送 session_id 给前端
				sessionData := map[string]interface{}{
					"type":       "session_id",
					"session_id": sessionID,
				}
				if err := toClientChatResponse(conn, sessionData, userPublicKey); err != nil {
					logger.Printf("[%s] ❌ 更新 session_id 失败: %v", fmt.Sprintf("STREAM_%s_%d", userID, time.Now().UnixNano()), err)
				} else {
					logger.Printf("[%s] ✅ 已更新 session_id 给前端: %s", fmt.Sprintf("STREAM_%s_%d", userID, time.Now().UnixNano()), sessionID)
				}
			}
		}

		// 发送流式数据块到前端
		chunkData := map[string]interface{}{
			"type":  "chat_chunk",
			"chunk": chunk,
		}

		if err := toClientChatResponse(conn, chunkData, userPublicKey); err != nil {
			logger.Printf("[%s] ❌ 发送流式数据块失败，WebSocket 可能已断开: %v", fmt.Sprintf("STREAM_%s_%d", userID, time.Now().UnixNano()), err)
			return err // 停止发送后续数据
		}
		return nil
	}

	// 调用interceptor处理流式AI聊天
	requestID := fmt.Sprintf("STREAM_%s_%d", userID, time.Now().UnixNano())
	if err := websocketInterceptorService.ProcessStreamChat(
		context.Background(),
		requestID,
		userID,
		requestData,
		sendMessage,
		sendChunk,
	); err != nil {
		logger.Printf("[%s] 处理流式AI聊天失败: %v", requestID, err)
		toClientChatError(conn, fmt.Sprintf("处理流式AI聊天失败: %v", err), userPublicKey)
		return
	}

	logger.Printf("[%s] AI流式处理完成，用户: %s", requestID, userID)
}

// toAIProcessClientExecutorResult 发送客户端执行器结果给AI处理
func toAIProcessClientExecutorResult(conn *websocket.Conn, userID string, messageData map[string]interface{}, userPublicKey string) {
	// 添加 panic 恢复机制
	defer func() {
		if r := recover(); r != nil {
			logger.Printf("❌ [PANIC] toAIProcessClientExecutorResult 发生 panic: %v", r)
			toClientChatError(conn, fmt.Sprintf("处理工具执行结果失败: %v", r), userPublicKey)
		}
	}()

	logger.Printf("开始处理用户 %s 的客户端执行器结果", userID)

	// 检查 userID 是否为空
	if userID == "" {
		logger.Printf("❌ 错误: userID 参数为空")
		toClientChatError(conn, "用户ID为空，无法处理工具执行结果", userPublicKey)
		return
	}

	// 创建消息发送函数
	sendMessage := func(messageType string, data map[string]interface{}) error {
		return toClientChatResponse(conn, data, userPublicKey)
	}

	requestID := fmt.Sprintf("CER_%s_%d", userID, time.Now().UnixNano())

	// 调用sessionmanagement处理业务逻辑
	if err := websocketInterceptorService.ProcessClientExecutorResult(
		context.Background(),
		requestID,
		userID,
		messageData,
		sendMessage,
	); err != nil {
		logger.Printf("[%s] 处理客户端执行器结果失败: %v", requestID, err)
		toClientChatError(conn, fmt.Sprintf("处理工具执行结果失败: %v", err), userPublicKey)
	}
}

// toClientChatResponse 发送聊天响应给客户端（支持加密）
func toClientChatResponse(conn *websocket.Conn, data map[string]interface{}, userPublicKey string) error {
	dataBytes, err := json.Marshal(data)
	if err != nil {
		logger.Printf("❌ 序列化响应数据失败: %v", err)
		return fmt.Errorf("序列化失败: %w", err)
	}
	
	// 如果有公钥，加密响应
	if userPublicKey != "" {
		// 将16进制编码的公钥转换为*rsa.PublicKey
		publicKeyBytes, err := hex.DecodeString(userPublicKey)
		if err != nil {
			logger.Printf("⚠️ 解码用户公钥失败: %v，发送明文", err)
			if writeErr := conn.WriteMessage(websocket.TextMessage, dataBytes); writeErr != nil {
				logger.Printf("❌ WebSocket 发送失败（明文）: %v", writeErr)
				return fmt.Errorf("发送失败: %w", writeErr)
			}
			return nil
		}
		
		block, _ := pem.Decode(publicKeyBytes)
		if block == nil {
			logger.Printf("⚠️ 用户公钥格式错误，发送明文")
			if writeErr := conn.WriteMessage(websocket.TextMessage, dataBytes); writeErr != nil {
				logger.Printf("❌ WebSocket 发送失败（明文）: %v", writeErr)
				return fmt.Errorf("发送失败: %w", writeErr)
			}
			return nil
		}
		
		parsedKey, err := x509.ParsePKIXPublicKey(block.Bytes)
		if err != nil {
			logger.Printf("⚠️ 解析用户公钥失败: %v，发送明文", err)
			if writeErr := conn.WriteMessage(websocket.TextMessage, dataBytes); writeErr != nil {
				logger.Printf("❌ WebSocket 发送失败（明文）: %v", writeErr)
				return fmt.Errorf("发送失败: %w", writeErr)
			}
			return nil
		}
		
		rsaPublicKey, ok := parsedKey.(*rsa.PublicKey)
		if !ok {
			logger.Printf("⚠️ 无效的RSA公钥，发送明文")
			if writeErr := conn.WriteMessage(websocket.TextMessage, dataBytes); writeErr != nil {
				logger.Printf("❌ WebSocket 发送失败（明文）: %v", writeErr)
				return fmt.Errorf("发送失败: %w", writeErr)
			}
			return nil
		}
		
		// 加密数据
		encryptedData, err := encrypt.AsymmetricEncryptService(string(dataBytes), rsaPublicKey)
		if err == nil {
			encryptedResp := map[string]interface{}{
				"ciphertext": encryptedData,
			}
			encryptedBytes, _ := json.Marshal(encryptedResp)
			if writeErr := conn.WriteMessage(websocket.TextMessage, encryptedBytes); writeErr != nil {
				logger.Printf("❌ WebSocket 发送失败（加密）: %v", writeErr)
				return fmt.Errorf("发送失败: %w", writeErr)
			}
			return nil
		}
		
		// 加密失败，记录日志并发送明文
		logger.Printf("⚠️ 加密响应失败: %v，发送明文", err)
	}
	
	// 发送明文
	if writeErr := conn.WriteMessage(websocket.TextMessage, dataBytes); writeErr != nil {
		logger.Printf("❌ WebSocket 发送失败（明文）: %v", writeErr)
		return fmt.Errorf("发送失败: %w", writeErr)
	}
	return nil
}

// toClientChatError 发送聊天错误消息给客户端
func toClientChatError(conn *websocket.Conn, message string, userPublicKey string) {
	errorData := map[string]interface{}{
		"type":    "chat_error",
		"message": message,
	}
	toClientChatResponse(conn, errorData, userPublicKey)
}
