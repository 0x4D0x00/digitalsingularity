package deactivation

import (
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"math/rand"
	"time"

	"digitalsingularity/backend/common/auth/tokenmanage"
)

// DeactivationService 账户注销服务
// 提供账户注销相关功能:
// 1. 申请注销验证码
// 2. 执行账户注销
type DeactivationService struct {
	authTokenService  *tokenmanage.CommonAuthTokenService
	verificationCodes map[string]VerificationCodeData
}

// VerificationCodeData 存储验证码及其过期时间
type VerificationCodeData struct {
	Code      string
	ExpiresAt time.Time
}

// RequestData 请求数据结构
type RequestData struct {
	Action           string `json:"action"`
	AuthToken        string `json:"auth_token"`
	VerificationCode string `json:"verification_code,omitempty"`
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

// DeactivationCodeResponse 验证码响应
type DeactivationCodeResponse struct {
	Message   string `json:"message"`
	Code      string `json:"code"`
	ExpiresIn int    `json:"expires_in"`
}

// DeactivationResponse 注销响应
type DeactivationResponse struct {
	Success bool   `json:"success"`
	Message string `json:"message"`
}

// NewDeactivationService 创建一个新的注销服务实例
func NewDeactivationService() *DeactivationService {
	return &DeactivationService{
		authTokenService:   tokenmanage.NewCommonAuthTokenService(),
		verificationCodes: make(map[string]VerificationCodeData),
	}
}

// ProcessRequest 处理账户注销请求
func (s *DeactivationService) ProcessRequest(clientID string, messageData []byte, connectionID string, msgID string, websocket interface{}) (ResponseData, error) {
	log.Printf("[WS:%s:%s] 处理账户注销请求: %s", connectionID, msgID, string(messageData))

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
		log.Printf("[WS:%s:%s] 注销请求使用了无效的authToken: %s", connectionID, msgID, payload)
		return ResponseData{Type: "error", Message: fmt.Sprintf("无效的authToken: %s", payload)}, nil
	}

	// 根据请求类型处理
	switch data.Action {
	case "request_deactivation_code":
		return s.requestDeactivationCode(clientID, connectionID, msgID)
	case "deactivate_account":
		verificationCode := data.VerificationCode
		if verificationCode == "" {
			return ResponseData{Type: "error", Message: "验证码不能为空"}, nil
		}
		return s.deactivateAccount(clientID, verificationCode, connectionID, msgID, websocket)
	default:
		return ResponseData{Type: "error", Message: fmt.Sprintf("未知的注销操作类型: %s", data.Action)}, nil
	}
}

// requestDeactivationCode 申请注销验证码
func (s *DeactivationService) requestDeactivationCode(userID string, connectionID string, msgID string) (ResponseData, error) {
	log.Printf("[WS:%s:%s] 用户 %s 申请注销验证码", connectionID, msgID, userID)

	// 生成6位数验证码
	const charset = "0123456789"
	verificationCode := make([]byte, 6)
	for i := range verificationCode {
		verificationCode[i] = charset[rand.Intn(len(charset))]
	}
	codeStr := string(verificationCode)

	// 设置过期时间(10分钟)
	expiresAt := time.Now().Add(10 * time.Minute)

	// 存储验证码
	s.verificationCodes[userID] = VerificationCodeData{
		Code:      codeStr,
		ExpiresAt: expiresAt,
	}

	log.Printf("[WS:%s:%s] 已生成用户 %s 的注销验证码: %s", connectionID, msgID, userID, codeStr)

	// 在实际应用中，应该将验证码发送到用户的手机或邮箱
	// 但为了测试，直接返回验证码
	return ResponseData{
		Type: "deactivation_code_response",
		Data: DeactivationCodeResponse{
			Message:   "注销验证码已发送",
			Code:      codeStr, // 实际应用中不应该直接返回验证码
			ExpiresIn: 600,     // 过期时间(秒)
		},
	}, nil
}

// deactivateAccount 执行账户注销
func (s *DeactivationService) deactivateAccount(userID string, verificationCode string, connectionID string, msgID string, websocket interface{}) (ResponseData, error) {
	log.Printf("[WS:%s:%s] 用户 %s 请求注销账户，验证码: %s", connectionID, msgID, userID, verificationCode)

	// 验证验证码
	codeData, exists := s.verificationCodes[userID]
	if !exists {
		log.Printf("[WS:%s:%s] 用户 %s 未申请注销验证码", connectionID, msgID, userID)
		return ResponseData{Type: "error", Message: "请先申请注销验证码"}, nil
	}

	// 验证码过期检查
	if time.Now().After(codeData.ExpiresAt) {
		log.Printf("[WS:%s:%s] 用户 %s 的注销验证码已过期", connectionID, msgID, userID)
		delete(s.verificationCodes, userID)
		return ResponseData{Type: "error", Message: "验证码已过期，请重新申请"}, nil
	}

	// 验证码匹配检查
	if verificationCode != codeData.Code {
		log.Printf("[WS:%s:%s] 用户 %s 提供的注销验证码不匹配", connectionID, msgID, userID)
		return ResponseData{Type: "error", Message: "验证码错误"}, nil
	}

	// 执行注销操作
	success, message := s.performDeactivation(userID, connectionID, msgID)

	// 注销成功后，清除验证码
	if success {
		delete(s.verificationCodes, userID)

		// 如果提供了websocket，关闭连接
		// 注意: 这里简化处理，实际实现需要根据具体的websocket库进行适配
		if websocket != nil {
			log.Printf("[WS:%s:%s] WebSocket连接已关闭", connectionID, msgID)
			// 实际中需要调用具体websocket库的关闭方法
		}
	}

	// 构建响应
	return ResponseData{
		Type: "deactivation_response",
		Data: DeactivationResponse{
			Success: success,
			Message: message,
		},
	}, nil
}

// performDeactivation 执行账户注销的实际操作
func (s *DeactivationService) performDeactivation(userID string, connectionID string, msgID string) (bool, string) {
	log.Printf("[WS:%s:%s] 执行用户 %s 的账户注销操作", connectionID, msgID, userID)

	// 在实际应用中，这里需要:
	// 1. 更新数据库，将用户账户标记为已注销
	// 2. 清理用户相关的会话和令牌
	// 3. 标记用户数据为待删除状态
	// 4. 执行可能的数据备份

	// 简化实现，仅模拟注销成功
	log.Printf("[WS:%s:%s] 用户 %s 账户注销成功", connectionID, msgID, userID)

	return true, "账户注销成功"
}

// 创建单例实例
var DeactivationServiceInstance = NewDeactivationService()

func init() {
	// 初始化随机数生成器
	rand.Seed(time.Now().UnixNano())
}
