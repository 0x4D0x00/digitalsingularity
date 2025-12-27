package http

import (
	"encoding/json"
	"net/http"
	"strconv"
	"time"
)

// 获取服务器公钥
func getServerPublicKey(w http.ResponseWriter, r *http.Request) {
	// 记录请求开始
	requestID := strconv.FormatInt(time.Now().UnixNano()/1e6, 10)
	clientIP := r.RemoteAddr
	logger.Printf("[%s] 收到获取服务器公钥请求，来自 %s", requestID, clientIP)

	// 处理OPTIONS预检请求
	if r.Method == "OPTIONS" {
		logger.Printf("[%s] 处理OPTIONS预检请求", requestID)
		buildCorsPreflightResponse(w)
		return
	}

	// 设置响应头
	w.Header().Set("Content-Type", "application/json")

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

	// 返回公钥
	json.NewEncoder(w).Encode(map[string]string{
		"status":          "success",
		"serverPublicKey": serverPublicKey,
	})
}