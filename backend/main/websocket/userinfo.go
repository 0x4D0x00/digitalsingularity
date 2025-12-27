package websocket

import "time"

// BroadcastUserInfoUpdate 向指定用户及其联系人广播资料变更
func BroadcastUserInfoUpdate(userID, notificationType string, contactIDs []string, info map[string]interface{}) {
	if userID == "" || notificationType == "" {
		return
	}

	payload := make(map[string]interface{}, len(info)+2)
	for k, v := range info {
		payload[k] = v
	}
	payload["user_id"] = userID
	payload["updated_at"] = time.Now().UTC().Format(time.RFC3339)

	// 去重并过滤非法联系人
	unique := make(map[string]struct{}, len(contactIDs))
	validContacts := make([]string, 0, len(contactIDs))
	for _, id := range contactIDs {
		if id == "" || id == userID {
			continue
		}
		if _, exists := unique[id]; exists {
			continue
		}
		unique[id] = struct{}{}
		validContacts = append(validContacts, id)
	}

	// 1. 先在本地服务器尝试推送（可能用户连接到本服务器）
	SendNotification(userID, notificationType, payload)
	if len(validContacts) > 0 {
		SendNotificationToMultipleUsers(validContacts, notificationType, payload)
	}

	// 2. 发布到 Redis Pub/Sub，让其他服务器也尝试推送
	// 如果用户连接到其他服务器，那边会收到并推送
	// 如果用户真的离线，所有服务器都会保存离线通知（但数据库会去重）
	PublishClusterNotification(userID, notificationType, validContacts, payload)
}
