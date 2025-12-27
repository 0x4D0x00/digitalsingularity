package websocket

import (
	"encoding/json"
	"fmt"
	"sync"

	"github.com/gorilla/websocket"
	"digitalsingularity/backend/speechsystem/interceptor"
	"digitalsingularity/backend/speechsystem/formatconverter"
)

// SpeechSession 语音会话状态
type SpeechSession struct {
	Active            bool
	Model             string
	Interceptor       interceptor.SpeechInterceptor
	Converter         formatconverter.FormatConverter
	ConnectionID      string
	UserID            string
	mu                sync.Mutex
}

// 全局语音会话存储
var speechSessions = make(map[*websocket.Conn]*SpeechSession)
var speechSessionsMu sync.RWMutex

// getSpeechSession 获取或创建语音会话
func getSpeechSession(conn *websocket.Conn, connectionID, userID string) *SpeechSession {
	speechSessionsMu.Lock()
	defer speechSessionsMu.Unlock()

	session, exists := speechSessions[conn]
	if !exists {
		session = &SpeechSession{
			ConnectionID: connectionID,
			UserID:       userID,
		}
		speechSessions[conn] = session
	}
	return session
}

// removeSpeechSession 移除语音会话
func removeSpeechSession(conn *websocket.Conn) {
	speechSessionsMu.Lock()
	defer speechSessionsMu.Unlock()
	delete(speechSessions, conn)
}

// handleSpeechInteractionStart 处理语音交互开始
func handleSpeechInteractionStart(conn *websocket.Conn, connectionID, userID string, data map[string]interface{}) error {
	session := getSpeechSession(conn, connectionID, userID)
	
	session.mu.Lock()
	defer session.mu.Unlock()

	// 如果已经有活跃会话，先停止
	if session.Active && session.Interceptor != nil {
		session.Interceptor.Stop()
	}

	// 获取模型参数（默认值）
	model := "claude-3-7-sonnet-20250219"
	if modelVal, ok := data["model"].(string); ok {
		model = modelVal
	}

	// 创建拦截器服务（拦截器内部会根据连接成功的提供商创建对应的格式转换器）
	interceptorService := interceptor.NewInterceptorService()
	interceptorInstance, err := interceptorService.CreateInterceptor()
	if err != nil {
		logger.Printf("[WS:%s] 创建语音拦截器失败: %v", connectionID, err)
		return fmt.Errorf("创建语音拦截器失败: %v", err)
	}

	// 创建格式转换器服务（用于转换识别结果）
	converterService := formatconverter.NewFormatConverterService()
	converter := converterService.GetConverter(interceptorInstance.GetProvider())

	// 初始化会话
	session.Active = true
	session.Model = model
	session.Interceptor = interceptorInstance
	session.Converter = converter

	// 设置识别结果回调，将结果发送给客户端
	callback := &recognitionCallback{
		conn:         conn,
		connectionID: connectionID,
	}
	interceptorInstance.SetCallback(callback)

	logger.Printf("[WS:%s] 语音会话已开始，模型: %s, 提供商: %s", connectionID, model, interceptorInstance.GetProvider())

	// 发送响应（明文，不加密）
	response := map[string]interface{}{
		"type":    "system",
		"message": "语音会话已开始",
	}
	responseBytes, _ := json.Marshal(response)
	return conn.WriteMessage(websocket.TextMessage, responseBytes)
}

// handleSpeechInteractionEnd 处理语音交互结束
func handleSpeechInteractionEnd(conn *websocket.Conn, connectionID, userID string) error {
	session := getSpeechSession(conn, connectionID, userID)
	
	session.mu.Lock()
	defer session.mu.Unlock()

	if !session.Active {
		logger.Printf("[WS:%s] 收到语音会话结束消息，但没有活跃的语音会话", connectionID)
		response := map[string]interface{}{
			"type":    "error",
			"message": "没有活跃的语音会话",
		}
		responseBytes, _ := json.Marshal(response)
		return conn.WriteMessage(websocket.TextMessage, responseBytes)
	}

	logger.Printf("[WS:%s] 结束语音会话", connectionID)

	// 停止拦截器
	if session.Interceptor != nil {
		if err := session.Interceptor.Stop(); err != nil {
			logger.Printf("[WS:%s] 停止语音拦截器失败: %v", connectionID, err)
		}
	}

	// 重置会话状态
	session.Active = false
	session.Interceptor = nil
	session.Converter = nil

	// 发送响应（明文，不加密）
	response := map[string]interface{}{
		"type":    "system",
		"message": "语音会话已结束",
	}
	responseBytes, _ := json.Marshal(response)
	return conn.WriteMessage(websocket.TextMessage, responseBytes)
}

// handleSpeechInteractionCancel 处理语音交互取消
func handleSpeechInteractionCancel(conn *websocket.Conn, connectionID, userID string) error {
	session := getSpeechSession(conn, connectionID, userID)
	
	session.mu.Lock()
	defer session.mu.Unlock()

	if !session.Active {
		logger.Printf("[WS:%s] 收到语音会话取消消息，但没有活跃的语音会话", connectionID)
		response := map[string]interface{}{
			"type":    "error",
			"message": "没有活跃的语音会话",
		}
		responseBytes, _ := json.Marshal(response)
		return conn.WriteMessage(websocket.TextMessage, responseBytes)
	}

	logger.Printf("[WS:%s] 取消语音会话", connectionID)

	// 停止拦截器
	if session.Interceptor != nil {
		if err := session.Interceptor.Stop(); err != nil {
			logger.Printf("[WS:%s] 停止语音拦截器失败: %v", connectionID, err)
		}
	}

	// 重置会话状态
	session.Active = false
	session.Interceptor = nil
	session.Converter = nil

	// 发送响应（明文，不加密）
	response := map[string]interface{}{
		"type":    "system",
		"message": "语音会话已取消",
	}
	responseBytes, _ := json.Marshal(response)
	return conn.WriteMessage(websocket.TextMessage, responseBytes)
}

// handleSpeechAudioData 处理语音音频数据（二进制）
func handleSpeechAudioData(conn *websocket.Conn, connectionID, userID string, audioData []byte) error {
	speechSessionsMu.RLock()
	session, exists := speechSessions[conn]
	speechSessionsMu.RUnlock()

	if !exists || !session.Active {
		logger.Printf("[WS:%s] 收到音频数据但没有活跃的语音会话", connectionID)
		return nil
	}

	session.mu.Lock()
	defer session.mu.Unlock()

	if session.Interceptor == nil {
		logger.Printf("[WS:%s] 会话未完全初始化", connectionID)
		return nil
	}

	// 直接将音频数据写入拦截器（拦截器内部会自动进行格式转换）
	if err := session.Interceptor.Write(audioData); err != nil {
		logger.Printf("[WS:%s] 写入音频数据到拦截器失败: %v", connectionID, err)
		return err
	}

	logger.Printf("[WS:%s] 处理音频数据，长度: %d", connectionID, len(audioData))
	return nil
}

// recognitionCallback 识别结果回调实现
// 将识别结果发送给客户端
type recognitionCallback struct {
	conn         *websocket.Conn
	connectionID string
}

// OnSentenceBegin 句子开始回调
func (c *recognitionCallback) OnSentenceBegin() {
	logger.Printf("[WS:%s] 句子开始", c.connectionID)
	response := map[string]interface{}{
		"type":  "sentence_state",
		"state": true,
	}
	responseBytes, _ := json.Marshal(response)
	c.conn.WriteMessage(websocket.TextMessage, responseBytes)
}

// OnPartialResult 部分识别结果回调（识别过程中实时更新）
func (c *recognitionCallback) OnPartialResult(text string) {
	logger.Printf("[WS:%s] 部分识别结果: %s", c.connectionID, text)
	response := map[string]interface{}{
		"type":   "partial",
		"result": text,
	}
	responseBytes, _ := json.Marshal(response)
	c.conn.WriteMessage(websocket.TextMessage, responseBytes)
}

// OnSentenceEnd 句子结束回调（一句话识别完成）
func (c *recognitionCallback) OnSentenceEnd(text string) {
	logger.Printf("[WS:%s] 句子结束，最终识别结果: %s", c.connectionID, text)
	response := map[string]interface{}{
		"type":   "final",
		"result": text,
	}
	responseBytes, _ := json.Marshal(response)
	c.conn.WriteMessage(websocket.TextMessage, responseBytes)
	
	// 同时发送句子结束状态
	stateResponse := map[string]interface{}{
		"type":  "sentence_state",
		"state": false,
	}
	stateResponseBytes, _ := json.Marshal(stateResponse)
	c.conn.WriteMessage(websocket.TextMessage, stateResponseBytes)
}

// OnError 识别错误回调
func (c *recognitionCallback) OnError(err error) {
	logger.Printf("[WS:%s] 识别错误: %v", c.connectionID, err)
	response := map[string]interface{}{
		"type":    "error",
		"message": err.Error(),
	}
	responseBytes, _ := json.Marshal(response)
	c.conn.WriteMessage(websocket.TextMessage, responseBytes)
}
