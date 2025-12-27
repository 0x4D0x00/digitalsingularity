package websocket

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/gorilla/websocket"
	relationship "digitalsingularity/backend/communicationsystem/relationship_management"
)

// handleRelationshipManagementMessage 处理关系管理相关的WebSocket消息
func handleRelationshipManagementMessage(conn *websocket.Conn, connectionID, userID string, messageData map[string]interface{}) {
	// 获取动作类型（第三层使用action字段保持与HTTP层一致）
	actionVal, ok := messageData["action"].(string)
	if !ok {
		logger.Printf("[WS:%s] 缺少动作类型", connectionID)
		sendErrorResponse(conn, "缺少动作类型")
		return
	}

	action := strings.TrimSpace(actionVal)
	if action == "" {
		logger.Printf("[WS:%s] 动作类型为空", connectionID)
		sendErrorResponse(conn, "缺少动作类型")
		return
	}

	logger.Printf("[WS:%s] 关系管理动作: %s, 用户: %s", connectionID, action, userID)

	var response map[string]interface{}

	switch action {
	// 好友相关操作
	case "friendAdd":
		response = toBackendWSFriendAdd(connectionID, userID, messageData)
	case "friendAccept":
		response = toBackendWSFriendAccept(connectionID, userID, messageData)
	case "friendReject":
		response = toBackendWSFriendReject(connectionID, userID, messageData)
	case "friendRemove":
		response = toBackendWSFriendRemove(connectionID, userID, messageData)
	case "followAdd":
		response = toBackendWSFollowAdd(connectionID, userID, messageData)
	case "followRemove":
		response = toBackendWSFollowRemove(connectionID, userID, messageData)
	default:
		logger.Printf("[WS:%s] 未知的关系管理操作: %s", connectionID, action)
		response = map[string]interface{}{
			"type": "relationship_error",
			"data": map[string]interface{}{
				"status":  "fail",
				"message": fmt.Sprintf("未知的操作: %s", action),
			},
		}
	}

	// 发送响应
	responseBytes, err := json.Marshal(response)
	if err != nil {
		logger.Printf("[WS:%s] 序列化响应失败: %v", connectionID, err)
		sendErrorResponse(conn, "内部错误")
		return
	}

	if err := conn.WriteMessage(websocket.TextMessage, responseBytes); err != nil {
		logger.Printf("[WS:%s] 发送响应失败: %v", connectionID, err)
	}
}

// handleWSFriendAdd 处理WebSocket添加好友请求
func toBackendWSFriendAdd(connectionID, userID string, messageData map[string]interface{}) map[string]interface{} {
	data, ok := messageData["data"].(map[string]interface{})
	if !ok {
		return map[string]interface{}{
			"type": "friend_add_response",
			"data": map[string]interface{}{
				"status":  "fail",
				"message": "缺少数据字段",
			},
		}
	}

	friendCommunicationID, ok := data["friend_communication_id"].(string)
	if !ok {
		return map[string]interface{}{
			"type": "friend_add_response",
			"data": map[string]interface{}{
				"status":  "fail",
				"message": "缺少好友Communication ID",
			},
		}
	}

	remark, _ := data["remark"].(string)

	// 将内部的 userID 转换为 user_communication_id
	userCommunicationID := getCommunicationIDOrFallback(userID)
	if userCommunicationID == userID {
		return map[string]interface{}{
			"type": "friend_add_response",
			"data": map[string]interface{}{
				"status":  "fail",
				"message": "无法获取用户通信ID",
			},
		}
	}

	err := relationship.AddFriend(userCommunicationID, friendCommunicationID, remark)
	if err != nil {
		logger.Printf("[WS:%s] 添加好友失败: %v", connectionID, err)
		return map[string]interface{}{
			"type": "friend_add_response",
			"data": map[string]interface{}{
				"status":  "fail",
				"message": err.Error(),
			},
		}
	}

	logger.Printf("[WS:%s] 好友请求已发送: %s -> %s", connectionID, userCommunicationID, friendCommunicationID)

	requesterCommID := userCommunicationID

	sendNotificationToCommunicationUser(friendCommunicationID, "friend_request_received", map[string]interface{}{
		"sender_communication_id": requesterCommID,
		"remark":                  remark,
	})

	return map[string]interface{}{
		"type": "friend_add_response",
		"data": map[string]interface{}{
			"status":  "success",
			"message": "好友请求已发送",
		},
	}
}

// handleWSFriendRemove 处理WebSocket删除好友请求
func toBackendWSFriendRemove(connectionID, userID string, messageData map[string]interface{}) map[string]interface{} {
	data, ok := messageData["data"].(map[string]interface{})
	if !ok {
		return map[string]interface{}{
			"type": "friend_remove_response",
			"data": map[string]interface{}{
				"status":  "fail",
				"message": "缺少数据字段",
			},
		}
	}

	friendCommunicationID, ok := data["friend_communication_id"].(string)
	if !ok {
		return map[string]interface{}{
			"type": "friend_remove_response",
			"data": map[string]interface{}{
				"status":  "fail",
				"message": "缺少好友Communication ID",
			},
		}
	}

	// 将内部的 userID 转换为 user_communication_id
	userCommunicationID := getCommunicationIDOrFallback(userID)
	if userCommunicationID == userID {
		return map[string]interface{}{
			"type": "friend_remove_response",
			"data": map[string]interface{}{
				"status":  "fail",
				"message": "无法获取用户通信ID",
			},
		}
	}

	err := relationship.RemoveFriend(userCommunicationID, friendCommunicationID)
	if err != nil {
		logger.Printf("[WS:%s] 删除好友失败: %v", connectionID, err)
		return map[string]interface{}{
			"type": "friend_remove_response",
			"data": map[string]interface{}{
				"status":  "fail",
				"message": err.Error(),
			},
		}
	}

	logger.Printf("[WS:%s] 删除好友成功: %s -> %s", connectionID, userCommunicationID, friendCommunicationID)

	removerCommID := userCommunicationID

	sendNotificationToCommunicationUser(friendCommunicationID, "friend_removed", map[string]interface{}{
		"user_communication_id":   removerCommID,
		"friend_communication_id": friendCommunicationID,
	})

	return map[string]interface{}{
		"type": "friend_remove_response",
		"data": map[string]interface{}{
			"status":  "success",
			"message": "删除好友成功",
		},
	}
}

// handleWSFriendAccept 处理WebSocket接受好友请求
func toBackendWSFriendAccept(connectionID, userID string, messageData map[string]interface{}) map[string]interface{} {
	data, ok := messageData["data"].(map[string]interface{})
	if !ok {
		return map[string]interface{}{
			"type": "friend_accept_response",
			"data": map[string]interface{}{
				"status":  "fail",
				"message": "缺少数据字段",
			},
		}
	}

	friendCommunicationID, ok := data["friend_communication_id"].(string)
	if !ok {
		return map[string]interface{}{
			"type": "friend_accept_response",
			"data": map[string]interface{}{
				"status":  "fail",
				"message": "缺少好友Communication ID",
			},
		}
	}

	remark, _ := data["remark"].(string)

	// 将内部的 userID 转换为 user_communication_id
	userCommunicationID := getCommunicationIDOrFallback(userID)
	if userCommunicationID == userID {
		return map[string]interface{}{
			"type": "friend_accept_response",
			"data": map[string]interface{}{
				"status":  "fail",
				"message": "无法获取用户通信ID",
			},
		}
	}

	if err := relationship.AcceptFriend(userCommunicationID, friendCommunicationID, remark); err != nil {
		logger.Printf("[WS:%s] 接受好友请求失败: %v", connectionID, err)
		return map[string]interface{}{
			"type": "friend_accept_response",
			"data": map[string]interface{}{
				"status":  "fail",
				"message": err.Error(),
			},
		}
	}

	logger.Printf("[WS:%s] 接受好友请求成功: %s <- %s", connectionID, userCommunicationID, friendCommunicationID)

	acceptorCommID := userCommunicationID
	if acceptorCommID != "" {
		sendNotificationToCommunicationUser(friendCommunicationID, "friend_request_accepted", map[string]interface{}{
			"friend_communication_id": acceptorCommID,
			"remark":                  remark,
		})
	}

	return map[string]interface{}{
		"type": "friend_accept_response",
		"data": map[string]interface{}{
			"status":  "success",
			"message": "已接受好友请求",
		},
	}
}

// handleWSFriendReject 处理WebSocket拒绝好友请求
func toBackendWSFriendReject(connectionID, userID string, messageData map[string]interface{}) map[string]interface{} {
	data, ok := messageData["data"].(map[string]interface{})
	if !ok {
		return map[string]interface{}{
			"type": "friend_reject_response",
			"data": map[string]interface{}{
				"status":  "fail",
				"message": "缺少数据字段",
			},
		}
	}

	friendCommunicationID, ok := data["friend_communication_id"].(string)
	if !ok {
		return map[string]interface{}{
			"type": "friend_reject_response",
			"data": map[string]interface{}{
				"status":  "fail",
				"message": "缺少好友Communication ID",
			},
		}
	}

	// 将内部的 userID 转换为 user_communication_id
	userCommunicationID := getCommunicationIDOrFallback(userID)
	if userCommunicationID == userID {
		return map[string]interface{}{
			"type": "friend_reject_response",
			"data": map[string]interface{}{
				"status":  "fail",
				"message": "无法获取用户通信ID",
			},
		}
	}

	if err := relationship.RejectFriend(userCommunicationID, friendCommunicationID); err != nil {
		logger.Printf("[WS:%s] 拒绝好友请求失败: %v", connectionID, err)
		return map[string]interface{}{
			"type": "friend_reject_response",
			"data": map[string]interface{}{
				"status":  "fail",
				"message": err.Error(),
			},
		}
	}

	logger.Printf("[WS:%s] 拒绝好友请求成功: %s <- %s", connectionID, userCommunicationID, friendCommunicationID)

	rejectorCommID := userCommunicationID
	if rejectorCommID != "" {
		sendNotificationToCommunicationUser(friendCommunicationID, "friend_request_rejected", map[string]interface{}{
			"friend_communication_id": rejectorCommID,
		})
	}

	return map[string]interface{}{
		"type": "friend_reject_response",
		"data": map[string]interface{}{
			"status":  "success",
			"message": "已拒绝好友请求",
		},
	}
}

// handleWSFollowAdd 处理WebSocket关注用户请求
func toBackendWSFollowAdd(connectionID, userID string, messageData map[string]interface{}) map[string]interface{} {
	data, ok := messageData["data"].(map[string]interface{})
	if !ok {
		return map[string]interface{}{
			"type": "follow_add_response",
			"data": map[string]interface{}{
				"status":  "fail",
				"message": "缺少数据字段",
			},
		}
	}

	followingCommunicationID, ok := data["following_communication_id"].(string)
	if !ok {
		return map[string]interface{}{
			"type": "follow_add_response",
			"data": map[string]interface{}{
				"status":  "fail",
				"message": "缺少被关注用户Communication ID",
			},
		}
	}

	// 将内部的 userID 转换为 user_communication_id
	userCommunicationID := getCommunicationIDOrFallback(userID)
	if userCommunicationID == userID {
		return map[string]interface{}{
			"type": "follow_add_response",
			"data": map[string]interface{}{
				"status":  "fail",
				"message": "无法获取用户通信ID",
			},
		}
	}

	err := relationship.FollowUser(userCommunicationID, followingCommunicationID)
	if err != nil {
		logger.Printf("[WS:%s] 关注用户失败: %v", connectionID, err)
		return map[string]interface{}{
			"type": "follow_add_response",
			"data": map[string]interface{}{
				"status":  "fail",
				"message": err.Error(),
			},
		}
	}

	logger.Printf("[WS:%s] 关注用户成功: %s -> %s", connectionID, userID, followingCommunicationID)

	followerCommID := getCommunicationIDOrFallback(userID)

	sendNotificationToCommunicationUser(followingCommunicationID, "followed", map[string]interface{}{
		"follower_communication_id":  followerCommID,
		"following_communication_id": followingCommunicationID,
	})

	return map[string]interface{}{
		"type": "follow_add_response",
		"data": map[string]interface{}{
			"status":  "success",
			"message": "关注用户成功",
		},
	}
}

// handleWSFollowRemove 处理WebSocket取消关注请求
func toBackendWSFollowRemove(connectionID, userID string, messageData map[string]interface{}) map[string]interface{} {
	data, ok := messageData["data"].(map[string]interface{})
	if !ok {
		return map[string]interface{}{
			"type": "follow_remove_response",
			"data": map[string]interface{}{
				"status":  "fail",
				"message": "缺少数据字段",
			},
		}
	}

	followingCommunicationID, ok := data["following_communication_id"].(string)
	if !ok {
		return map[string]interface{}{
			"type": "follow_remove_response",
			"data": map[string]interface{}{
				"status":  "fail",
				"message": "缺少被关注用户Communication ID",
			},
		}
	}

	// 将内部的 userID 转换为 user_communication_id
	userCommunicationID := getCommunicationIDOrFallback(userID)
	if userCommunicationID == userID {
		return map[string]interface{}{
			"type": "follow_remove_response",
			"data": map[string]interface{}{
				"status":  "fail",
				"message": "无法获取用户通信ID",
			},
		}
	}

	err := relationship.UnfollowUser(userCommunicationID, followingCommunicationID)
	if err != nil {
		logger.Printf("[WS:%s] 取消关注失败: %v", connectionID, err)
		return map[string]interface{}{
			"type": "follow_remove_response",
			"data": map[string]interface{}{
				"status":  "fail",
				"message": err.Error(),
			},
		}
	}

	logger.Printf("[WS:%s] 取消关注成功: %s -> %s", connectionID, userID, followingCommunicationID)

	unfollowerCommID := getCommunicationIDOrFallback(userID)

	sendNotificationToCommunicationUser(followingCommunicationID, "unfollowed", map[string]interface{}{
		"follower_communication_id":  unfollowerCommID,
		"following_communication_id": followingCommunicationID,
	})

	return map[string]interface{}{
		"type": "follow_remove_response",
		"data": map[string]interface{}{
			"status":  "success",
			"message": "取消关注成功",
		},
	}
}

// handleCommunicationSystemMessage 处理通信系统消息（WebSocket消息转发）
// 遵循三级结构：type="communication_system", operation="chat", action="send_message"
// 注意：后端不保存消息，只负责将A用户的消息推送给B用户
func toBackendCommunicationSystemMessage(conn *websocket.Conn, connectionID, senderUserID string, messageData map[string]interface{}) {
	// 获取第二级：operation
	operationVal, ok := messageData["operation"].(string)
	if !ok {
		logger.Printf("[WS:%s] 通信系统消息缺少operation字段", connectionID)
		toClientChatErrorResponse(conn, "缺少operation字段")
		return
	}

	// 获取第三级：action
	actionVal, ok := messageData["action"].(string)
	if !ok {
		logger.Printf("[WS:%s] 通信系统消息缺少action字段", connectionID)
		toClientChatErrorResponse(conn, "缺少action字段")
		return
	}

	logger.Printf("[WS:%s] 通信系统消息 - operation: %s, action: %s, 发送者: %s", connectionID, operationVal, actionVal, senderUserID)

	// 根据operation和action路由
	if operationVal == "chat" && actionVal == "send_message" {
		toBackendSendChatMessage(conn, connectionID, senderUserID, messageData)
	} else {
		logger.Printf("[WS:%s] 未知的通信系统操作: operation=%s, action=%s", connectionID, operationVal, actionVal)
		toClientChatErrorResponse(conn, fmt.Sprintf("未知的操作: operation=%s, action=%s", operationVal, actionVal))
	}
}

// handleSendChatMessage 处理发送聊天消息请求
func toBackendSendChatMessage(conn *websocket.Conn, connectionID, senderUserID string, messageData map[string]interface{}) {
	data, ok := messageData["data"].(map[string]interface{})
	if !ok {
		toClientChatErrorResponse(conn, "缺少data字段")
		return
	}

	// 获取接收者的communication_id
	recipientCommunicationID, ok := data["recipient_communication_id"].(string)
	if !ok || recipientCommunicationID == "" {
		toClientChatErrorResponse(conn, "缺少recipient_communication_id")
		return
	}

	// 获取消息内容
	content, ok := data["content"].(string)
	if !ok || content == "" {
		toClientChatErrorResponse(conn, "缺少消息内容")
		return
	}

	// 获取时间戳（如果客户端提供）
	timestamp, ok := data["timestamp"].(string)
	if !ok || timestamp == "" {
		timestamp = time.Now().UTC().Format(time.RFC3339)
	}

	logger.Printf("[WS:%s] 转发消息: %s -> %s, 内容: %s", connectionID, senderUserID, recipientCommunicationID, content)

	senderCommunicationID := getCommunicationIDOrFallback(senderUserID)

	notificationData := map[string]interface{}{
		"sender_communication_id": senderCommunicationID,
		"content":                 content,
		"timestamp":               timestamp,
	}

	// 使用统一通知服务推送消息给接收者
	// 如果接收者在线，实时推送；如果离线，保存到数据库
	isOnline := sendNotificationToCommunicationUser(recipientCommunicationID, "chat_message_received", notificationData)

	// 给发送者返回成功确认
	response := map[string]interface{}{
		"type": "chat_message_sent",
		"data": map[string]interface{}{
			"status":                     "success",
			"recipient_communication_id": recipientCommunicationID,
			"timestamp":                  timestamp,
			"delivered":                  isOnline, // 是否已送达（在线用户）
		},
	}

	responseBytes, err := json.Marshal(response)
	if err != nil {
		logger.Printf("[WS:%s] 序列化响应失败: %v", connectionID, err)
		return
	}

	if err := conn.WriteMessage(websocket.TextMessage, responseBytes); err != nil {
		logger.Printf("[WS:%s] 发送响应失败: %v", connectionID, err)
	}

	if isOnline {
		logger.Printf("[WS:%s] 消息已实时推送给在线用户: %s", connectionID, recipientCommunicationID)
	} else {
		logger.Printf("[WS:%s] 接收者离线，消息已保存: %s", connectionID, recipientCommunicationID)
	}
}

// toClientChatErrorResponse 发送聊天错误响应给客户端
func toClientChatErrorResponse(conn *websocket.Conn, message string) {
	errorResponse := map[string]interface{}{
		"type": "chat_error",
		"data": map[string]interface{}{
			"status":  "fail",
			"message": message,
		},
	}
	responseBytes, _ := json.Marshal(errorResponse)
	conn.WriteMessage(websocket.TextMessage, responseBytes)
}

// ================================
// 辅助方法
// ================================

func getCommunicationIDOrFallback(userID string) string {
	commID, err := relationship.GetCommunicationIDByUserID(userID)
	if err != nil || commID == "" {
		if err != nil {
			logger.Printf("[WS] 获取用户 %s 的通信ID失败，回退 user_id: %v", userID, err)
		}
		return userID
	}
	return commID
}

func sendErrorResponse(conn *websocket.Conn, message string) {
	errorResponse := map[string]interface{}{
		"type": "relationship_error",
		"data": map[string]interface{}{
			"status":  "fail",
			"message": message,
		},
	}
	responseBytes, _ := json.Marshal(errorResponse)
	conn.WriteMessage(websocket.TextMessage, responseBytes)
}

func sendNotificationToCommunicationUser(targetID, notificationType string, payload map[string]interface{}) bool {
	if targetID == "" {
		return false
	}
	if userID, err := relationship.GetUserIDByCommunicationID(targetID); err == nil {
		return sendNotification(userID, notificationType, payload)
	}
	if looksLikeInternalUserID(targetID) {
		logger.Printf("[通知] 目标ID %s 不是通信ID，按内部 user_id 处理。请尽快调整调用方。", targetID)
		return sendNotification(targetID, notificationType, payload)
	}
	logger.Printf("[通知] 通信ID转内部ID失败 (%s): 未找到对应用户", targetID)
	return false
}

func looksLikeInternalUserID(identifier string) bool {
	return len(identifier) > 0 && len(identifier) <= 12 && !strings.Contains(identifier, "-")
}

func sendNotification(userID, notificationType string, payload map[string]interface{}) bool {
	if payload == nil {
		payload = make(map[string]interface{})
	}
	return SendNotification(userID, notificationType, payload)
}

