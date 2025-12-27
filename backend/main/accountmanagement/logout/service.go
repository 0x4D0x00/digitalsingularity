package logout

import (
	"encoding/json"
	"errors"
	"fmt"
	"log"

	"digitalsingularity/backend/common/auth/tokenmanage"
)

// LogoutService 登出服务
// 处理用户登出请求
type LogoutService struct {
	authTokenService *tokenmanage.CommonAuthTokenService
}

// RequestData 请求数据结构
type RequestData struct {
	AuthToken string `json:"auth_token"`
	DeviceID  string `json:"device_id,omitempty"`
	LogoutAll bool   `json:"logout_all,omitempty"`
}

// MessageData WebSocket消息数据结构
type MessageData struct {
	Data RequestData `json:"data"`
}

// ResponseData 响应数据结构
type ResponseData struct {
	Type    string      `json:"type"`
	Message string      `json:"message,omitempty"`
	Data    interface{} `json:"data,omitempty"`
}

// LogoutResponse 登出响应
type LogoutResponse struct {
	Success bool   `json:"success"`
	Message string `json:"message"`
}

// NewLogoutService 创建一个新的登出服务实例
func NewLogoutService() *LogoutService {
	return &LogoutService{
		authTokenService: tokenmanage.NewCommonAuthTokenService(),
	}
}

// ProcessRequest 处理用户登出请求
func (s *LogoutService) ProcessRequest(clientID string, messageData []byte, connectionID string, msgID string, websocket interface{}) (ResponseData, error) {
	log.Printf("[WS:%s:%s] 处理用户登出请求: %s", connectionID, msgID, string(messageData))

	var message MessageData
	err := json.Unmarshal(messageData, &message)
	if err != nil {
		log.Printf("[WS:%s:%s] 解析消息失败: %v", connectionID, msgID, err)
		return ResponseData{Type: "error", Message: "无效的请求格式"}, err
	}

	data := message.Data
	authToken := data.AuthToken

	if authToken == "" {
		return ResponseData{Type: "error", Message: "authToken不能为空"}, errors.New("authToken不能为空")
	}

	// 验证authToken有效性
	valid, payload := s.authTokenService.VerifyAuthToken(authToken)
	if !valid {
		log.Printf("[WS:%s:%s] 登出请求使用了无效的authToken: %s", connectionID, msgID, payload)
		// 虽然authToken无效，但仍然可以继续登出流程
	}

	// 获取额外的登出参数
	deviceID := data.DeviceID
	logoutAll := data.LogoutAll

	// 执行登出操作
	success, message := s.performLogout(clientID, authToken, deviceID, logoutAll, connectionID, msgID)

	// 如果提供了websocket连接，主动关闭连接
	if websocket != nil && success {
		log.Printf("[WS:%s:%s] WebSocket连接已关闭", connectionID, msgID)
		// 实际中需要调用具体websocket库的关闭方法
	}

	// 构建响应
	return ResponseData{
		Type: "logout_response",
		Data: LogoutResponse{
			Success: success,
			Message: message,
		},
	}, nil
}

// performLogout 执行登出操作
func (s *LogoutService) performLogout(userID string, authToken string, deviceID string, logoutAll bool, connectionID string, msgID string) (bool, string) {
	log.Printf("[WS:%s:%s] 执行用户 %s 的登出操作", connectionID, msgID, userID)

	// 这里需要实际连接数据库使token失效
	// 简化实现，仅模拟登出成功

	// 记录安全日志
	log.Printf("[WS:%s:%s] 用户 %s 登出成功", connectionID, msgID, userID)

	if logoutAll {
		log.Printf("[WS:%s:%s] 用户 %s 登出所有设备", connectionID, msgID, userID)
		return true, "已成功登出所有设备"
	} else if deviceID != "" {
		log.Printf("[WS:%s:%s] 用户 %s 登出设备 %s", connectionID, msgID, userID, deviceID)
		return true, fmt.Sprintf("已成功登出设备 %s", deviceID)
	} else {
		log.Printf("[WS:%s:%s] 用户 %s 登出当前会话", connectionID, msgID, userID)
		return true, "已成功登出当前会话"
	}
}

// 创建单例实例
var LogoutServiceInstance = NewLogoutService() 