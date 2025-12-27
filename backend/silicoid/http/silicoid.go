package http

import (
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"digitalsingularity/backend/silicoid/interceptor"
)

// silicoidChatCompletions 适配器函数，用于路由处理
func silicoidChatCompletions(w http.ResponseWriter, r *http.Request) {
	// 生成请求ID
	requestID := strconv.FormatInt(time.Now().UnixNano()/1e6, 10)
	logger.Printf("[%s] 收到聊天完成请求", requestID)

	// 确保拦截器已初始化（线程安全）
	interceptorOnce.Do(func() {
		interceptorService = interceptor.CreateInterceptor()
		logger.Printf("✅ SilicoID拦截器初始化成功")
	})

	// 解析请求体
	var data map[string]interface{}
	if err := json.NewDecoder(r.Body).Decode(&data); err != nil {
		logger.Printf("[%s] 解析请求体失败: %v", requestID, err)
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte(`{"error":{"message":"无效的JSON格式","type":"invalid_request","code":"invalid_request"}}`))
		return
	}

	SilicoidChatCompletions(w, r, data, requestID)
}
	
// SilicoidChatCompletions直接处理聊天完成请求（不通过service.go的中间层）
func SilicoidChatCompletions(w http.ResponseWriter, r *http.Request, data map[string]interface{}, requestID string) {
	logger.Printf("[%s] 直接处理Silicoid聊天完成请求", requestID)

	// 确保拦截器已初始化（线程安全）
	interceptorOnce.Do(func() {
		interceptorService = interceptor.CreateInterceptor()
		if interceptorService == nil {
			logger.Printf("❌ SilicoID拦截器初始化失败")
			w.WriteHeader(http.StatusInternalServerError)
			w.Write([]byte(`{"error":{"message":"服务初始化失败","type":"internal_error","code":"service_initialization_failed"}}`))
			return
		}
		logger.Printf("✅ SilicoID拦截器初始化成功")
	})

	// 确保拦截器服务可用
	if interceptorService == nil {
		logger.Printf("[%s] 拦截器服务不可用", requestID)
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(`{"error":{"message":"服务不可用","type":"internal_error","code":"service_unavailable"}}`))
		return
	}

	// 提取实际的数据（data字段可能是JSON字符串或对象）
	var actualData map[string]interface{}
	if dataStr, ok := data["data"].(string); ok {
		// data 是字符串，需要解析
		if err := json.Unmarshal([]byte(dataStr), &actualData); err != nil {
			logger.Printf("[%s] 解析data字段失败: %v", requestID, err)
			w.WriteHeader(http.StatusBadRequest)
			w.Write([]byte(`{"error":{"message":"无效的请求数据格式","type":"invalid_request","code":"invalid_request"}}`))
			return
		}
	} else if dataObj, ok := data["data"].(map[string]interface{}); ok {
		// data 是对象，直接使用
		actualData = dataObj
	} else {
		// data 字段不存在或格式不对，尝试直接使用 data 本身
		actualData = data
	}

	// 检查是否是流式请求
	streamValue := actualData["stream"]
	stream, _ := streamValue.(bool)

	// 创建Gin上下文
	ginCtx := createGinContext(w, r)

	logger.Printf("[%s] 请求 stream 参数: %v", requestID, stream)

	// 根据stream参数直接调用对应的处理方法
	if stream {
		// 处理流式请求
		logger.Printf("[%s] 处理HTTP流式请求", requestID)
		interceptorService.HandleHTTPRequestStream(ginCtx, requestID, "", actualData)
	} else {
		// 处理非流式请求
		logger.Printf("[%s] 处理HTTP非流式请求", requestID)
		interceptorService.HandleHTTPRequestNonStream(ginCtx, requestID, "", actualData)
	}
}
