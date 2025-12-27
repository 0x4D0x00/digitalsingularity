package http

import (
	"fmt"
	"strings"

	aibasicplatformdatabase "digitalsingularity/backend/aibasicplatform/database"
	websocketsystemprompt "digitalsingularity/backend/main/websocket"
)

// 系统提示词管理接口处理函数

// handleSystemPromptListRequest 获取所有系统提示词（业务逻辑函数）
func handleSystemPromptListRequest(requestID string, data map[string]interface{}) map[string]interface{} {
	logger.Printf("[%s] 收到获取系统提示词列表请求", requestID)

	// 从Redis获取所有系统提示词（使用datahandle中的方法）
	if readWrite == nil {
		logger.Printf("[%s] readWrite服务未初始化", requestID)
		return map[string]interface{}{
			"status":  "fail",
			"message": "服务未初始化",
		}
	}

	prompts, err := readWrite.GetAllSystemPromptsFromRedis()
	if err != nil {
		logger.Printf("[%s] 获取系统提示词失败: %v", requestID, err)
		return map[string]interface{}{
			"status":  "fail",
			"message": fmt.Sprintf("获取系统提示词失败: %v", err),
		}
	}

	logger.Printf("[%s] 成功获取 %d 个系统提示词", requestID, len(prompts))

	return map[string]interface{}{
		"status": "success",
		"data":   prompts,
	}
}

// handleSystemPromptRefreshRequest 刷新单个系统提示词（业务逻辑函数）
func handleSystemPromptRefreshRequest(requestID string, data map[string]interface{}) map[string]interface{} {
	logger.Printf("[%s] 收到刷新系统提示词请求", requestID)

	// 刷新系统提示词缓存（从Redis重新加载）
	err := RefreshAllSystemPromptCache()
	if err != nil {
		logger.Printf("[%s] 刷新系统提示词缓存失败: %v", requestID, err)
		return map[string]interface{}{
			"status":  "fail",
			"message": fmt.Sprintf("刷新系统提示词缓存失败: %v", err),
		}
	}

	logger.Printf("[%s] 系统提示词缓存刷新成功", requestID)

	// 触发WebSocket推送系统提示词更新
	BroadcastSystemPromptUpdate()

	return map[string]interface{}{
		"status":  "success",
		"message": "系统提示词缓存刷新成功",
	}
}

// handleSystemPromptRefreshAllRequest 刷新所有系统提示词（业务逻辑函数）
func handleSystemPromptRefreshAllRequest(requestID string, data map[string]interface{}) map[string]interface{} {
	logger.Printf("[%s] 收到刷新所有系统提示词请求", requestID)

	// 刷新所有提示词（从Redis重新加载）
	if err := RefreshAllSystemPromptCache(); err != nil {
		logger.Printf("[%s] 刷新系统提示词缓存失败: %v", requestID, err)
		return map[string]interface{}{
			"status":  "fail",
			"message": fmt.Sprintf("刷新系统提示词缓存失败: %v", err),
		}
	}

	logger.Printf("[%s] 系统提示词缓存刷新成功", requestID)

	// 触发WebSocket推送系统提示词更新
	BroadcastSystemPromptUpdate()

	return map[string]interface{}{
		"status":  "success",
		"message": "系统提示词缓存刷新成功",
	}
}

// RoleInfo 角色信息
type RoleInfo struct {
	RoleType string `json:"role_type"` // 角色类型（如：comprehensive_strike）
	RoleName string `json:"role_name"` // 角色名称（如：全面打击）
}

// GetAllRolesFromRedis 从Redis获取所有对外可用角色（供前端使用）
// 通过调用service.go中的方法从Redis读取
func GetAllRolesFromRedis() ([]RoleInfo, error) {
	if readWrite == nil {
		return nil, fmt.Errorf("readWrite服务未初始化")
	}

	// 调用service.go中的方法从Redis读取
	roles, err := readWrite.GetAllRolesFromRedis()
	if err != nil {
		return nil, err
	}

	// 转换为当前包的RoleInfo类型
	result := make([]RoleInfo, len(roles))
	for i, role := range roles {
		result[i] = RoleInfo{
			RoleType: role.RoleType,
			RoleName: role.RoleName,
		}
	}

	return result, nil
}

// handleGetAllRolesRequest 获取所有角色（业务逻辑函数）
func handleGetAllRolesRequest(requestID string, data map[string]interface{}) map[string]interface{} {
	logger.Printf("[%s] 收到获取所有角色请求", requestID)

	// 从Redis获取所有角色
	roles, err := GetAllRolesFromRedis()
	if err != nil {
		logger.Printf("[%s] 获取角色失败: %v", requestID, err)
		return map[string]interface{}{
			"status":  "fail",
			"message": fmt.Sprintf("获取角色失败: %v", err),
			"data":    []RoleInfo{},
		}
	}

	logger.Printf("[%s] 成功获取 %d 个角色", requestID, len(roles))

	return map[string]interface{}{
		"status": "success",
		"data":   roles,
	}
}

// RefreshAllSystemPromptCache 刷新所有系统提示词缓存（重新从数据库加载到Redis）
func RefreshAllSystemPromptCache() error {
	if readWrite == nil {
		return fmt.Errorf("readWrite服务未初始化")
	}

	// 先清空Redis中所有system_prompt相关键
	if err := readWrite.ClearSystemPromptKeys(); err != nil {
		logger.Printf("刷新前清空system_prompt相关键失败: %v", err)
		// 继续执行，不清空可能只是警告
	} else {
		logger.Printf("刷新前已清空Redis中所有system_prompt相关键")
	}

	var errors []string

	// 刷新AiBasicPlatform系统提示词
	aiBasicPlatformService := aibasicplatformdatabase.NewAIBasicPlatformDataService(readWrite)
	if aiBasicPlatformService == nil {
		errMsg := "AiBasicPlatform数据服务未初始化"
		errors = append(errors, fmt.Sprintf("AiBasicPlatform: %s", errMsg))
		logger.Printf("刷新AiBasicPlatform系统提示词失败: %s", errMsg)
	} else if err := aiBasicPlatformService.LoadPromptsToRedis(); err != nil {
		errors = append(errors, fmt.Sprintf("AiBasicPlatform: %v", err))
		logger.Printf("刷新AiBasicPlatform系统提示词失败: %v", err)
	} else {
		logger.Printf("AiBasicPlatform系统提示词刷新成功")
	}

	// comprehensive_strike 模块已移除，跳过该项刷新

	if len(errors) > 0 {
		return fmt.Errorf("部分服务刷新失败: %s", strings.Join(errors, "; "))
	}

	return nil
}

// BroadcastSystemPromptUpdate 广播系统提示词更新消息给所有WebSocket客户端
func BroadcastSystemPromptUpdate() {
	// 获取更新后的角色列表
	roles, err := GetAllRolesFromRedis()
	if err != nil {
		logger.Printf("获取角色列表失败，无法广播: %v", err)
		return
	}

	// 转换为websocket包的类型
	wsRoles := make([]websocketsystemprompt.SystemPromptRoleInfo, len(roles))
	for i, role := range roles {
		wsRoles[i] = websocketsystemprompt.SystemPromptRoleInfo{
			RoleType: role.RoleType,
			RoleName: role.RoleName,
		}
	}

	// 调用websocket包的广播函数
	websocketsystemprompt.BroadcastSystemPromptUpdate(wsRoles)
}
