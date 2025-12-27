package websocket

import "time"

// ForceLogout 向用户推送强制下线通知并断开所有活跃连接
func ForceLogout(userID, reason string, extra map[string]interface{}) {
	if userID == "" {
		return
	}

	payload := make(map[string]interface{}, len(extra)+2)
	payload["forced_at"] = time.Now().UTC().Format(time.RFC3339)
	if reason != "" {
		payload["reason"] = reason
	}
	for k, v := range extra {
		payload[k] = v
	}

	SendNotification(userID, "force_logout", payload)
	disconnectUserConnections(userID)
}

func disconnectUserConnections(userID string) {
	onlineMutex.Lock()
	conns, exists := userConnections[userID]
	if !exists || len(conns) == 0 {
		onlineMutex.Unlock()
		return
	}

	delete(userConnections, userID)
	for conn := range conns {
		delete(connectionUsers, conn)
	}
	onlineMutex.Unlock()

	for conn := range conns {
		_ = conn.Close()
	}
}
