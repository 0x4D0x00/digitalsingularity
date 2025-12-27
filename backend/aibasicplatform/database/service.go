package database

import (
	"fmt"
	"log"
	"time"

	"digitalsingularity/backend/common/utils/datahandle"
	"digitalsingularity/backend/common/security/symmetricencryption/decrypt"
)

// AIBasicPlatformDataService 处理AI平台用户数据
type AIBasicPlatformDataService struct {
	readWrite *datahandle.CommonReadWriteService
	dbName    string
}

// NewAIBasicPlatformDataService 创建新的数据服务实例
// readWrite 参数可选，未提供时自动创建默认实例
func NewAIBasicPlatformDataService(readWrite ...*datahandle.CommonReadWriteService) *AIBasicPlatformDataService {
	var rw *datahandle.CommonReadWriteService
	if len(readWrite) > 0 {
		rw = readWrite[0]
	}

	service := &AIBasicPlatformDataService{
		readWrite: rw,
		dbName:    "aibasicplatform", // 默认值
	}

	// 如果 readWrite 为 nil，创建一个新的实例
	if service.readWrite == nil {
		rwService, err := datahandle.NewCommonReadWriteService("database")
		if err != nil {
			log.Printf("创建数据服务实例错误: %v", err)
			return service
		}
		service.readWrite = rwService
	}

	log.Printf("[AiBasicPlatform] 使用默认数据库名: %s", service.dbName)
	return service
}

// GetDatabaseName 返回当前使用的数据库名
func (s *AIBasicPlatformDataService) GetDatabaseName() string {
	return s.dbName
}
 

// GetUserAiPlatformData 获取AI平台用户数据，根据前端需求返回必要数据
func (s *AIBasicPlatformDataService) GetUserAiPlatformData(userID string) map[string]interface{} {
	defer func() {
		if r := recover(); r != nil {
			fmt.Printf("获取AI平台用户数据错误: %v\n", r)
		}
	}()
	
	// 仅获取用户基本信息，不获取token信息
	userInfo := s.getUserBasicInfo(userID)
	
	// 检查是否获取到有效的用户信息
	if len(userInfo) == 0 {
		return map[string]interface{}{"error": "未找到用户信息"}
	}
	
	// 确保所有时间对象都被转换为字符串
	s.convertDateTimeToStr(userInfo)
	
	// 返回完整数据，不进行字段过滤
	// 注释掉过滤代码，让上层调用方决定哪些返回给前端
	// allowedFields := []string{
	//    "user_name", "nick_name", "email", "mobile_number", "user_account",
	//    "created_at", "last_login", "user_level", "profile_picture_url", 
	//    "location_region", "user_signature",
	// }
	// filteredUserInfo := make(map[string]interface{})
	// for k, v := range userInfo {
	//     if contains(allowedFields, k) {
	//         filteredUserInfo[k] = v
	//     }
	// }
	
	return userInfo
}

// convertDateTimeToStr 将映射中的所有时间对象转换为字符串
func (s *AIBasicPlatformDataService) convertDateTimeToStr(data map[string]interface{}) {
	if data == nil {
		return
	}
	
	for key, value := range data {
		switch v := value.(type) {
		case time.Time:
			data[key] = v.Format("2006-01-02 15:04:05")
		case map[string]interface{}:
			s.convertDateTimeToStr(v)
		case []interface{}:
			for _, item := range v {
				if itemMap, ok := item.(map[string]interface{}); ok {
					s.convertDateTimeToStr(itemMap)
				}
			}
		}
	}
}

// getUserBasicInfo 获取用户基本信息
func (s *AIBasicPlatformDataService) getUserBasicInfo(userID string) map[string]interface{} {
	var userInfo map[string]interface{}
	
	defer func() {
		if r := recover(); r != nil {
			fmt.Printf("获取用户基本信息错误: %v\n", r)
			userInfo = map[string]interface{}{}
		}
	}()

	// 使用配置文件中定义的数据库名称: data_common_database = common
	query := `
		SELECT 
			user_id, user_name, nickname, avatar_url, email, phone, 
			created_at, last_login_at as last_login, status
		FROM common.users
		WHERE user_id = ?
	`
	opResult := s.readWrite.QueryDb(query, userID)
	if !opResult.IsSuccess() {
		fmt.Printf("数据库查询错误: %v\n", opResult.Error)
		return map[string]interface{}{}
	}
	
	queryResult, ok := opResult.Data.([]map[string]interface{})
	if !ok || len(queryResult) == 0 {
		return map[string]interface{}{}
	}
	
	userInfo = queryResult[0]
	
	// 解密电话号码
	if phone, ok := userInfo["phone"]; ok && phone != nil {
		func() {
			defer func() {
				if r := recover(); r != nil {
					fmt.Printf("电话号码解密错误: %v\n", r)
					userInfo["phone"] = "******"
					userInfo["mobile_number"] = "******"
					userInfo["user_account"] = "******"
				}
			}()
			
			// 修改为正确的调用方式，如果decrypt.SymmetricDecryptService需要特定参数，请适当调整
			decryptedPhone, err := decrypt.SymmetricDecryptService(phone.(string), "", "")
			if err != nil {
				fmt.Printf("电话号码解密错误: %v\n", err)
				userInfo["phone"] = "******"
			} else {
				userInfo["phone"] = decryptedPhone
				// 增加前端需要的mobile_number字段（与phone相同）
				userInfo["mobile_number"] = decryptedPhone
				// 增加user_account字段
				userInfo["user_account"] = decryptedPhone
			}
		}()
	}
	
	// 添加前端需要的字段，使用默认值
	// user_name 已经从数据库查询中获取，不需要再赋值
	if nickname, ok := userInfo["nickname"]; ok {
		userInfo["nick_name"] = nickname
	} else {
		userInfo["nick_name"] = ""
	}
	// 确保 user_name 字段存在
	if _, ok := userInfo["user_name"]; !ok {
		userInfo["user_name"] = ""
	}
	userInfo["user_level"] = ""
	
	if avatarURL, ok := userInfo["avatar_url"]; ok {
		userInfo["profile_picture_url"] = avatarURL
	} else {
		userInfo["profile_picture_url"] = ""
	}
	userInfo["location_region"] = ""
	userInfo["user_signature"] = ""
	
	// 确保时间字段被转换为字符串
	if createdAt, ok := userInfo["created_at"]; ok {
		if t, ok := createdAt.(time.Time); ok {
			userInfo["created_at"] = t.Format("2006-01-02 15:04:05")
		}
	}
	
	if lastLogin, ok := userInfo["last_login"]; ok {
		if t, ok := lastLogin.(time.Time); ok {
			userInfo["last_login"] = t.Format("2006-01-02 15:04:05")
		}
	}
	
	return userInfo
}

// AiBasicPlatformLoginService 处理AI平台登录服务
type AiBasicPlatformLoginService struct {
	readWrite   *datahandle.CommonReadWriteService
	dataService *AIBasicPlatformDataService
}

// NewAiBasicPlatformLoginService 创建新的登录服务实例
func NewAiBasicPlatformLoginService() *AiBasicPlatformLoginService {
	rwService, err := datahandle.NewCommonReadWriteService("database")
	if err != nil {
		log.Printf("创建数据服务实例错误: %v", err)
		return &AiBasicPlatformLoginService{}
	}
	
	return &AiBasicPlatformLoginService{
		readWrite:   rwService,
		dataService: NewAIBasicPlatformDataService(),
	}
}

// GetUserAiBasicPlatformData 获取AI基础平台用户数据，符合前端需求
func (s *AiBasicPlatformLoginService) GetUserAiBasicPlatformData(user interface{}) map[string]interface{} {
	defer func() {
		if r := recover(); r != nil {
			fmt.Printf("获取AI基础平台数据错误: %v\n", r)
		}
	}()

	// 确保能够安全获取userId，无论user是什么格式
	var userID string
	
	switch u := user.(type) {
	case map[string]interface{}:
		if id, ok := u["user_id"]; ok {
			userID = fmt.Sprintf("%v", id)
		}
	case string:
		userID = u
	default:
		userID = ""
	}
	
	if userID == "" {
		fmt.Println("获取AI基础平台数据错误: 无效的用户ID")
		return map[string]interface{}{"error": "无效的用户ID"}
	}
	
	// 获取用户数据
	userData := s.dataService.GetUserAiPlatformData(userID)
	
	// 检查是否有错误
	if errMsg, ok := userData["error"]; ok {
		return map[string]interface{}{"error": errMsg}
	}
	
	// 返回结果
	return userData
} 