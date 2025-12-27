package http

import (
	"encoding/json"
	"net/http"
	"strconv"
	"time"
)

// 获取nonce (POST方法)
func getNonce(w http.ResponseWriter, r *http.Request) {
	// 记录请求开始
	requestID := strconv.FormatInt(time.Now().UnixNano()/1e6, 10)
	clientIP := r.RemoteAddr
	logger.Printf("[%s] 收到获取nonce请求 (%s)，来自 %s", requestID, r.Method, clientIP)

	// 处理OPTIONS预检请求
	if r.Method == "OPTIONS" {
		logger.Printf("[%s] 处理OPTIONS预检请求", requestID)
		buildCorsPreflightResponse(w)
		return
	}

	// 设置响应头
	w.Header().Set("Content-Type", "application/json")

	// 生成nonce
	nonceStr := nonceService.GenerateNonce()
	logger.Printf("[%s] 生成nonce成功: %s", requestID, nonceStr)

	// 返回nonce
	json.NewEncoder(w).Encode(map[string]interface{}{
		"status": "success",
		"data": map[string]string{
			"nonce": nonceStr,
		},
	})
}