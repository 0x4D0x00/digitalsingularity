package http

import (
	"fmt"
	"strings"

	relationship "digitalsingularity/backend/communicationsystem/relationship_management"
	ws "digitalsingularity/backend/main/websocket"
)

// handleRelationshipManagementRequest 处理通信系统关系管理请求（第三层动作使用action）
func handleRelationshipManagementRequest(requestID string, data map[string]interface{}) map[string]interface{} {
	// 获取动作类型
	actionVal, ok := data["action"].(string)
	if !ok {
		logger.Printf("[%s] 缺少action", requestID)
		return map[string]interface{}{
			"status":  "fail",
			"message": "缺少动作类型",
		}
	}

	action := strings.TrimSpace(actionVal)
	if action == "" {
		logger.Printf("[%s] action为空", requestID)
		return map[string]interface{}{
			"status":  "fail",
			"message": "缺少动作类型",
		}
	}

	logger.Printf("[%s] 通信系统关系管理动作: %s", requestID, action)

	// 获取auth_token
	authToken, ok := data["auth_token"].(string)
	if !ok || authToken == "" {
		logger.Printf("[%s] 缺少auth_token", requestID)
		return map[string]interface{}{
			"status":  "fail",
			"message": "缺少auth_token",
		}
	}

	// 验证auth_token并获取用户ID
	valid, payload := authTokenService.VerifyAuthToken(authToken)
	if !valid {
		logger.Printf("[%s] auth_token验证失败", requestID)
		return map[string]interface{}{
			"status":  "fail",
			"message": "无效的auth_token",
		}
	}

	// 从auth_token payload中提取用户ID
	payloadMap, ok := payload.(map[string]interface{})
	if !ok {
		logger.Printf("[%s] auth_token payload格式错误", requestID)
		return map[string]interface{}{
			"status":  "fail",
			"message": "auth_token格式错误",
		}
	}

	userID, ok := payloadMap["userId"].(string)
	if !ok || userID == "" {
		logger.Printf("[%s] 无法从auth_token获取用户ID", requestID)
		return map[string]interface{}{
			"status":  "fail",
			"message": "无法获取用户ID",
		}
	}

	// 根据userID获取user_communication_id
	userCommunicationID, err := relationship.GetCommunicationIDByUserID(userID)
	if err != nil {
		logger.Printf("[%s] 获取user_communication_id失败: %v", requestID, err)
		return map[string]interface{}{
			"status":  "fail",
			"message": "无法获取用户通信ID",
		}
	}

	logger.Printf("[%s] 用户身份验证成功: userID=%s, userCommunicationID=%s", requestID, userID, userCommunicationID)

	switch action {
	// 好友相关操作
	case "friendAdd":
		return handleFriendAdd(requestID, userCommunicationID, data)
	case "friendAccept":
		return handleFriendAccept(requestID, userCommunicationID, data)
	case "friendReject":
		return handleFriendReject(requestID, userCommunicationID, data)
	case "friendRemove":
		return handleFriendRemove(requestID, userCommunicationID, data)
	case "friendUpdate":
		return handleFriendUpdate(requestID, userCommunicationID, data)
	case "friendCheck":
		return handleFriendCheck(requestID, userCommunicationID, data)
	case "friendList":
		return handleFriendList(requestID, userCommunicationID, data)
	case "friendRequests":
		return handleFriendRequests(requestID, userCommunicationID)
	case "friendImport":
		return handleFriendImport(requestID, userCommunicationID, data)
	case "friendStats":
		return handleFriendStats(requestID, userCommunicationID)
	case "searchUser":
		return handleSearchUser(requestID, userCommunicationID, data)

	// 关注相关操作
	case "followAdd":
		return handleFollowAdd(requestID, userCommunicationID, data)
	case "followRemove":
		return handleFollowRemove(requestID, userCommunicationID, data)
	case "followList":
		return handleFollowList(requestID, userCommunicationID)
	case "followerList":
		return handleFollowerList(requestID, userCommunicationID)
	case "mutualList":
		return handleMutualList(requestID, userCommunicationID)
	case "followCheck":
		return handleFollowCheck(requestID, userCommunicationID, data)
	case "followStats":
		return handleFollowStats(requestID, userCommunicationID)

	default:
		logger.Printf("[%s] 未知的通信系统动作: %s", requestID, action)
		return map[string]interface{}{
			"status":  "fail",
			"message": fmt.Sprintf("未知的动作: %s", action),
		}
	}
}

// handleFriendAdd 处理添加好友请求
func handleFriendAdd(requestID, userCommunicationID string, data map[string]interface{}) map[string]interface{} {
	friendCommunicationID, ok := data["friend_communication_id"].(string)
	if !ok {
		return map[string]interface{}{
			"status":  "fail",
			"message": "缺少好友通信ID",
		}
	}

	remark, _ := data["remark"].(string)

	err := relationship.AddFriend(userCommunicationID, friendCommunicationID, remark)
	if err != nil {
		logger.Printf("[%s] 添加好友失败: %v", requestID, err)
		return map[string]interface{}{
			"status":  "fail",
			"message": err.Error(),
		}
	}

	logger.Printf("[%s] 添加好友成功: %s -> %s", requestID, userCommunicationID, friendCommunicationID)

	// 通过 WebSocket 通知对方有一条新的好友请求
	// 需要将 communication_id 转换为 user_id
	if friendUserID, err := relationship.GetUserIDByCommunicationID(friendCommunicationID); err == nil {
		ws.SendNotification(friendUserID, "friend_request_received", map[string]interface{}{
			"sender_communication_id": userCommunicationID,
			"sender_nickname":         "", // 可以从数据库查询发送者昵称
			"remark":                  remark,
		})
	} else {
		logger.Printf("[%s] 转换好友communication_id失败: %v", requestID, err)
	}

	return map[string]interface{}{
		"status":  "success",
		"message": "好友请求已发送",
	}
}

// handleFriendAccept 处理接受好友请求
func handleFriendAccept(requestID, userCommunicationID string, data map[string]interface{}) map[string]interface{} {
	friendCommunicationID, ok := data["friend_communication_id"].(string)
	if !ok {
		return map[string]interface{}{
			"status":  "fail",
			"message": "缺少好友通信ID",
		}
	}

	remark, _ := data["remark"].(string)

	if err := relationship.AcceptFriend(userCommunicationID, friendCommunicationID, remark); err != nil {
		logger.Printf("[%s] 接受好友请求失败: %v", requestID, err)
		return map[string]interface{}{
			"status":  "fail",
			"message": err.Error(),
		}
	}

	logger.Printf("[%s] 接受好友请求成功: %s <- %s", requestID, userCommunicationID, friendCommunicationID)

	// 通知对方：请求已被接受
	// 需要将 communication_id 转换为 user_id
	if friendUserID, err := relationship.GetUserIDByCommunicationID(friendCommunicationID); err == nil {
		ws.SendNotification(friendUserID, "friend_request_accepted", map[string]interface{}{
			"friend_communication_id": userCommunicationID,
			"remark":                  remark,
		})
	} else {
		logger.Printf("[%s] 转换好友communication_id失败: %v", requestID, err)
	}

	return map[string]interface{}{
		"status":  "success",
		"message": "已接受好友请求",
	}
}

// handleFriendReject 处理拒绝好友请求
func handleFriendReject(requestID, userCommunicationID string, data map[string]interface{}) map[string]interface{} {
	friendCommunicationID, ok := data["friend_communication_id"].(string)
	if !ok {
		return map[string]interface{}{
			"status":  "fail",
			"message": "缺少好友通信ID",
		}
	}

	if err := relationship.RejectFriend(userCommunicationID, friendCommunicationID); err != nil {
		logger.Printf("[%s] 拒绝好友请求失败: %v", requestID, err)
		return map[string]interface{}{
			"status":  "fail",
			"message": err.Error(),
		}
	}

	logger.Printf("[%s] 拒绝好友请求成功: %s <- %s", requestID, userCommunicationID, friendCommunicationID)

	// 通知对方（请求发送者）被拒绝
	// 需要将 communication_id 转换为 user_id
	if friendUserID, err := relationship.GetUserIDByCommunicationID(friendCommunicationID); err == nil {
		ws.SendNotification(friendUserID, "friend_request_rejected", map[string]interface{}{
			"friend_communication_id": userCommunicationID,
		})
	} else {
		logger.Printf("[%s] 转换好友communication_id失败: %v", requestID, err)
	}

	return map[string]interface{}{
		"status":  "success",
		"message": "已拒绝好友请求",
	}
}

// handleFriendRemove 处理删除好友请求
func handleFriendRemove(requestID, userCommunicationID string, data map[string]interface{}) map[string]interface{} {
	friendCommunicationID, ok := data["friend_communication_id"].(string)
	if !ok {
		return map[string]interface{}{
			"status":  "fail",
			"message": "缺少好友通信ID",
		}
	}

	err := relationship.RemoveFriend(userCommunicationID, friendCommunicationID)
	if err != nil {
		logger.Printf("[%s] 删除好友失败: %v", requestID, err)
		return map[string]interface{}{
			"status":  "fail",
			"message": err.Error(),
		}
	}

	logger.Printf("[%s] 删除好友成功: %s -> %s", requestID, userCommunicationID, friendCommunicationID)
	return map[string]interface{}{
		"status":  "success",
		"message": "删除好友成功",
	}
}

// handleFriendUpdate 处理更新好友备注请求
func handleFriendUpdate(requestID, userCommunicationID string, data map[string]interface{}) map[string]interface{} {
	friendCommunicationID, ok := data["friend_communication_id"].(string)
	if !ok {
		return map[string]interface{}{
			"status":  "fail",
			"message": "缺少好友通信ID",
		}
	}

	remark, ok := data["remark"].(string)
	if !ok {
		return map[string]interface{}{
			"status":  "fail",
			"message": "缺少备注信息",
		}
	}

	err := relationship.UpdateFriend(userCommunicationID, friendCommunicationID, remark)
	if err != nil {
		logger.Printf("[%s] 更新好友备注失败: %v", requestID, err)
		return map[string]interface{}{
			"status":  "fail",
			"message": err.Error(),
		}
	}

	logger.Printf("[%s] 更新好友备注成功: %s -> %s", requestID, userCommunicationID, friendCommunicationID)
	return map[string]interface{}{
		"status":  "success",
		"message": "更新好友备注成功",
	}
}

// handleFriendCheck 处理检查好友关系请求
func handleFriendCheck(requestID, userCommunicationID string, data map[string]interface{}) map[string]interface{} {
	friendCommunicationID, ok := data["friend_communication_id"].(string)
	if !ok {
		return map[string]interface{}{
			"status":  "fail",
			"message": "缺少好友通信ID",
		}
	}

	isFriend, err := relationship.CheckFriend(userCommunicationID, friendCommunicationID)
	if err != nil {
		logger.Printf("[%s] 检查好友关系失败: %v", requestID, err)
		return map[string]interface{}{
			"status":  "fail",
			"message": err.Error(),
		}
	}

	return map[string]interface{}{
		"status":  "success",
		"data":    map[string]interface{}{"is_friend": isFriend},
		"message": "检查完成",
	}
}

// handleFriendList 处理获取好友列表请求
func handleFriendList(requestID, userCommunicationID string, data map[string]interface{}) map[string]interface{} {
	status, _ := data["status"].(string)

	friends, err := relationship.GetFriends(userCommunicationID, status)
	if err != nil {
		logger.Printf("[%s] 获取好友列表失败: %v", requestID, err)
		return map[string]interface{}{
			"status":  "fail",
			"message": err.Error(),
		}
	}

	// 转换为JSON兼容格式，包含完整的用户信息
	friendsData := make([]map[string]interface{}, len(friends))
	for i, friend := range friends {
		friendData := map[string]interface{}{
			"user_communication_id":     friend.UserCommunicationID,
			"friend_communication_id":   friend.FriendCommunicationID,
			"remark":                    friend.Remark,
			"status":                    friend.Status,
			"created_at":                friend.CreatedAt.Format("2006-01-02 15:04:05"),
			"updated_at":                friend.UpdatedAt.Format("2006-01-02 15:04:05"),
			// 好友的详细用户信息
			"user_name":                 friend.UserName,
			"phone":                     friend.Phone,
			"email":                     friend.Email,
			"nickname":                  friend.Nickname,
			"avatar_url":                friend.AvatarURL,
			"bio":                       friend.Bio,
			"user_created_at":           friend.UserCreatedAt.Format("2006-01-02 15:04:05"),
			"user_updated_at":           friend.UserUpdatedAt.Format("2006-01-02 15:04:05"),
			"gender":                    friend.Gender,
			"personal_assistant_enabled": friend.PersonalAssistantEnabled,
		}
		if friend.DeletedAt != nil {
			friendData["deleted_at"] = friend.DeletedAt.Format("2006-01-02 15:04:05")
		}
		if friend.LastLoginAt != nil {
			friendData["last_login_at"] = friend.LastLoginAt.Format("2006-01-02 15:04:05")
		}
		if friend.BirthDate != nil {
			friendData["birth_date"] = friend.BirthDate.Format("2006-01-02")
		}
		friendsData[i] = friendData
	}

	return map[string]interface{}{
		"status":  "success",
		"data":    map[string]interface{}{"friends": friendsData},
		"message": "获取好友列表成功",
	}
}

// handleFriendRequests 获取收到的好友请求列表
func handleFriendRequests(requestID, userCommunicationID string) map[string]interface{} {
	requests, err := relationship.GetFriendRequests(userCommunicationID)
	if err != nil {
		logger.Printf("[%s] 获取好友请求列表失败: %v", requestID, err)
		return map[string]interface{}{
			"status":  "fail",
			"message": err.Error(),
		}
	}

	requestsData := make([]map[string]interface{}, len(requests))
	for i, f := range requests {
		requestsData[i] = map[string]interface{}{
			"user_communication_id":   f.UserCommunicationID,   // 发起请求方
			"friend_communication_id": f.FriendCommunicationID, // 当前用户
			"remark":                  f.Remark,
			"status":                  f.Status,
			"created_at":              f.CreatedAt.Format("2006-01-02 15:04:05"),
			"updated_at":              f.UpdatedAt.Format("2006-01-02 15:04:05"),
			"user_name":               f.UserName,
			"phone":                   f.Phone,
			"email":                   f.Email,
			"nickname":                f.Nickname,
			"avatar_url":              f.AvatarURL,
			"bio":                     f.Bio,
			"gender":                  f.Gender,
			"personal_assistant_enabled": f.PersonalAssistantEnabled,
		}
		if f.DeletedAt != nil {
			requestsData[i]["deleted_at"] = f.DeletedAt.Format("2006-01-02 15:04:05")
		}
		if f.LastLoginAt != nil {
			requestsData[i]["last_login_at"] = f.LastLoginAt.Format("2006-01-02 15:04:05")
		}
		if f.BirthDate != nil {
			requestsData[i]["birth_date"] = f.BirthDate.Format("2006-01-02")
		}
		if !f.UserCreatedAt.IsZero() {
			requestsData[i]["user_created_at"] = f.UserCreatedAt.Format("2006-01-02 15:04:05")
		}
		if !f.UserUpdatedAt.IsZero() {
			requestsData[i]["user_updated_at"] = f.UserUpdatedAt.Format("2006-01-02 15:04:05")
		}
	}

	return map[string]interface{}{
		"status":  "success",
		"data":    map[string]interface{}{"friends": requestsData},
		"message": "获取好友请求列表成功",
	}
}

// handleFriendImport 处理批量导入好友请求
func handleFriendImport(requestID, userCommunicationID string, data map[string]interface{}) map[string]interface{} {
	friendCommunicationIDsInterface, ok := data["friend_communication_ids"].([]interface{})
	if !ok {
		return map[string]interface{}{
			"status":  "fail",
			"message": "缺少好友通信ID列表",
		}
	}

	friendCommunicationIDs := make([]string, 0, len(friendCommunicationIDsInterface))
	for _, id := range friendCommunicationIDsInterface {
		if idStr, ok := id.(string); ok {
			friendCommunicationIDs = append(friendCommunicationIDs, idStr)
		}
	}

	err := relationship.ImportFriends(userCommunicationID, friendCommunicationIDs)
	if err != nil {
		logger.Printf("[%s] 批量导入好友失败: %v", requestID, err)
		return map[string]interface{}{
			"status":  "fail",
			"message": err.Error(),
		}
	}

	logger.Printf("[%s] 批量导入好友成功: %s, 数量: %d", requestID, userCommunicationID, len(friendCommunicationIDs))
	return map[string]interface{}{
		"status":  "success",
		"message": "批量导入好友成功",
	}
}

// handleFriendStats 处理获取好友数量请求
func handleFriendStats(requestID, userCommunicationID string) map[string]interface{} {
	count, err := relationship.GetFriendStats(userCommunicationID)
	if err != nil {
		logger.Printf("[%s] 获取好友数量失败: %v", requestID, err)
		return map[string]interface{}{
			"status":  "fail",
			"message": err.Error(),
		}
	}

	return map[string]interface{}{
		"status":  "success",
		"data":    map[string]interface{}{"friend_count": count},
		"message": "获取好友数量成功",
	}
}

// handleFollowAdd 处理关注用户请求
func handleFollowAdd(requestID, userCommunicationID string, data map[string]interface{}) map[string]interface{} {
	followingCommunicationID, ok := data["following_communication_id"].(string)
	if !ok {
		return map[string]interface{}{
			"status":  "fail",
			"message": "缺少被关注用户通信ID",
		}
	}

	err := relationship.FollowUser(userCommunicationID, followingCommunicationID)
	if err != nil {
		logger.Printf("[%s] 关注用户失败: %v", requestID, err)
		return map[string]interface{}{
			"status":  "fail",
			"message": err.Error(),
		}
	}

	logger.Printf("[%s] 关注用户成功: %s -> %s", requestID, userCommunicationID, followingCommunicationID)
	return map[string]interface{}{
		"status":  "success",
		"message": "关注用户成功",
	}
}

// handleFollowRemove 处理取消关注请求
func handleFollowRemove(requestID, userCommunicationID string, data map[string]interface{}) map[string]interface{} {
	followingCommunicationID, ok := data["following_communication_id"].(string)
	if !ok {
		return map[string]interface{}{
			"status":  "fail",
			"message": "缺少被关注用户通信ID",
		}
	}

	err := relationship.UnfollowUser(userCommunicationID, followingCommunicationID)
	if err != nil {
		logger.Printf("[%s] 取消关注失败: %v", requestID, err)
		return map[string]interface{}{
			"status":  "fail",
			"message": err.Error(),
		}
	}

	logger.Printf("[%s] 取消关注成功: %s -> %s", requestID, userCommunicationID, followingCommunicationID)
	return map[string]interface{}{
		"status":  "success",
		"message": "取消关注成功",
	}
}

// handleFollowList 处理获取关注列表请求
func handleFollowList(requestID, userCommunicationID string) map[string]interface{} {
	users, err := relationship.GetFollowing(userCommunicationID)
	if err != nil {
		logger.Printf("[%s] 获取关注列表失败: %v", requestID, err)
		return map[string]interface{}{
			"status":  "fail",
			"message": err.Error(),
		}
	}

	usersData := make([]map[string]interface{}, len(users))
	for i, user := range users {
		usersData[i] = map[string]interface{}{
			"user_communication_id": user.UserCommunicationID,
			"nickname":              user.Nickname,
			"avatar_url":            user.AvatarURL,
		}
	}

	return map[string]interface{}{
		"status":  "success",
		"data":    map[string]interface{}{"following": usersData},
		"message": "获取关注列表成功",
	}
}

// handleFollowerList 处理获取粉丝列表请求
func handleFollowerList(requestID, userCommunicationID string) map[string]interface{} {
	users, err := relationship.GetFollowers(userCommunicationID)
	if err != nil {
		logger.Printf("[%s] 获取粉丝列表失败: %v", requestID, err)
		return map[string]interface{}{
			"status":  "fail",
			"message": err.Error(),
		}
	}

	usersData := make([]map[string]interface{}, len(users))
	for i, user := range users {
		usersData[i] = map[string]interface{}{
			"user_communication_id": user.UserCommunicationID,
			"nickname":              user.Nickname,
			"avatar_url":            user.AvatarURL,
		}
	}

	return map[string]interface{}{
		"status":  "success",
		"data":    map[string]interface{}{"followers": usersData},
		"message": "获取粉丝列表成功",
	}
}

// handleMutualList 处理获取互关列表请求
func handleMutualList(requestID, userCommunicationID string) map[string]interface{} {
	users, err := relationship.GetMutualFollows(userCommunicationID)
	if err != nil {
		logger.Printf("[%s] 获取互关列表失败: %v", requestID, err)
		return map[string]interface{}{
			"status":  "fail",
			"message": err.Error(),
		}
	}

	usersData := make([]map[string]interface{}, len(users))
	for i, user := range users {
		usersData[i] = map[string]interface{}{
			"user_communication_id": user.UserCommunicationID,
			"nickname":              user.Nickname,
			"avatar_url":            user.AvatarURL,
		}
	}

	return map[string]interface{}{
		"status":  "success",
		"data":    map[string]interface{}{"mutual_follows": usersData},
		"message": "获取互关列表成功",
	}
}

// handleFollowCheck 处理检查关注关系请求
func handleFollowCheck(requestID, userCommunicationID string, data map[string]interface{}) map[string]interface{} {
	followingCommunicationID, ok := data["following_communication_id"].(string)
	if !ok {
		return map[string]interface{}{
			"status":  "fail",
			"message": "缺少被关注用户通信ID",
		}
	}

	isFollowing, err := relationship.CheckFollow(userCommunicationID, followingCommunicationID)
	if err != nil {
		logger.Printf("[%s] 检查关注关系失败: %v", requestID, err)
		return map[string]interface{}{
			"status":  "fail",
			"message": err.Error(),
		}
	}

	return map[string]interface{}{
		"status":  "success",
		"data":    map[string]interface{}{"is_following": isFollowing},
		"message": "检查完成",
	}
}

// handleFollowStats 处理获取关注统计请求
func handleFollowStats(requestID, userCommunicationID string) map[string]interface{} {
	stats, err := relationship.GetFollowStats(userCommunicationID)
	if err != nil {
		logger.Printf("[%s] 获取关注统计失败: %v", requestID, err)
		return map[string]interface{}{
			"status":  "fail",
			"message": err.Error(),
		}
	}

	return map[string]interface{}{
		"status":  "success",
		"data": map[string]interface{}{
			"follower_count":     stats.FollowerCount,
			"following_count":    stats.FollowingCount,
			"mutual_follow_count": stats.MutualFollowCount,
		},
		"message": "获取关注统计成功",
	}
}

// handleSearchUser 处理搜索用户请求
func handleSearchUser(requestID, userCommunicationID string, data map[string]interface{}) map[string]interface{} {
	keyword, ok := data["keyword"].(string)
	if !ok || keyword == "" {
		return map[string]interface{}{
			"status":  "fail",
			"message": "缺少搜索关键词",
		}
	}

	users, err := relationship.SearchUser(userCommunicationID, keyword)
	if err != nil {
		logger.Printf("[%s] 搜索用户失败: %v", requestID, err)
		return map[string]interface{}{
			"status":  "fail",
			"message": err.Error(),
		}
	}

	// 转换为JSON兼容格式
	usersData := make([]map[string]interface{}, len(users))
	for i, user := range users {
		userData := map[string]interface{}{
			"user_communication_id":     user.UserCommunicationID,
			"friend_communication_id":   user.FriendCommunicationID,
			"remark":                    user.Remark,
			"status":                    user.Status,
			"created_at":                user.CreatedAt.Format("2006-01-02 15:04:05"),
			"updated_at":                user.UpdatedAt.Format("2006-01-02 15:04:05"),
			// 用户详细信息
			"user_name":                 user.UserName,
			"phone":                     user.Phone,
			"email":                     user.Email,
			"nickname":                  user.Nickname,
			"avatar_url":                user.AvatarURL,
			"bio":                       user.Bio,
			"user_created_at":           user.UserCreatedAt.Format("2006-01-02 15:04:05"),
			"user_updated_at":           user.UserUpdatedAt.Format("2006-01-02 15:04:05"),
			"gender":                    user.Gender,
			"personal_assistant_enabled": user.PersonalAssistantEnabled,
		}
		if user.DeletedAt != nil {
			userData["deleted_at"] = user.DeletedAt.Format("2006-01-02 15:04:05")
		}
		if user.LastLoginAt != nil {
			userData["last_login_at"] = user.LastLoginAt.Format("2006-01-02 15:04:05")
		}
		if user.BirthDate != nil {
			userData["birth_date"] = user.BirthDate.Format("2006-01-02")
		}
		usersData[i] = userData
	}

	logger.Printf("[%s] 搜索用户成功: 找到 %d 个用户", requestID, len(users))
	return map[string]interface{}{
		"status":  "success",
		"data":    map[string]interface{}{"users": usersData},
		"message": "搜索用户成功",
	}
}

