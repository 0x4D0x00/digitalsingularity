package smsverify

import (
	"crypto/rand"
	"fmt"
	"log"
	"strconv"
	"time"

	"digitalsingularity/backend/common/sendsms/tencent"
	"digitalsingularity/backend/common/utils/datahandle"
)

// 创建logger
var logger = log.New(log.Writer(), "[SmsVerify] ", log.LstdFlags)

// SmsVerifyService 短信验证码服务，负责验证码的生成、存储和验证
type SmsVerifyService struct {
	readWrite        *datahandle.CommonReadWriteService
	smsService       *tencent.SmsService
	codeExpireSeconds int
	codeLength        int
	dailyLimit        int
	lockoutSeconds    int
}

// NewSmsVerifyService 创建新的SmsVerifyService实例
func NewSmsVerifyService() (*SmsVerifyService, error) {
	// 创建读写服务
	readWrite, err := datahandle.NewCommonReadWriteService("database")
	if err != nil {
		logger.Printf("创建读写服务失败: %v", err)
		return nil, err
	}

	// 创建短信服务
	smsService := tencent.NewSmsService()

	// 创建服务实例
	service := &SmsVerifyService{
		readWrite:        readWrite,
		smsService:       smsService,
		codeExpireSeconds: 600, // 验证码10分钟有效期
		codeLength:        6,   // 验证码长度
		dailyLimit:        10,  // 每天最多发送10条短信
		lockoutSeconds:    60,  // 发送间隔限制，60秒
	}

	return service, nil
}

// GenerateVerifyCode 生成并发送验证码
func (s *SmsVerifyService) GenerateVerifyCode(phone string) (bool, string) {
	// 检查发送次数限制
	sendCount := s.getSendLimit(phone)
	if sendCount >= s.dailyLimit {
		return false, fmt.Sprintf("今日发送验证码次数已达上限(%d次)", s.dailyLimit)
	}

	// 检查发送间隔限制
	lockoutKey := fmt.Sprintf("sms_lockout:%s", phone)
	lockoutResult := s.readWrite.GetRedis(lockoutKey)
	
	if lockoutResult.IsSuccess() {
		lockoutTimeStr, ok := lockoutResult.Data.(string)
		if ok {
			lockoutTime, err := strconv.ParseInt(lockoutTimeStr, 10, 64)
			if err == nil {
				now := time.Now().Unix()
				remainingTime := lockoutTime - now
				if remainingTime > 0 {
					return false, fmt.Sprintf("请在%d秒后重试", remainingTime)
				}
			}
		}
	}

	// 生成随机验证码
	verifyCode := s.generateRandomDigits(s.codeLength)

	// 存储验证码到Redis
	redisKey := fmt.Sprintf("sms_code:%s", phone)
	result := s.readWrite.SetRedis(redisKey, verifyCode, time.Duration(s.codeExpireSeconds)*time.Second)
	if !result.IsSuccess() {
		logger.Printf("存储验证码到Redis失败: %v", result.Error)
		return false, "系统错误，请稍后重试"
	}

	// 调用短信发送服务发送验证码
	phoneNumber := fmt.Sprintf("+86%s", phone) // 添加中国区号
	
	// 使用与16664captcha.py一致的参数
	success, message := s.smsService.SendSmsCode(
		phoneNumber, 
		verifyCode,
		"2294011",  // 使用实际业务的模板ID
		"数界奇点",  // 使用实际业务的签名
		"1400938156", // 使用实际业务的应用ID
	)
	
	if success {
		// 增加发送计数
		s.incrementSendCount(phone)
		// 设置发送间隔锁定
		now := time.Now().Unix()
		lockoutTime := now + int64(s.lockoutSeconds)
		s.readWrite.SetRedis(lockoutKey, strconv.FormatInt(lockoutTime, 10), time.Duration(s.lockoutSeconds)*time.Second)
		return true, "验证码发送成功"
	} else {
		return false, fmt.Sprintf("短信发送失败: %s", message)
	}
}

// VerifyCode 验证短信验证码
func (s *SmsVerifyService) VerifyCode(phone string, verifyCode string) (bool, string) {
	// 从Redis获取存储的验证码
	redisKey := fmt.Sprintf("sms_code:%s", phone)
	result := s.readWrite.GetRedis(redisKey)
	
	if !result.IsSuccess() {
		return false, "验证码错误或已过期"
	}
	
	// 获取存储的验证码
	storedCode, ok := result.Data.(string)
	if !ok || storedCode != verifyCode {
		return false, "验证码错误或已过期"
	}
	
	// 验证成功后删除Redis中的验证码，防止重复使用
	s.readWrite.DeleteRedis(redisKey)
	
	return true, "验证码验证成功"
}

// GetSendLimit 获取手机号当日发送次数限制
func (s *SmsVerifyService) getSendLimit(phone string) int {
	// 获取当前日期
	today := time.Now().Format("2006-01-02")
	redisKey := fmt.Sprintf("sms_limit:%s:%s", phone, today)
	
	// 获取当日已发送次数
	result := s.readWrite.GetRedis(redisKey)
	
	if !result.IsSuccess() {
		return 0
	}
	
	sentCountStr, ok := result.Data.(string)
	if !ok {
		return 0
	}
	
	sentCount, err := strconv.Atoi(sentCountStr)
	if err != nil {
		logger.Printf("解析发送次数失败: %v", err)
		return 0
	}
	
	return sentCount
}

// IncrementSendCount 增加手机号当日发送次数计数
func (s *SmsVerifyService) incrementSendCount(phone string) int {
	// 获取当前日期
	today := time.Now().Format("2006-01-02")
	redisKey := fmt.Sprintf("sms_limit:%s:%s", phone, today)
	
	// 获取当日已发送次数
	result := s.readWrite.GetRedis(redisKey)
	
	if !result.IsSuccess() {
		// 首次发送，设置为1并设置过期时间（当天结束）
		secondsUntilTomorrow := s.getSecondsUntilTomorrow()
		s.readWrite.SetRedis(redisKey, "1", time.Duration(secondsUntilTomorrow)*time.Second)
		return 1
	}
	
	sentCountStr, ok := result.Data.(string)
	if !ok {
		return 0
	}
	
	sentCount, err := strconv.Atoi(sentCountStr)
	if err != nil {
		logger.Printf("解析发送次数失败: %v", err)
		return 0
	}
	
	// 递增计数
	newCount := sentCount + 1
	s.readWrite.SetRedis(redisKey, strconv.Itoa(newCount), 0)
	return newCount
}

// 生成指定长度的随机数字
func (s *SmsVerifyService) generateRandomDigits(length int) string {
	const digits = "0123456789"
	bytes := make([]byte, length)
	
	// 读取随机字节
	_, err := rand.Read(bytes)
	if err != nil {
		logger.Printf("生成随机数字失败: %v", err)
		// 出错时使用时间戳
		timestamp := time.Now().UnixNano()
		ts := strconv.FormatInt(timestamp, 10)
		return ts[len(ts)-length:]
	}
	
	// 映射到数字
	for i, b := range bytes {
		bytes[i] = digits[int(b)%len(digits)]
	}
	
	return string(bytes)
}

// 计算到第二天0点的秒数
func (s *SmsVerifyService) getSecondsUntilTomorrow() int {
	now := time.Now()
	tomorrow := time.Date(now.Year(), now.Month(), now.Day()+1, 0, 0, 0, 0, now.Location())
	return int(tomorrow.Sub(now).Seconds())
} 