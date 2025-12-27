package modification

import (
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"math/rand"
	"regexp"
	"time"

	"digitalsingularity/backend/common/auth/tokenmanage"
)

// ModificationService 账户信息修改服务
// 提供修改用户信息的功能:
// 1. 修改用户名
// 2. 修改昵称
// 3. 修改手机号码
// 4. 修改邮箱
type ModificationService struct {
	authTokenService   *tokenmanage.CommonAuthTokenService
	verificationCodes map[string]VerificationCodeData
}

// VerificationCodeData 存储验证码及其过期时间
type VerificationCodeData struct {
	Type      string
	Code      string
	Value     string
	ExpiresAt time.Time
}

// RequestData 请求数据结构
type RequestData struct {
	Type            string `json:"type"`
	Action          string `json:"action"`
	AuthToken       string `json:"auth_token"`
	NewValue        string `json:"new_value,omitempty"`
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

// VerificationCodeResponse 验证码响应
type VerificationCodeResponse struct {
	Message   string `json:"message"`
	Code      string `json:"code"`
	ExpiresIn int    `json:"expires_in"`
}

// ModificationResponse 修改响应
type ModificationResponse struct {
	Success bool   `json:"success"`
	Message string `json:"message"`
}

// NewModificationService 创建一个新的账户信息修改服务实例
func NewModificationService() *ModificationService {
	return &ModificationService{
		authTokenService:   tokenmanage.NewCommonAuthTokenService(),
		verificationCodes: make(map[string]VerificationCodeData),
	}
}

// ProcessRequest 处理账户信息修改请求
func (s *ModificationService) ProcessRequest(clientID string, messageData []byte, connectionID string, msgID string) (ResponseData, error) {
	log.Printf("[WS:%s:%s] 处理账户信息修改请求: %s", connectionID, msgID, string(messageData))

	var message MessageData
	err := json.Unmarshal(messageData, &message)
	if err != nil {
		log.Printf("[WS:%s:%s] 解析消息失败: %v", connectionID, msgID, err)
		return ResponseData{Type: "error", Message: "无效的请求格式"}, err
	}

	data := message.Data
	modificationType := data.Type
	action := data.Action
	if action == "" {
		action = "modify" // 操作类型默认值
	}
	authToken := data.AuthToken

	if authToken == "" {
		return ResponseData{Type: "error", Message: "authToken不能为空"}, errors.New("authToken不能为空")
	}

	// 验证authToken有效性
	valid, payload := s.authTokenService.VerifyAuthToken(authToken)
	if !valid {
		log.Printf("[WS:%s:%s] 修改账户信息请求使用了无效的authToken: %s", connectionID, msgID, payload)
		return ResponseData{Type: "error", Message: fmt.Sprintf("无效的authToken: %s", payload)}, nil
	}

	// 基于修改类型和操作选择处理方法
	if action == "request_code" {
		// 验证码请求处理
		newValue := data.NewValue
		if newValue == "" {
			return ResponseData{Type: "error", Message: "新的值不能为空"}, nil
		}

		return s.requestVerificationCode(clientID, modificationType, newValue, connectionID, msgID)
	} else if action == "modify" {
		// 执行修改操作
		newValue := data.NewValue
		verificationCode := data.VerificationCode

		// 需要验证码的修改操作
		if modificationType == "email" || modificationType == "mobile_number" {
			if verificationCode == "" {
				return ResponseData{Type: "error", Message: "验证码不能为空"}, nil
			}

			return s.modifyWithVerification(clientID, modificationType, newValue, verificationCode, connectionID, msgID)
		}

		// 不需要验证码的修改操作
		if modificationType == "username" || modificationType == "nickname" {
			if newValue == "" {
				return ResponseData{Type: "error", Message: "新的值不能为空"}, nil
			}

			return s.modifyWithoutVerification(clientID, modificationType, newValue, connectionID, msgID)
		}

		return ResponseData{Type: "error", Message: fmt.Sprintf("未知的修改类型: %s", modificationType)}, nil
	}

	return ResponseData{Type: "error", Message: fmt.Sprintf("未知的修改操作: %s", action)}, nil
}

// requestVerificationCode 请求验证码
func (s *ModificationService) requestVerificationCode(userID string, modificationType string, newValue string, connectionID string, msgID string) (ResponseData, error) {
	log.Printf("[WS:%s:%s] 用户 %s 申请 %s 修改验证码，新值: %s", connectionID, msgID, userID, modificationType, newValue)

	// 验证新值的格式
	if modificationType == "email" {
		// 邮箱格式验证
		matched, _ := regexp.MatchString(`^[a-zA-Z0-9_.+-]+@[a-zA-Z0-9-]+\.[a-zA-Z0-9-.]+$`, newValue)
		if !matched {
			log.Printf("[WS:%s:%s] 邮箱格式错误: %s", connectionID, msgID, newValue)
			return ResponseData{Type: "error", Message: "邮箱格式错误"}, nil
		}
	} else if modificationType == "mobile_number" {
		// 手机号格式验证(简化版)
		matched, _ := regexp.MatchString(`^\d{11}$`, newValue)
		if !matched {
			log.Printf("[WS:%s:%s] 手机号格式错误: %s", connectionID, msgID, newValue)
			return ResponseData{Type: "error", Message: "手机号格式错误"}, nil
		}
	}

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
		Type:      modificationType,
		Code:      codeStr,
		Value:     newValue,
		ExpiresAt: expiresAt,
	}

	log.Printf("[WS:%s:%s] 已生成用户 %s 的 %s 修改验证码: %s", connectionID, msgID, userID, modificationType, codeStr)

	// 在实际应用中，应该将验证码发送到用户的手机或邮箱
	// 但为了测试，直接返回验证码
	return ResponseData{
		Type: fmt.Sprintf("%s_verification_code_response", modificationType),
		Data: VerificationCodeResponse{
			Message:   fmt.Sprintf("%s修改验证码已发送", modificationType),
			Code:      codeStr, // 实际应用中不应该直接返回验证码
			ExpiresIn: 600,     // 过期时间(秒)
		},
	}, nil
}

// modifyWithVerification 执行需要验证码的修改操作
func (s *ModificationService) modifyWithVerification(userID string, modificationType string, newValue string, verificationCode string, connectionID string, msgID string) (ResponseData, error) {
	log.Printf("[WS:%s:%s] 用户 %s 请求修改 %s，新值: %s，验证码: %s", connectionID, msgID, userID, modificationType, newValue, verificationCode)

	// 验证验证码
	codeData, exists := s.verificationCodes[userID]
	if !exists {
		log.Printf("[WS:%s:%s] 用户 %s 未申请 %s 修改验证码", connectionID, msgID, userID, modificationType)
		return ResponseData{Type: "error", Message: "请先申请验证码"}, nil
	}

	// 验证修改类型
	if codeData.Type != modificationType {
		log.Printf("[WS:%s:%s] 验证码类型不匹配: 期望%s，实际%s", connectionID, msgID, modificationType, codeData.Type)
		return ResponseData{Type: "error", Message: "验证码类型错误"}, nil
	}

	// 验证新值匹配
	if newValue != "" && codeData.Value != newValue {
		log.Printf("[WS:%s:%s] 新值不匹配: 验证时%s，提交时%s", connectionID, msgID, codeData.Value, newValue)
		return ResponseData{Type: "error", Message: "新值与申请验证码时不一致"}, nil
	}

	// 验证码过期检查
	if time.Now().After(codeData.ExpiresAt) {
		log.Printf("[WS:%s:%s] 用户 %s 的验证码已过期", connectionID, msgID, userID)
		delete(s.verificationCodes, userID)
		return ResponseData{Type: "error", Message: "验证码已过期，请重新申请"}, nil
	}

	// 验证码匹配检查
	if verificationCode != codeData.Code {
		log.Printf("[WS:%s:%s] 用户 %s 提供的验证码不匹配", connectionID, msgID, userID)
		return ResponseData{Type: "error", Message: "验证码错误"}, nil
	}

	// 执行修改操作
	success, message := s.performModification(userID, modificationType, newValue, connectionID, msgID)

	// 修改成功后，清除验证码
	if success {
		delete(s.verificationCodes, userID)
	}

	// 构建响应
	return ResponseData{
		Type: fmt.Sprintf("%s_modification_response", modificationType),
		Data: ModificationResponse{
			Success: success,
			Message: message,
		},
	}, nil
}

// modifyWithoutVerification 执行不需要验证码的修改操作
func (s *ModificationService) modifyWithoutVerification(userID string, modificationType string, newValue string, connectionID string, msgID string) (ResponseData, error) {
	log.Printf("[WS:%s:%s] 用户 %s 请求修改 %s，新值: %s", connectionID, msgID, userID, modificationType, newValue)

	// 验证新值的格式和合法性
	if modificationType == "username" {
		// 用户名格式验证
		matched, _ := regexp.MatchString(`^[a-zA-Z0-9_]{3,20}$`, newValue)
		if !matched {
			log.Printf("[WS:%s:%s] 用户名格式错误: %s", connectionID, msgID, newValue)
			return ResponseData{Type: "error", Message: "用户名格式错误，只能包含字母、数字和下划线，长度3-20"}, nil
		}
	} else if modificationType == "nickname" {
		// 昵称长度验证
		if len(newValue) < 2 || len(newValue) > 30 {
			log.Printf("[WS:%s:%s] 昵称长度错误: %s", connectionID, msgID, newValue)
			return ResponseData{Type: "error", Message: "昵称长度必须在2-30个字符之间"}, nil
		}
	}

	// 执行修改操作
	success, message := s.performModification(userID, modificationType, newValue, connectionID, msgID)

	// 构建响应
	return ResponseData{
		Type: fmt.Sprintf("%s_modification_response", modificationType),
		Data: ModificationResponse{
			Success: success,
			Message: message,
		},
	}, nil
}

// performModification 执行实际的修改操作
func (s *ModificationService) performModification(userID string, modificationType string, newValue string, connectionID string, msgID string) (bool, string) {
	log.Printf("[WS:%s:%s] 执行用户 %s 的 %s 修改，新值: %s", connectionID, msgID, userID, modificationType, newValue)

	// 在实际应用中，这里需要连接数据库更新用户信息
	// 简化实现，仅模拟修改成功

	if modificationType == "username" {
		// 检查用户名是否已存在
		// 这里模拟检查，随机返回成功或失败
		if newValue == "admin" || newValue == "root" || newValue == "system" {
			log.Printf("[WS:%s:%s] 用户名 %s 已被占用", connectionID, msgID, newValue)
			return false, "用户名已被占用"
		}

		log.Printf("[WS:%s:%s] 修改用户名成功: %s", connectionID, msgID, newValue)
		return true, "用户名修改成功"
	} else if modificationType == "nickname" {
		log.Printf("[WS:%s:%s] 修改昵称成功: %s", connectionID, msgID, newValue)
		return true, "昵称修改成功"
	} else if modificationType == "email" {
		// 检查邮箱是否已存在
		// 这里模拟检查，随机返回成功或失败
		if newValue == "admin@example.com" || newValue == "root@example.com" {
			log.Printf("[WS:%s:%s] 邮箱 %s 已被占用", connectionID, msgID, newValue)
			return false, "邮箱已被占用"
		}

		log.Printf("[WS:%s:%s] 修改邮箱成功: %s", connectionID, msgID, newValue)
		return true, "邮箱修改成功"
	} else if modificationType == "mobile_number" {
		// 检查手机号是否已存在
		// 这里模拟检查，随机返回成功或失败
		if newValue == "13800000000" || newValue == "13900000000" {
			log.Printf("[WS:%s:%s] 手机号 %s 已被占用", connectionID, msgID, newValue)
			return false, "手机号已被占用"
		}

		log.Printf("[WS:%s:%s] 修改手机号成功: %s", connectionID, msgID, newValue)
		return true, "手机号修改成功"
	} else {
		log.Printf("[WS:%s:%s] 未知的修改类型: %s", connectionID, msgID, modificationType)
		return false, fmt.Sprintf("未知的修改类型: %s", modificationType)
	}
}

// 创建单例实例
var ModificationServiceInstance = NewModificationService()

func init() {
	// 初始化随机数生成器
	rand.Seed(time.Now().UnixNano())
} 