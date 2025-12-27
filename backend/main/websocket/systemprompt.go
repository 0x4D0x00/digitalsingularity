package websocket

import (
	"fmt"
	"log"
)

// RoleInfo 角色信息（用于系统提示词更新）
type SystemPromptRoleInfo struct {
	RoleType string `json:"role_type"` // 角色类型（如：comprehensive_strike）
	RoleName string `json:"role_name"` // 角色名称（如：全面打击）
}

// BroadcastSystemPromptUpdate 广播系统提示词更新消息给所有WebSocket客户端
func BroadcastSystemPromptUpdate(roles []SystemPromptRoleInfo) {
	if connectionManager != nil {
		connectionManager.BroadcastMessage("system_prompt_updated", map[string]interface{}{
			"roles": roles,
		})
		logger.Printf("已广播系统提示词更新，角色数量: %d", len(roles))
	} else {
		log.Printf("[WebSocket] 连接管理器未初始化，无法广播系统提示词更新")
	}
}

// GetAllSystemPromptRolesFromRedis 从Redis获取所有对外可用角色（供WebSocket使用）
// 通过调用service.go中的方法从Redis读取
func GetAllSystemPromptRolesFromRedis() ([]SystemPromptRoleInfo, error) {
	if readWrite == nil {
		return nil, fmt.Errorf("readWrite服务未初始化")
	}
	
	// 调用service.go中的方法从Redis读取
	roles, err := readWrite.GetAllRolesFromRedis()
	if err != nil {
		return nil, err
	}
	
	// 转换为当前包的SystemPromptRoleInfo类型
	result := make([]SystemPromptRoleInfo, len(roles))
	for i, role := range roles {
		result[i] = SystemPromptRoleInfo{
			RoleType: role.RoleType,
			RoleName: role.RoleName,
		}
	}
	
	return result, nil
}

