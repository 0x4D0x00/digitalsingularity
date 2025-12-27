package http

import (
	"net/http"
	"strconv"
	"time"

	"digitalsingularity/backend/silicoid/interceptor"
)

// SilicoID OpenAI兼容接口 - 获取模型列表
func silicoidModels(w http.ResponseWriter, r *http.Request) {
	requestID := strconv.FormatInt(time.Now().UnixNano()/1e6, 10)
	logger.Printf("[%s] 收到获取OpenAI格式模型列表请求", requestID)

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

	// 设置响应头
	w.Header().Set("Content-Type", "application/json")

	// 调用SilicoID拦截器处理请求
	interceptorService.HandleModels(createGinContext(w, r))
}

// SilicoidModels 导出的模型列表处理函数
func SilicoidModels(w http.ResponseWriter, r *http.Request) {
	silicoidModels(w, r)
}
