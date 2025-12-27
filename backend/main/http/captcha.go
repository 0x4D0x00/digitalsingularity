package http

import (
	"digitalsingularity/backend/common/auth/smsverify"
)

// 处理发送验证码请求
// 注意：此函数由 processRequest 调用，解密和验证已在 handleEncryptedRequest 中完成
func handleCaptchaRequest(requestID string, data map[string]interface{}) map[string]interface{} {
	// 获取手机号
	mobileNumber, ok := data["mobile_number"].(string)
	if !ok {
		logger.Printf("[%s] 请求缺少mobile_number参数", requestID)
		return map[string]interface{}{
			"status":  "fail",
			"message": "无效请求：缺少手机号",
		}
	}

	logger.Printf("[%s] 处理发送验证码请求，手机号: %s", requestID, mobileNumber)

	// 创建短信验证服务
	smsService, err := smsverify.NewSmsVerifyService()
	if err != nil {
		logger.Printf("[%s] 创建短信服务失败: %v", requestID, err)
		return map[string]interface{}{
			"status":  "fail",
			"message": "服务暂时不可用",
		}
	}

	// 发送验证码
	success, message := smsService.GenerateVerifyCode(mobileNumber)

	if success {
		logger.Printf("[%s] 验证码发送成功", requestID)
		return map[string]interface{}{
			"status":  "success",
			"message": "验证码已发送",
		}
	} else {
		logger.Printf("[%s] 验证码发送失败: %s", requestID, message)
		return map[string]interface{}{
			"status":  "fail",
			"message": message,
		}
	}
}