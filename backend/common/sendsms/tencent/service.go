package tencent

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"net/url"
	"strings"
	"time"

	pathconfig "digitalsingularity/backend/common/configs"

	"github.com/spf13/viper"
	"github.com/tencentcloud/tencentcloud-sdk-go/tencentcloud/common"
	"github.com/tencentcloud/tencentcloud-sdk-go/tencentcloud/common/errors"
	"github.com/tencentcloud/tencentcloud-sdk-go/tencentcloud/common/profile"
	sms "github.com/tencentcloud/tencentcloud-sdk-go/tencentcloud/sms/v20210111"
)

// 创建logger
var logger = log.New(log.Writer(), "[TencentSMS] ", log.LstdFlags)

// SmsService 腾讯云短信服务
type SmsService struct {
	secretId  string
	secretKey string
	region    string
	appId     string
}

// NewSmsService 创建新的短信服务实例
func NewSmsService() *SmsService {
	// 使用统一的路径配置
	pathCfg := pathconfig.GetInstance()
	
	// 配置Viper
	viper.SetConfigName("backendserviceconfig")
	viper.SetConfigType("ini")
	
	// 添加配置路径
	viper.AddConfigPath(pathCfg.ConfigPath)
	viper.AddConfigPath(pathCfg.ConfigPathLegacy)
	viper.AddConfigPath("backend/common/config")
	viper.AddConfigPath("config")
	
	// 读取配置文件
	if err := viper.ReadInConfig(); err != nil {
		logger.Printf("无法读取配置文件: %v，使用默认配置", err)
	}
	
	// 从配置文件读取短信服务配置
	secretId := viper.GetString("ShortMessage.SecretId")
	if secretId == "" {
		secretId = "XXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXX"
		logger.Printf("配置文件中未找到ShortMessage.SecretId，使用默认值")
	}
	
	secretKey := viper.GetString("ShortMessage.SecretKey")
	if secretKey == "" {
		secretKey = "XXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXX"
		logger.Printf("配置文件中未找到ShortMessage.SecretKey，使用默认值")
	}
	
	// 其他配置
	region := viper.GetString("ShortMessage.Region")
	if region == "" {
		region = "ap-guangzhou"  // 腾讯云短信默认地区
	}
	
	appId := viper.GetString("ShortMessage.AppId")
	if appId == "" {
		appId = "XXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXX"  // 默认应用ID
	}
	
	logger.Printf("短信服务初始化完成，使用SecretId: %s", secretId[:5]+"*****")
	
	return &SmsService{
		secretId:  secretId,
		secretKey: secretKey,
		region:    region,
		appId:     appId,
	}
}

// SendSmsCode 发送短信验证码
// phoneNumber: 手机号，格式为+86XXXXXXXXXX
// code: 验证码
// templateId: 模板ID
// signName: 短信签名
// sdkAppId: 应用ID
// 返回: 是否成功，错误消息
func (s *SmsService) SendSmsCode(phoneNumber, code, templateId, signName, sdkAppId string) (bool, string) {
	// 参数检查
	if phoneNumber == "" || code == "" || templateId == "" || signName == "" {
		return false, "参数不完整"
	}

	// 处理电话号码格式
	// 如果是 +86 开头，移除国家代码，因为腾讯云SDK要求分开填写
	var nationCode string
	var mobile string

	if strings.HasPrefix(phoneNumber, "+") {
		parts := strings.SplitN(phoneNumber[1:], "", 2)
		if len(parts) == 2 {
			nationCode = parts[0]
			mobile = parts[1]
		} else {
			nationCode = "86" // 默认中国大陆
			mobile = phoneNumber[1:]
		}
	} else {
		nationCode = "86" // 默认中国大陆
		mobile = phoneNumber
	}

	// 使用SDK还是直接HTTP请求
	if s.secretId != "" && s.secretKey != "" {
		return s.sendSmsWithSdk(nationCode, mobile, code, templateId, signName, sdkAppId)
	} else {
		return s.sendSmsWithHttp(nationCode, mobile, code, templateId, signName, sdkAppId)
	}
}

// 使用腾讯云SDK发送短信
func (s *SmsService) sendSmsWithSdk(nationCode, mobile, code, templateId, signName, sdkAppId string) (bool, string) {
	// 初始化认证信息
	credential := common.NewCredential(s.secretId, s.secretKey)
	
	// 配置客户端
	cpf := profile.NewClientProfile()
	cpf.HttpProfile.Endpoint = "sms.tencentcloudapi.com"
	
	// 创建客户端
	client, err := sms.NewClient(credential, s.region, cpf)
	if err != nil {
		logger.Printf("创建短信客户端失败: %v", err)
		return false, fmt.Sprintf("创建短信客户端失败: %v", err)
	}
	
	// 构造请求
	request := sms.NewSendSmsRequest()
	request.PhoneNumberSet = common.StringPtrs([]string{fmt.Sprintf("+%s%s", nationCode, mobile)})
	request.TemplateId = common.StringPtr(templateId)
	request.SignName = common.StringPtr(signName)
	request.TemplateParamSet = common.StringPtrs([]string{code})
	request.SmsSdkAppId = common.StringPtr(sdkAppId)
	
	// 发送请求
	response, err := client.SendSms(request)
	if err != nil {
		if _, ok := err.(*errors.TencentCloudSDKError); ok {
			logger.Printf("腾讯云API错误: %v", err)
			return false, fmt.Sprintf("腾讯云API错误: %v", err)
		}
		logger.Printf("发送短信失败: %v", err)
		return false, fmt.Sprintf("发送短信失败: %v", err)
	}
	
	// 处理响应
	for _, statusInfo := range response.Response.SendStatusSet {
		if *statusInfo.Code != "Ok" {
			logger.Printf("短信发送失败: %s", *statusInfo.Message)
			return false, fmt.Sprintf("短信发送失败: %s", *statusInfo.Message)
		}
	}
	
	return true, "短信发送成功"
}

// 使用HTTP请求直接发送短信
func (s *SmsService) sendSmsWithHttp(nationCode, mobile, code, templateId, signName, sdkAppId string) (bool, string) {
	// 构建请求参数
	params := url.Values{}
	params.Add("sdkappid", sdkAppId)
	params.Add("random", fmt.Sprintf("%d", time.Now().UnixNano()))
	
	// 构建请求体
	type SmsRequest struct {
		Tel struct {
			Nationcode string `json:"nationcode"`
			Mobile     string `json:"mobile"`
		} `json:"tel"`
		Type       int      `json:"type"`
		Msg        string   `json:"msg"`
		Sig        string   `json:"sig"`
		Time       int64    `json:"time"`
		Extend     string   `json:"extend"`
		Ext        string   `json:"ext"`
		TemplateID string   `json:"tpl_id"`
		Params     []string `json:"params"`
		Sign       string   `json:"sign"`
	}
	
	// 当前时间戳
	timestamp := time.Now().Unix()
	
	// 构建请求对象
	request := SmsRequest{
		Type:       0,
		Msg:        "",
		Sig:        "",
		Time:       timestamp,
		Extend:     "",
		Ext:        "",
		TemplateID: templateId,
		Params:     []string{code},
		Sign:       signName,
	}
	request.Tel.Nationcode = nationCode
	request.Tel.Mobile = mobile
	
	// 序列化为JSON
	jsonData, err := json.Marshal(request)
	if err != nil {
		logger.Printf("JSON序列化失败: %v", err)
		return false, fmt.Sprintf("准备请求数据失败: %v", err)
	}
	
	// 发送请求
	apiUrl := fmt.Sprintf("https://yun.tim.qq.com/v5/tlssmssvr/sendsms?%s", params.Encode())
	req, err := http.NewRequest("POST", apiUrl, strings.NewReader(string(jsonData)))
	if err != nil {
		logger.Printf("创建HTTP请求失败: %v", err)
		return false, fmt.Sprintf("创建HTTP请求失败: %v", err)
	}
	
	// 设置请求头
	req.Header.Set("Content-Type", "application/json")
	
	// 发送请求
	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		logger.Printf("发送HTTP请求失败: %v", err)
		return false, fmt.Sprintf("发送HTTP请求失败: %v", err)
	}
	defer resp.Body.Close()
	
	// 读取响应
	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		logger.Printf("读取响应失败: %v", err)
		return false, fmt.Sprintf("读取响应失败: %v", err)
	}
	
	// 解析响应
	type SmsResponse struct {
		Result int    `json:"result"`
		Errmsg string `json:"errmsg"`
	}
	
	var response SmsResponse
	err = json.Unmarshal(body, &response)
	if err != nil {
		logger.Printf("解析响应失败: %v", err)
		return false, fmt.Sprintf("解析响应失败: %v", err)
	}
	
	// 处理结果
	if response.Result != 0 {
		logger.Printf("短信发送失败: %s", response.Errmsg)
		return false, fmt.Sprintf("短信发送失败: %s", response.Errmsg)
	}
	
	return true, "短信发送成功"
} 