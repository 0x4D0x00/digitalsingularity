package formatconverter

import (
	"fmt"
	"strings"
	"time"
)

// processSystemPrompt 处理 system_prompt 的获取和注入
// 使用 database.AIBasicPlatformDataService 的 GetSystemPrompt 方法获取系统提示词
func (s *SilicoidFormatConverterService) processSystemPrompt(requestData map[string]interface{}) string {
	// 如果 dataService 未初始化，跳过
	if s.dataService == nil {
		logger.Printf("⚠️  DataService 未初始化，跳过 system_prompt 处理")
		return ""
	}
	
	// 获取前端传来的 role_name
	roleName, _ := requestData["role_name"].(string)
	if roleName == "" {
		roleName = "general_assistant" // 默认使用通用助手
	}
	
	logger.Printf("📌 请求的 role_name: %s", roleName)
	
	// 构建最终的 system_prompt
	var finalSystemPrompt strings.Builder
	
	// 1. 如果不是 general_assistant，且也不是 title_generator，先拼接 general_assistant 的 system_prompt
	lowerRoleName := strings.ToLower(roleName)
	isGeneralAssistant := lowerRoleName == "general_assistant"
	isTitleGenerator := lowerRoleName == "title_generator"
	if !isGeneralAssistant && !isTitleGenerator {
		generalPrompt, err := s.dataService.GetSystemPrompt("general_assistant")
		if err != nil {
			logger.Printf("⚠️  获取 general_assistant 的 system_prompt 失败: %v", err)
		} else if generalPrompt != "" {
			finalSystemPrompt.WriteString(generalPrompt)
			logger.Printf("✅ 已拼接 general_assistant 的 system_prompt (长度: %d)", len(generalPrompt))
		}
	}
	
	// 2. 拼接当前时间（所有角色都需要）
	currentTime := time.Now().Format("2006-01-02 15:04:05")
	timeInfo := fmt.Sprintf("\n\n当前时间：%s", currentTime)
	finalSystemPrompt.WriteString(timeInfo)
	logger.Printf("✅ 已拼接当前时间: %s", currentTime)
	
	// 3. 如果不是 general_assistant，拼接当前角色的 system_prompt
	if !isGeneralAssistant {
		rolePrompt, err := s.dataService.GetSystemPrompt(roleName)
		if err != nil {
			logger.Printf("❌ 获取 system_prompt 失败 (role_name: %s): %v，将使用前端传来的 system 消息", roleName, err)
			// 如果获取失败，返回已拼接的 general_assistant + 时间
			return finalSystemPrompt.String()
		}
		
		if rolePrompt == "" {
			logger.Printf("⚠️  未找到 role_name=%s 对应的 system_prompt，将使用前端传来的 system 消息", roleName)
			// 如果为空，返回已拼接的 general_assistant + 时间
			return finalSystemPrompt.String()
		}
		
		// 拼接当前角色的 system_prompt
		finalSystemPrompt.WriteString("\n\n")
		finalSystemPrompt.WriteString(rolePrompt)
		
		// 截断显示前200个字符
		preview := rolePrompt
		if len(preview) > 200 {
			preview = preview[:200] + "..."
		}
		logger.Printf("✅ 成功获取并拼接当前角色的 system_prompt (role_name: %s, 长度: %d，前200字符: %s)", 
			roleName, len(rolePrompt), preview)
	} else {
		// general_assistant 自身，只需要获取自己的 system_prompt
		rolePrompt, err := s.dataService.GetSystemPrompt(roleName)
		if err != nil {
			logger.Printf("❌ 获取 system_prompt 失败 (role_name: %s): %v，将使用前端传来的 system 消息", roleName, err)
			// 如果获取失败，返回已拼接的时间
			return finalSystemPrompt.String()
		}
		
		if rolePrompt == "" {
			logger.Printf("⚠️  未找到 role_name=%s 对应的 system_prompt，将使用前端传来的 system 消息", roleName)
			// 如果为空，返回已拼接的时间
			return finalSystemPrompt.String()
		}
		
		// 拼接 general_assistant 自身的 system_prompt
		finalSystemPrompt.WriteString("\n\n")
		finalSystemPrompt.WriteString(rolePrompt)
		
		// 截断显示前200个字符
		preview := rolePrompt
		if len(preview) > 200 {
			preview = preview[:200] + "..."
		}
		logger.Printf("✅ 成功获取并拼接 general_assistant 的 system_prompt (长度: %d，前200字符: %s)", 
			len(rolePrompt), preview)
	}
	
	// 4. 如果是 storagebox 相关的角色，需要拼接 user_id
	if strings.HasPrefix(strings.ToLower(roleName), "storagebox") {
		// 获取 user_id（可能在不同的字段中）
		userId, _ := requestData["_user_id"].(string)
		if userId == "" {
			// 兼容调用方仅传递 user_id 的情况（如 WebSocket 路径）
			if uid, ok := requestData["user_id"].(string); ok && uid != "" {
				userId = uid
			}
		}
		if userId == "" {
			// 再次兜底：少数路径可能使用 user 字段
			if uid, ok := requestData["user"].(string); ok && uid != "" {
				userId = uid
			}
		}
		
		if userId != "" {
			// 拼接 user_id 到 system_prompt 中
			userIdInfo := fmt.Sprintf("\n\n重要提示：当前用户的 user_id 是 %s。在处理用户相关数据时，请使用此 user_id 进行查询和操作。", userId)
			finalSystemPrompt.WriteString(userIdInfo)
			logger.Printf("✅ 已为 storagebox 角色拼接 user_id: %s (role_name: %s)", userId, roleName)
		} else {
			logger.Printf("⚠️  storagebox 角色未找到 user_id，无法拼接 (role_name: %s)", roleName)
		}
	}
	
	// 注意：不在这里删除 role_name，因为流式请求可能需要再次使用
	// 清理工作将在最终发送给 AI API 之前进行
	
	result := finalSystemPrompt.String()
	logger.Printf("✅ 最终 system_prompt 总长度: %d", len(result))
	
	return result
}