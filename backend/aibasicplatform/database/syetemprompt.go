package database

import (
	"encoding/json"
	"fmt"
	"log"
)

// LoadPromptsToRedis 在系统启动时将系统提示词加载到Redis
// 从aibasicplatform.aibasicplatform_system_prompt表读取提示词信息并缓存到Redis中
// 使用 role_name 作为 Redis key 格式 (system_prompt:{role_name})
func (s *AIBasicPlatformDataService) LoadPromptsToRedis() error {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("加载系统提示词到Redis异常: %v", r)
		}
	}()

	// 从数据库查询所有启用的系统提示词（只加载 enabled=1 的）
	query := `
		SELECT role_type, system_prompt, role_name, description, priority, is_internal
		FROM aibasicplatform.aibasicplatform_system_prompt
		WHERE enabled = 1 AND system_prompt IS NOT NULL AND system_prompt != ''
		ORDER BY priority DESC
	`

	opResult := s.readWrite.QueryDb(query)
	if !opResult.IsSuccess() {
		log.Printf("[AIBasicPlatform] 查询系统提示词失败: %v", opResult.Error)
		return opResult.Error
	}

	rows, ok := opResult.Data.([]map[string]interface{})
	if !ok {
		log.Printf("[AIBasicPlatform] 系统提示词数据格式错误")
		return nil
	}

	log.Printf("[AIBasicPlatform] 从 aibasicplatform.aibasicplatform_system_prompt 表读取到 %d 个系统提示词", len(rows))

	loadedCount := 0
	skippedCount := 0

	// 将每个提示词写入Redis，使用 role_name 作为 key
	for _, row := range rows {
		roleName, ok := row["role_name"].(string)
		if !ok || roleName == "" {
			log.Printf("[AIBasicPlatform] 角色名称格式错误或为空，跳过该提示词")
			skippedCount++
			continue
		}

		systemPrompt, ok := row["system_prompt"].(string)
		if !ok || systemPrompt == "" {
			log.Printf("[AIBasicPlatform] 角色 %s 的 system_prompt 为空，跳过", roleName)
			skippedCount++
			continue
		}

		// 使用 role_name 作为 Redis key
		redisKey := "system_prompt:" + roleName

		// 提取其他字段
		// MySQL的tinyint(1)可能被解析为多种类型，需要支持所有可能的类型
		isInternal := 0
		if internal, ok := row["is_internal"].(int64); ok {
			isInternal = int(internal)
		} else if internal, ok := row["is_internal"].(int); ok {
			isInternal = internal
		} else if internal, ok := row["is_internal"].(uint8); ok {
			isInternal = int(internal)
		} else if internal, ok := row["is_internal"].(uint64); ok {
			isInternal = int(internal)
		} else if internal, ok := row["is_internal"].(int32); ok {
			isInternal = int(internal)
		} else if internal, ok := row["is_internal"].(uint32); ok {
			isInternal = int(internal)
		} else if internal, ok := row["is_internal"].(bool); ok {
			if internal {
				isInternal = 1
			}
		} else if internalBytes, ok := row["is_internal"].([]uint8); ok {
			// MySQL可能返回字节数组，需要转换为数字
			if len(internalBytes) > 0 {
				isInternal = int(internalBytes[0])
			}
		} else if internalStr, ok := row["is_internal"].(string); ok {
			// 处理字符串类型（虽然不应该出现，但为了健壮性）
			if internalStr == "1" || internalStr == "true" {
				isInternal = 1
			}
		}

		// 添加调试日志
		if roleName == "title_generator" || roleName == "training_data_collection_expert" {
			log.Printf("[AIBasicPlatform] 🔍 DEBUG LoadPromptsToRedis role_name=%s is_internal 类型: %T, 值: %v, 转换后: %d",
				roleName, row["is_internal"], row["is_internal"], isInternal)
		}

		roleType := ""
		if rt, ok := row["role_type"].(string); ok {
			roleType = rt
		}

		description := ""
		if desc, ok := row["description"].(string); ok {
			description = desc
		}

		// 构建完整的角色信息 JSON 对象
		roleInfo := map[string]interface{}{
			"system_prompt": systemPrompt,
			"is_internal":   isInternal,
			"role_name":     roleName,
			"role_type":     roleType,
		}
		if description != "" {
			roleInfo["description"] = description
		}

		// 序列化为 JSON
		roleInfoJSON, err := json.Marshal(roleInfo)
		if err != nil {
			log.Printf("[AIBasicPlatform] 序列化角色信息失败 %s: %v", roleName, err)
			skippedCount++
			continue
		}

		// 将完整的角色信息存储到Redis，设置永不过期
		opResult := s.readWrite.RedisWrite(redisKey, string(roleInfoJSON), 0)
		if !opResult.IsSuccess() {
			log.Printf("[AIBasicPlatform] 写入角色 %s 的完整信息到Redis失败: %v", roleName, opResult.Error)
			skippedCount++
			continue
		}

		log.Printf("[AIBasicPlatform] ✅ 成功加载系统提示词到Redis: %s (system_prompt长度: %d, is_internal: %d)",
			roleName, len(systemPrompt), isInternal)
		loadedCount++
	}

	log.Printf("[AIBasicPlatform] 加载完成: 成功 %d 个，跳过 %d 个", loadedCount, skippedCount)

	// 同时保存所有角色名称的列表到单独的key，方便查询
	roleNames := make([]string, 0, len(rows))
	for _, row := range rows {
		if roleName, ok := row["role_name"].(string); ok && roleName != "" {
			roleNames = append(roleNames, roleName)
		}
	}

	listKey := "aibasicplatform:prompt:list"
	opResult = s.readWrite.RedisWrite(listKey, roleNames, 0)
	if !opResult.IsSuccess() {
		log.Printf("[AIBasicPlatform] 写入提示词列表到Redis失败: %v", opResult.Error)
	} else {
		log.Printf("[AIBasicPlatform] 成功加载提示词列表到Redis: %v", roleNames)
	}

	return nil
}

// GetSystemPrompt 从 Redis 获取系统提示词
// 使用 role_name 作为 Redis key 格式 (system_prompt:{role_name})
// 要求数据必须是 JSON 格式，包含 system_prompt 字段
// 先尝试从 datahandle 的 GetAllSystemPromptsFromRedis 获取，如果没有再加载
func (s *AIBasicPlatformDataService) GetSystemPrompt(roleName string) (string, error) {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("获取系统提示词异常: %v", r)
		}
	}()

	if roleName == "" {
		roleName = "通用助手" // 默认使用通用助手
	}

	// 先尝试从 datahandle 的 GetAllSystemPromptsFromRedis 获取
	prompts, err := s.readWrite.GetAllSystemPromptsFromRedis()
	if err == nil && len(prompts) > 0 {
		// 在结果中查找匹配的 roleName
		// 注意：prompts 中只有 role_type，需要通过 role_name 从 Redis 查找
		redisKey := "system_prompt:" + roleName
		result := s.readWrite.RedisRead(redisKey)
		if result.IsSuccess() {
			var roleInfo map[string]interface{}
			switch data := result.Data.(type) {
			case string:
				if data != "" {
					if err := json.Unmarshal([]byte(data), &roleInfo); err == nil {
						if systemPrompt, ok := roleInfo["system_prompt"].(string); ok && systemPrompt != "" {
							log.Printf("[AIBasicPlatform] ✅ 成功从Redis获取 system_prompt (role_name: %s, 长度: %d)", roleName, len(systemPrompt))
							return systemPrompt, nil
						}
					}
				}
			case map[string]interface{}:
				roleInfo = data
				if systemPrompt, ok := roleInfo["system_prompt"].(string); ok && systemPrompt != "" {
					log.Printf("[AIBasicPlatform] ✅ 成功从Redis获取 system_prompt (role_name: %s, 长度: %d)", roleName, len(systemPrompt))
					return systemPrompt, nil
				}
			}
		}
	}

	// 如果没有找到，先加载提示词到Redis
	log.Printf("[AIBasicPlatform] Redis中没有找到 system_prompt (role_name: %s)，尝试重新加载", roleName)
	if err := s.LoadPromptsToRedis(); err != nil {
		log.Printf("[AIBasicPlatform] 加载提示词到Redis失败: %v", err)
		return "", fmt.Errorf("加载提示词到Redis失败: %v", err)
	}

	// 加载后再次尝试从 GetAllSystemPromptsFromRedis 获取
	prompts, err = s.readWrite.GetAllSystemPromptsFromRedis()
	if err != nil {
		log.Printf("[AIBasicPlatform] 重新加载后获取 system_prompt 失败: %v", err)
		return "", fmt.Errorf("获取 system_prompt 失败: %v", err)
	}

	// 再次尝试从 Redis 读取
	redisKey := "system_prompt:" + roleName
	result := s.readWrite.RedisRead(redisKey)
	if !result.IsSuccess() {
		log.Printf("[AIBasicPlatform] 从Redis读取 system_prompt 失败 (role_name: %s): %v", roleName, result.Error)
		return "", result.Error
	}

	// 处理两种情况：数据可能是string（JSON字符串）或map[string]interface{}（已解析的对象）
	var roleInfo map[string]interface{}
	switch data := result.Data.(type) {
	case string:
		// 如果是字符串，需要解析JSON
		if data == "" {
			log.Printf("[AIBasicPlatform] Redis中的 system_prompt 为空 (role_name: %s)", roleName)
			return "", fmt.Errorf("system_prompt为空")
		}
		if err := json.Unmarshal([]byte(data), &roleInfo); err != nil {
			log.Printf("[AIBasicPlatform] Redis中的 system_prompt 不是有效的JSON格式 (role_name: %s): %v", roleName, err)
			return "", fmt.Errorf("system_prompt不是有效的JSON格式: %v", err)
		}
	case map[string]interface{}:
		// 如果已经是map类型，直接使用
		roleInfo = data
	default:
		log.Printf("[AIBasicPlatform] Redis中的 system_prompt 数据类型不支持 (role_name: %s, 类型: %T)", roleName, result.Data)
		return "", fmt.Errorf("system_prompt数据类型不支持: %T", result.Data)
	}

	// 提取 system_prompt 字段
	systemPrompt, ok := roleInfo["system_prompt"].(string)
	if !ok || systemPrompt == "" {
		log.Printf("[AIBasicPlatform] Redis中的 system_prompt JSON缺少system_prompt字段或为空 (role_name: %s)", roleName)
		return "", fmt.Errorf("system_prompt JSON缺少system_prompt字段或为空")
	}

	log.Printf("[AIBasicPlatform] ✅ 成功从Redis获取 system_prompt (role_name: %s, 长度: %d)", roleName, len(systemPrompt))
	return systemPrompt, nil
}

