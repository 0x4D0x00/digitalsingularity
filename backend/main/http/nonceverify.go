package http

import (
	"fmt"
)

// VerifyNonce 验证 nonce，完全按照 processRequest 的逻辑
// 参数:
//   - requestID: 请求ID，用于日志记录
//   - data: 请求数据 map
// 返回:
//   - error: 如果验证失败或缺少 nonce，返回错误；验证成功返回 nil
func VerifyNonce(requestID string, data map[string]interface{}) error {
	// 验证nonce
	nonce, ok := data["nonce"].(string)
	if !ok {
		logger.Printf("[%s] 请求缺少nonce参数", requestID)
		return fmt.Errorf("无效请求")
	}

	// 验证nonce有效性
	logger.Printf("[%s] 验证nonce: %s", requestID, nonce)
	valid, nonceMessage := nonceService.VerifyNonce(nonce)
	if !valid {
		logger.Printf("[%s] nonce验证失败: %s", requestID, nonceMessage)
		return fmt.Errorf("无效请求")
	}

	return nil
}
