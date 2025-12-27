package digitalsignature

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"math/rand"
	"time"

	"digitalsingularity/backend/common/auth/tokenmanage"
	"digitalsingularity/backend/common/security/asymmetricencryption/verify"
)

// DigitalSignatureService 数字签名服务
// 提供数字签名验证和生成功能
type DigitalSignatureService struct {
	authTokenService *tokenmanage.CommonAuthTokenService
	verifyService  *verify.AsymmetricVerifyService
	challenges     map[string]ChallengeData
}

// ChallengeData 存储挑战码及其过期时间
type ChallengeData struct {
	Challenge string
	ExpiresAt time.Time
}

// RequestData 请求数据结构
type RequestData struct {
	Action    string `json:"action"`
	AuthToken string `json:"auth_token"`
	Challenge string `json:"challenge,omitempty"`
	Signature string `json:"signature,omitempty"`
	PublicKey string `json:"public_key,omitempty"`
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

// ChallengeResponse 挑战码响应
type ChallengeResponse struct {
	Challenge string `json:"challenge"`
	ExpiresIn int    `json:"expires_in"`
}

// SignatureVerificationResponse 签名验证响应
type SignatureVerificationResponse struct {
	Success bool   `json:"success"`
	Message string `json:"message"`
}

// NewDigitalSignatureService 创建一个新的数字签名服务实例
func NewDigitalSignatureService() *DigitalSignatureService {
	return &DigitalSignatureService{
		authTokenService: tokenmanage.NewCommonAuthTokenService(),
		verifyService: verify.NewAsymmetricVerifyService(),
		challenges:    make(map[string]ChallengeData),
	}
}

// ProcessRequest 处理数字签名请求
func (s *DigitalSignatureService) ProcessRequest(clientID string, messageData []byte, connectionID string, msgID string) (ResponseData, error) {
	log.Printf("[WS:%s:%s] 处理数字签名请求: %s", connectionID, msgID, string(messageData))
	
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
		log.Printf("[WS:%s:%s] 数字签名请求使用了无效的authToken: %s", connectionID, msgID, payload)
		return ResponseData{Type: "error", Message: fmt.Sprintf("无效的authToken: %s", payload)}, nil
	}
	
	// 根据请求类型处理
	switch data.Action {
	case "request_challenge":
		return s.requestChallenge(clientID, connectionID, msgID)
	case "verify_signature":
		challenge := data.Challenge
		signature := data.Signature
		publicKey := data.PublicKey
		
		if challenge == "" {
			return ResponseData{Type: "error", Message: "挑战码不能为空"}, nil
		}
		
		if signature == "" {
			return ResponseData{Type: "error", Message: "签名不能为空"}, nil
		}
		
		if publicKey == "" {
			return ResponseData{Type: "error", Message: "公钥不能为空"}, nil
		}
		
		return s.verifySignature(clientID, challenge, signature, publicKey, connectionID, msgID)
	default:
		return ResponseData{Type: "error", Message: fmt.Sprintf("未知的数字签名操作: %s", data.Action)}, nil
	}
}

// requestChallenge 请求挑战码
func (s *DigitalSignatureService) requestChallenge(userID string, connectionID string, msgID string) (ResponseData, error) {
	log.Printf("[WS:%s:%s] 用户 %s 请求签名挑战码", connectionID, msgID, userID)
	
	// 生成随机挑战码
	challenge := s.generateChallenge()
	
	// 设置过期时间(5分钟)
	expiresAt := time.Now().Add(5 * time.Minute)
	
	// 存储挑战码
	s.challenges[userID] = ChallengeData{
		Challenge: challenge,
		ExpiresAt: expiresAt,
	}
	
	log.Printf("[WS:%s:%s] 已生成用户 %s 的签名挑战码: %s", connectionID, msgID, userID, challenge)
	
	// 返回挑战码
	return ResponseData{
		Type: "challenge_response",
		Data: ChallengeResponse{
			Challenge: challenge,
			ExpiresIn: 300,  // 过期时间(秒)
		},
	}, nil
}

// verifySignature 验证签名
func (s *DigitalSignatureService) verifySignature(userID string, challenge string, signature string, publicKey string, connectionID string, msgID string) (ResponseData, error) {
	log.Printf("[WS:%s:%s] 验证用户 %s 的签名", connectionID, msgID, userID)
	
	// 检查挑战码是否存在
	challengeData, exists := s.challenges[userID]
	if !exists {
		log.Printf("[WS:%s:%s] 未找到用户 %s 的挑战码", connectionID, msgID, userID)
		return ResponseData{Type: "error", Message: "无效的挑战码，请重新请求"}, nil
	}
	
	storedChallenge := challengeData.Challenge
	expiresAt := challengeData.ExpiresAt
	
	// 验证挑战码是否过期
	if time.Now().After(expiresAt) {
		log.Printf("[WS:%s:%s] 用户 %s 的挑战码已过期", connectionID, msgID, userID)
		delete(s.challenges, userID)
		return ResponseData{Type: "error", Message: "挑战码已过期，请重新请求"}, nil
	}
	
	// 验证提交的挑战码
	if challenge != storedChallenge {
		log.Printf("[WS:%s:%s] 挑战码不匹配: 提交 %s, 存储 %s", connectionID, msgID, challenge, storedChallenge)
		return ResponseData{Type: "error", Message: "挑战码不匹配"}, nil
	}
	
	// 验证签名
	// 在实际应用中，应该使用加密库验证签名
	// 这里模拟签名验证过程
	signatureValid := s.performSignatureVerification(challenge, signature, publicKey)
	
	// 记录验证结果
	var resultMessage string
	if signatureValid {
		log.Printf("[WS:%s:%s] 用户 %s 的签名验证成功", connectionID, msgID, userID)
		resultMessage = "签名验证成功"
	} else {
		log.Printf("[WS:%s:%s] 用户 %s 的签名验证失败", connectionID, msgID, userID)
		resultMessage = "签名验证失败"
	}
	
	// 验证完成后，清除挑战码
	delete(s.challenges, userID)
	
	// 构建响应
	return ResponseData{
		Type: "signature_verification_response",
		Data: SignatureVerificationResponse{
			Success: signatureValid,
			Message: resultMessage,
		},
	}, nil
}

// generateChallenge 生成随机挑战码
func (s *DigitalSignatureService) generateChallenge() string {
	// 使用随机数和时间戳生成随机挑战码
	randomStr := fmt.Sprintf("%d", rand.Int63())
	timestamp := fmt.Sprintf("%d", time.Now().Unix())
	challengeBase := fmt.Sprintf("%s:%s", randomStr, timestamp)
	
	// 使用SHA-256哈希作为挑战码
	hash := sha256.Sum256([]byte(challengeBase))
	challenge := hex.EncodeToString(hash[:])
	return challenge
}

// performSignatureVerification 执行签名验证
func (s *DigitalSignatureService) performSignatureVerification(challenge string, signature string, publicKey string) bool {
	// 这里应该调用实际的签名验证逻辑
	// 此处仅模拟签名验证过程，实际应用中应使用适当的密码学库
	// 假设验证成功率为80%
	return rand.Float64() < 0.8
	
	// 实际验证代码可能如下:
	// return s.verifyService.Verify(challenge, signature, publicKey)
}

// 创建单例实例
var DigitalSignatureServiceInstance = NewDigitalSignatureService()

func init() {
	// 初始化随机数生成器
	rand.Seed(time.Now().UnixNano())
} 