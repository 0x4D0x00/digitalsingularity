package database

import (
	"fmt"
	"strings"
	"time"
	"unicode"


	"digitalsingularity/backend/common/security/symmetricencryption/encrypt"
	"digitalsingularity/backend/common/utils/datahandle"
)

// Friend 好友关系结构
type Friend struct {
	UserCommunicationID   string     `json:"user_communication_id"`
	FriendCommunicationID string     `json:"friend_communication_id"`
	Remark                string     `json:"remark"`
	Status                string     `json:"status"`
	CreatedAt             time.Time  `json:"created_at"`
	UpdatedAt             time.Time  `json:"updated_at"`
	DeletedAt             *time.Time `json:"deleted_at,omitempty"`
}

// Follow 关注关系结构
type Follow struct {
	FollowerCommunicationID  string     `json:"follower_communication_id"`
	FollowingCommunicationID string     `json:"following_communication_id"`
	CreatedAt                time.Time  `json:"created_at"`
	UpdatedAt                time.Time  `json:"updated_at"`
	DeletedAt                *time.Time `json:"deleted_at,omitempty"`
}

// FollowStats 关注统计结构
type FollowStats struct {
	FollowerCount     int `json:"follower_count"`
	FollowingCount    int `json:"following_count"`
	MutualFollowCount int `json:"mutual_follow_count"`
}

// User 用户基本信息结构（用于返回用户列表）
type User struct {
	UserCommunicationID string `json:"user_communication_id"`
	Nickname            string `json:"nickname"`
	AvatarURL           string `json:"avatar_url"`
}

// FriendWithUserInfo 好友关系及用户详细信息结构
type FriendWithUserInfo struct {
	UserCommunicationID   string     `json:"user_communication_id"`
	FriendCommunicationID string     `json:"friend_communication_id"`
	Remark                string     `json:"remark"`
	Status                string     `json:"status"`
	CreatedAt             time.Time  `json:"created_at"`
	UpdatedAt             time.Time  `json:"updated_at"`
	DeletedAt             *time.Time `json:"deleted_at,omitempty"`
	// 好友的详细信息
	UserName                 string     `json:"user_name"`
	Phone                    string     `json:"phone"`
	Email                    string     `json:"email"`
	Nickname                 string     `json:"nickname"`
	AvatarURL                string     `json:"avatar_url"`
	Bio                      string     `json:"bio"`
	UserCreatedAt            time.Time  `json:"user_created_at"`
	UserUpdatedAt            time.Time  `json:"user_updated_at"`
	LastLoginAt              *time.Time `json:"last_login_at,omitempty"`
	Gender                   string     `json:"gender"`
	BirthDate                *time.Time `json:"birth_date,omitempty"`
	PersonalAssistantEnabled int        `json:"personal_assistant_enabled"`
}

// getService 获取数据库服务实例
func getService() (*datahandle.CommonReadWriteService, error) {
	return datahandle.NewCommonReadWriteService("communication_system")
}

// GetServiceForRelationship 暴露给上层的服务获取方法
func GetServiceForRelationship() (*datahandle.CommonReadWriteService, error) {
	return getService()
}

// GetUserIDByCommunicationID 根据通信ID查询内部 user_id
// 注意：此函数主要用于 websocket 通知等内部场景，业务逻辑应使用 communication_id
func GetUserIDByCommunicationID(service *datahandle.CommonReadWriteService, userCommunicationID string) (string, error) {
	if userCommunicationID == "" {
		return "", fmt.Errorf("通信ID不能为空")
	}

	query := `SELECT user_id FROM communication_system_users WHERE user_communication_id = ? LIMIT 1`
	result := service.QueryDb(query, userCommunicationID)
	if !result.IsSuccess() {
		return "", fmt.Errorf("查询用户ID失败: %v", result.Error)
	}

	rows, ok := result.Data.([]map[string]interface{})
	if !ok || len(rows) == 0 {
		return "", fmt.Errorf("通信ID不存在")
	}

	if userID, ok := rows[0]["user_id"].(string); ok && userID != "" {
		return userID, nil
	}

	return "", fmt.Errorf("通信ID不存在")
}

// GetCommunicationIDByUserID 根据内部 user_id 查询通信ID
func GetCommunicationIDByUserID(service *datahandle.CommonReadWriteService, userID string) (string, error) {
	if userID == "" {
		return "", fmt.Errorf("用户ID不能为空")
	}

	query := `SELECT user_communication_id FROM communication_system_users WHERE user_id = ? LIMIT 1`
	result := service.QueryDb(query, userID)
	if !result.IsSuccess() {
		return "", fmt.Errorf("查询通信ID失败: %v", result.Error)
	}

	rows, ok := result.Data.([]map[string]interface{})
	if !ok || len(rows) == 0 {
		return "", fmt.Errorf("用户ID不存在")
	}

	if userCommunicationID, ok := rows[0]["user_communication_id"].(string); ok && userCommunicationID != "" {
		return userCommunicationID, nil
	}

	return "", fmt.Errorf("用户ID不存在")
}

// ensureCommunicationUserExists 确认目标通信 ID 存在
func ensureCommunicationUserExists(service *datahandle.CommonReadWriteService, userCommunicationID string) error {
	query := `SELECT 1 FROM communication_system_users WHERE user_communication_id = ? LIMIT 1`
	result := service.QueryDb(query, userCommunicationID)
	if !result.IsSuccess() {
		return fmt.Errorf("查询通信系统用户失败: %v", result.Error)
	}

	rows, ok := result.Data.([]map[string]interface{})
	if !ok || len(rows) == 0 {
		return fmt.Errorf("目标用户不存在或未开通通信系统")
	}

	return nil
}

// AddFriend 发送好友请求（若对方已发起请求则直接通过）
func AddFriend(userCommunicationID, friendCommunicationID, remark string) error {
	service, err := getService()
	if err != nil {
		return fmt.Errorf("获取数据库服务失败: %v", err)
	}

	if userCommunicationID == friendCommunicationID {
		return fmt.Errorf("不能添加自己为好友")
	}

	if err := ensureCommunicationUserExists(service, friendCommunicationID); err != nil {
		return err
	}

	// 如果对方已对我发起请求，则直接接受
	incomingQuery := `SELECT status FROM friends WHERE user_communication_id = ? AND friend_communication_id = ? AND deleted_at IS NULL`
	incomingResult := service.QueryDb(incomingQuery, friendCommunicationID, userCommunicationID)
	if incomingResult.IsSuccess() {
		if rows, ok := incomingResult.Data.([]map[string]interface{}); ok && len(rows) > 0 {
			if status, ok := rows[0]["status"].(string); ok {
				switch status {
				case "pending":
					return AcceptFriend(userCommunicationID, friendCommunicationID, remark)
				case "accepted":
					return fmt.Errorf("已经是好友关系")
				}
			}
		}
	}

	// 检查自己是否已经发过请求
	checkQuery := `SELECT status, deleted_at FROM friends WHERE user_communication_id = ? AND friend_communication_id = ? LIMIT 1`
	checkResult := service.QueryDb(checkQuery, userCommunicationID, friendCommunicationID)
	if checkResult.IsSuccess() {
		if rows, ok := checkResult.Data.([]map[string]interface{}); ok && len(rows) > 0 {
			status, _ := rows[0]["status"].(string)
			deletedAt := rows[0]["deleted_at"]

			if status == "accepted" && deletedAt == nil {
				return fmt.Errorf("已经是好友关系")
			}
			if status == "pending" && deletedAt == nil {
				return fmt.Errorf("已发送好友请求，请等待对方处理")
			}
		}
	}

	// 创建或更新好友请求
	requestQuery := `INSERT INTO friends (user_communication_id, friend_communication_id, remark, status) 
		VALUES (?, ?, ?, 'pending')
		ON DUPLICATE KEY UPDATE 
			remark = VALUES(remark),
			status = 'pending',
			deleted_at = NULL,
			updated_at = NOW()`
	result := service.ExecuteDb(requestQuery, userCommunicationID, friendCommunicationID, remark)
	if !result.IsSuccess() {
		return fmt.Errorf("发送好友请求失败: %v", result.Error)
	}

	return nil
}

// RemoveFriend 删除好友
func RemoveFriend(userCommunicationID, friendCommunicationID string) error {
	service, err := getService()
	if err != nil {
		return fmt.Errorf("获取数据库服务失败: %v", err)
	}

	// 软删除好友关系（双向）
	now := time.Now()
	query1 := `UPDATE friends SET deleted_at = ? WHERE user_communication_id = ? AND friend_communication_id = ? AND deleted_at IS NULL`
	result1 := service.ExecuteDb(query1, now, userCommunicationID, friendCommunicationID)
	if !result1.IsSuccess() {
		return fmt.Errorf("删除好友失败: %v", result1.Error)
	}

	query2 := `UPDATE friends SET deleted_at = ? WHERE user_communication_id = ? AND friend_communication_id = ? AND deleted_at IS NULL`
	result2 := service.ExecuteDb(query2, now, friendCommunicationID, userCommunicationID)
	if !result2.IsSuccess() {
		return fmt.Errorf("删除好友失败: %v", result2.Error)
	}

	// 更新统计缓存
	updateFriendStats(userCommunicationID)
	updateFriendStats(friendCommunicationID)

	return nil
}

// AcceptFriend 接受好友请求
func AcceptFriend(userCommunicationID, requesterID, remark string) error {
	service, err := getService()
	if err != nil {
		return fmt.Errorf("获取数据库服务失败: %v", err)
	}
	if err != nil {
		return fmt.Errorf("获取通信系统用户ID失败: %v", err)
	}

	if userCommunicationID == requesterID {
		return fmt.Errorf("不能接受自己的好友请求")
	}

	if err := ensureCommunicationUserExists(service, requesterID); err != nil {
		return err
	}

	// 确认存在待处理请求
	pendingQuery := `SELECT status FROM friends WHERE user_communication_id = ? AND friend_communication_id = ? AND status = 'pending' AND deleted_at IS NULL LIMIT 1`
	pendingResult := service.QueryDb(pendingQuery, requesterID, userCommunicationID)
	if !pendingResult.IsSuccess() {
		return fmt.Errorf("查询好友请求失败: %v", pendingResult.Error)
	}
	rows, ok := pendingResult.Data.([]map[string]interface{})
	if !ok || len(rows) == 0 {
		return fmt.Errorf("未找到对应的好友请求")
	}

	// 更新请求状态为已接受
	updateQuery := `UPDATE friends SET status = 'accepted', deleted_at = NULL, updated_at = NOW() 
		WHERE user_communication_id = ? AND friend_communication_id = ? AND status = 'pending' AND deleted_at IS NULL`
	updateResult := service.ExecuteDb(updateQuery, requesterID, userCommunicationID)
	if !updateResult.IsSuccess() {
		return fmt.Errorf("更新好友请求状态失败: %v", updateResult.Error)
	}

	// 为当前用户创建/更新好友记录
	createQuery := `INSERT INTO friends (user_communication_id, friend_communication_id, remark, status) 
		VALUES (?, ?, ?, 'accepted')
		ON DUPLICATE KEY UPDATE 
			status = 'accepted',
			remark = CASE WHEN VALUES(remark) = '' THEN remark ELSE VALUES(remark) END,
			deleted_at = NULL,
			updated_at = NOW()`
	createResult := service.ExecuteDb(createQuery, userCommunicationID, requesterID, remark)
	if !createResult.IsSuccess() {
		return fmt.Errorf("创建好友关系失败: %v", createResult.Error)
	}

	// 更新统计
	updateFriendStats(userCommunicationID)
	updateFriendStats(requesterID)

	return nil
}

// RejectFriend 拒绝好友请求
func RejectFriend(userCommunicationID, requesterID string) error {
	service, err := getService()
	if err != nil {
		return fmt.Errorf("获取数据库服务失败: %v", err)
	}

	rejectQuery := `UPDATE friends SET status = 'rejected', deleted_at = NOW(), updated_at = NOW()
		WHERE user_communication_id = ? AND friend_communication_id = ? AND status = 'pending' AND deleted_at IS NULL`
	result := service.ExecuteDb(rejectQuery, requesterID, userCommunicationID)
	if !result.IsSuccess() {
		return fmt.Errorf("拒绝好友请求失败: %v", result.Error)
	}

	return nil
}

// UpdateFriend 更新好友备注
func UpdateFriend(userCommunicationID, friendCommunicationID, remark string) error {
	service, err := getService()
	if err != nil {
		return fmt.Errorf("获取数据库服务失败: %v", err)
	}

	query := `UPDATE friends SET remark = ?, updated_at = NOW() WHERE user_communication_id = ? AND friend_communication_id = ? AND deleted_at IS NULL`
	result := service.ExecuteDb(query, remark, userCommunicationID, friendCommunicationID)
	if !result.IsSuccess() {
		return fmt.Errorf("更新好友备注失败: %v", result.Error)
	}

	return nil
}

// CheckFriend 检查是否为好友
func CheckFriend(userCommunicationID, friendCommunicationID string) (bool, error) {
	service, err := getService()
	if err != nil {
		return false, fmt.Errorf("获取数据库服务失败: %v", err)
	}

	query := `SELECT COUNT(*) as count FROM friends WHERE user_communication_id = ? AND friend_communication_id = ? AND deleted_at IS NULL`
	result := service.QueryDb(query, userCommunicationID, friendCommunicationID)
	if !result.IsSuccess() {
		return false, fmt.Errorf("检查好友关系失败: %v", result.Error)
	}

	rows, ok := result.Data.([]map[string]interface{})
	if !ok || len(rows) == 0 {
		return false, nil
	}

	count, ok := rows[0]["count"].(int64)
	if !ok {
		// 尝试转换为其他数字类型
		if countFloat, ok := rows[0]["count"].(float64); ok {
			count = int64(countFloat)
		} else {
			return false, nil
		}
	}

	return count > 0, nil
}

// GetFriendRequests 获取收到的好友请求列表
func GetFriendRequests(userCommunicationID string) ([]FriendWithUserInfo, error) {
	service, err := getService()
	if err != nil {
		return nil, fmt.Errorf("获取数据库服务失败: %v", err)
	}

	query := `SELECT 
			f.user_communication_id, f.friend_communication_id, f.remark, f.status, 
			f.created_at, f.updated_at, f.deleted_at,
			u.user_name, u.phone, u.email, u.nickname, u.avatar_url, u.bio,
			u.created_at as user_created_at, u.updated_at as user_updated_at, 
			u.last_login_at, u.gender, u.birth_date,
			COALESCE(up.preference_value, '0') as personal_assistant_enabled
		FROM friends f 
		INNER JOIN communication_system_users csu ON f.user_communication_id = csu.user_communication_id
		INNER JOIN common.users u ON csu.user_id = u.user_id
		LEFT JOIN common.user_preferences up ON u.user_id = up.user_id AND up.preference_key = 'personal_assistant_enabled'
		WHERE f.friend_communication_id = ? AND f.status = 'pending' AND f.deleted_at IS NULL
		ORDER BY f.created_at DESC`

	result := service.QueryDb(query, userCommunicationID)
	if !result.IsSuccess() {
		return nil, fmt.Errorf("获取好友请求失败: %v", result.Error)
	}

	rows, ok := result.Data.([]map[string]interface{})
	if !ok {
		return []FriendWithUserInfo{}, nil
	}

	friendList := mapRowsToFriendList(rows)
	
	// 交换 user_communication_id 和 friend_communication_id
	// 让前端始终理解为：user_communication_id = 自己，friend_communication_id = 对方（请求发送者）
	for i := range friendList {
		temp := friendList[i].UserCommunicationID
		friendList[i].UserCommunicationID = friendList[i].FriendCommunicationID
		friendList[i].FriendCommunicationID = temp
	}

	return friendList, nil
}

// GetFriends 获取好友列表（包含好友详细信息）
func GetFriends(userCommunicationID string, status string) ([]FriendWithUserInfo, error) {
	service, err := getService()
	if err != nil {
		return nil, fmt.Errorf("获取数据库服务失败: %v", err)
	}

	var query string
	var params []interface{}

	if status != "" {
		query = `SELECT 
			f.user_communication_id, f.friend_communication_id, f.remark, f.status, 
			f.created_at, f.updated_at, f.deleted_at,
			u.user_name, u.phone, u.email, u.nickname, u.avatar_url, u.bio,
			u.created_at as user_created_at, u.updated_at as user_updated_at, 
			u.last_login_at, u.gender, u.birth_date,
			COALESCE(up.preference_value, '0') as personal_assistant_enabled
			FROM friends f 
			INNER JOIN communication_system_users csu ON f.friend_communication_id = csu.user_communication_id
			INNER JOIN common.users u ON csu.user_id = u.user_id
			LEFT JOIN common.user_preferences up ON u.user_id = up.user_id AND up.preference_key = 'personal_assistant_enabled'
			WHERE f.user_communication_id = ? AND f.status = ? AND f.deleted_at IS NULL
			ORDER BY f.created_at DESC`
		params = []interface{}{userCommunicationID, status}
	} else {
		query = `SELECT 
			f.user_communication_id, f.friend_communication_id, f.remark, f.status, 
			f.created_at, f.updated_at, f.deleted_at,
			u.user_name, u.phone, u.email, u.nickname, u.avatar_url, u.bio,
			u.created_at as user_created_at, u.updated_at as user_updated_at, 
			u.last_login_at, u.gender, u.birth_date,
			COALESCE(up.preference_value, '0') as personal_assistant_enabled
			FROM friends f 
			INNER JOIN communication_system_users csu ON f.friend_communication_id = csu.user_communication_id
			INNER JOIN common.users u ON csu.user_id = u.user_id
			LEFT JOIN common.user_preferences up ON u.user_id = up.user_id AND up.preference_key = 'personal_assistant_enabled'
			WHERE f.user_communication_id = ? AND f.deleted_at IS NULL
			ORDER BY f.created_at DESC`
		params = []interface{}{userCommunicationID}
	}

	result := service.QueryDb(query, params...)
	if !result.IsSuccess() {
		return nil, fmt.Errorf("获取好友列表失败: %v", result.Error)
	}

	rows, ok := result.Data.([]map[string]interface{})
	if !ok {
		return []FriendWithUserInfo{}, nil
	}

	return mapRowsToFriendList(rows), nil
}

func mapRowsToFriendList(rows []map[string]interface{}) []FriendWithUserInfo {
	friends := make([]FriendWithUserInfo, 0, len(rows))
	for _, row := range rows {
		friend := FriendWithUserInfo{}

		if userCommunicationID, ok := row["user_communication_id"].(string); ok {
			friend.UserCommunicationID = userCommunicationID
		}
		if friendCommunicationID, ok := row["friend_communication_id"].(string); ok {
			friend.FriendCommunicationID = friendCommunicationID
		}
		if remark, ok := row["remark"].(string); ok {
			friend.Remark = remark
		}
		if status, ok := row["status"].(string); ok {
			friend.Status = status
		}
		if createdAt, ok := row["created_at"].(time.Time); ok {
			friend.CreatedAt = createdAt
		} else if createdAtStr, ok := row["created_at"].(string); ok {
			if t, err := time.Parse("2006-01-02 15:04:05", createdAtStr); err == nil {
				friend.CreatedAt = t
			}
		}
		if updatedAt, ok := row["updated_at"].(time.Time); ok {
			friend.UpdatedAt = updatedAt
		} else if updatedAtStr, ok := row["updated_at"].(string); ok {
			if t, err := time.Parse("2006-01-02 15:04:05", updatedAtStr); err == nil {
				friend.UpdatedAt = t
			}
		}
		if deletedAt, ok := row["deleted_at"].(time.Time); ok {
			friend.DeletedAt = &deletedAt
		} else if deletedAtStr, ok := row["deleted_at"].(string); ok {
			if t, err := time.Parse("2006-01-02 15:04:05", deletedAtStr); err == nil {
				friend.DeletedAt = &t
			}
		}

		if userName, ok := row["user_name"].(string); ok {
			friend.UserName = userName
		}
		if phone, ok := row["phone"].(string); ok {
			friend.Phone = phone
		}
		if email, ok := row["email"].(string); ok {
			friend.Email = email
		}
		if nickname, ok := row["nickname"].(string); ok {
			friend.Nickname = nickname
		}
		if avatarURL, ok := row["avatar_url"].(string); ok {
			friend.AvatarURL = avatarURL
		}
		if bio, ok := row["bio"].(string); ok {
			friend.Bio = bio
		}
		if userCreatedAt, ok := row["user_created_at"].(time.Time); ok {
			friend.UserCreatedAt = userCreatedAt
		} else if userCreatedAtStr, ok := row["user_created_at"].(string); ok {
			if t, err := time.Parse("2006-01-02 15:04:05", userCreatedAtStr); err == nil {
				friend.UserCreatedAt = t
			}
		}
		if userUpdatedAt, ok := row["user_updated_at"].(time.Time); ok {
			friend.UserUpdatedAt = userUpdatedAt
		} else if userUpdatedAtStr, ok := row["user_updated_at"].(string); ok {
			if t, err := time.Parse("2006-01-02 15:04:05", userUpdatedAtStr); err == nil {
				friend.UserUpdatedAt = t
			}
		}
		if lastLoginAt, ok := row["last_login_at"].(time.Time); ok {
			friend.LastLoginAt = &lastLoginAt
		} else if lastLoginAtStr, ok := row["last_login_at"].(string); ok && lastLoginAtStr != "" {
			if t, err := time.Parse("2006-01-02 15:04:05", lastLoginAtStr); err == nil {
				friend.LastLoginAt = &t
			}
		}
		if gender, ok := row["gender"].(string); ok {
			friend.Gender = gender
		}
		if birthDate, ok := row["birth_date"].(time.Time); ok {
			friend.BirthDate = &birthDate
		} else if birthDateStr, ok := row["birth_date"].(string); ok && birthDateStr != "" {
			if t, err := time.Parse("2006-01-02", birthDateStr); err == nil {
				friend.BirthDate = &t
			}
		}

		paValue := row["personal_assistant_enabled"]
		switch v := paValue.(type) {
		case string:
			if v == "1" {
				friend.PersonalAssistantEnabled = 1
			}
		case []byte:
			if string(v) == "1" {
				friend.PersonalAssistantEnabled = 1
			}
		case int64:
			friend.PersonalAssistantEnabled = int(v)
		case int:
			friend.PersonalAssistantEnabled = v
		case float64:
			friend.PersonalAssistantEnabled = int(v)
		}

		friends = append(friends, friend)
	}

	return friends
}

// SearchUser 搜索用户（按昵称或用户名）
func SearchUser(userCommunicationID, keyword string) ([]FriendWithUserInfo, error) {
	service, err := getService()
	if err != nil {
		return nil, fmt.Errorf("获取数据库服务失败: %v", err)
	}

	keyword = strings.TrimSpace(keyword)
	searchKeyword := "%" + keyword + "%"

	var encryptedPhone string
	if is11DigitNumber(keyword) {
		encryptedPhone, err = encrypt.SymmetricEncryptService(keyword, "", "")
		if err != nil {
			fmt.Printf("[WARN] 手机号加密失败: %v\n", err)
			encryptedPhone = ""
		}
	}

	// 搜索用户，返回用户信息（包括是否为好友）
	query := `SELECT 
		csu.user_communication_id,
		COALESCE(f.friend_communication_id, csu.user_communication_id) as friend_communication_id,
		COALESCE(f.remark, '') as remark,
		COALESCE(f.status, 'not_friend') as status,
		COALESCE(f.created_at, NOW()) as created_at,
		COALESCE(f.updated_at, NOW()) as updated_at,
		f.deleted_at,
		u.user_name,
		u.phone,
		u.email,
		u.nickname,
		u.avatar_url,
		u.bio,
		u.created_at as user_created_at,
		u.updated_at as user_updated_at,
		u.last_login_at,
		u.gender,
		u.birth_date,
		COALESCE(up.preference_value, '0') as personal_assistant_enabled
		FROM communication_system_users csu
		INNER JOIN common.users u ON csu.user_id = u.user_id
		LEFT JOIN friends f ON csu.user_communication_id = f.friend_communication_id 
			AND f.user_communication_id = ?
			AND f.deleted_at IS NULL
		LEFT JOIN common.user_preferences up ON u.user_id = up.user_id AND up.preference_key = 'personal_assistant_enabled'
		WHERE csu.user_communication_id != ? 
		AND (u.nickname LIKE ? OR u.user_name LIKE ? OR u.email LIKE ?`
	if encryptedPhone != "" {
		query += ` OR u.phone = ?`
	}
	query += `)
		LIMIT 20`

	params := []interface{}{userCommunicationID, userCommunicationID, searchKeyword, searchKeyword, searchKeyword}
	if encryptedPhone != "" {
		params = append(params, encryptedPhone)
	}

	result := service.QueryDb(query, params...)
	if !result.IsSuccess() {
		return nil, fmt.Errorf("搜索用户失败: %v", result.Error)
	}

	rows, ok := result.Data.([]map[string]interface{})
	if !ok {
		return []FriendWithUserInfo{}, nil
	}

	users := make([]FriendWithUserInfo, 0, len(rows))
	for _, row := range rows {
		user := FriendWithUserInfo{}

		if userCommunicationID, ok := row["user_communication_id"].(string); ok {
			user.UserCommunicationID = userCommunicationID
		}
		if friendCommunicationID, ok := row["friend_communication_id"].(string); ok {
			user.FriendCommunicationID = friendCommunicationID
		}
		if remark, ok := row["remark"].(string); ok {
			user.Remark = remark
		}
		if status, ok := row["status"].(string); ok {
			user.Status = status
		}
		if createdAt, ok := row["created_at"].(time.Time); ok {
			user.CreatedAt = createdAt
		} else if createdAtStr, ok := row["created_at"].(string); ok {
			if t, err := time.Parse("2006-01-02 15:04:05", createdAtStr); err == nil {
				user.CreatedAt = t
			}
		}
		if updatedAt, ok := row["updated_at"].(time.Time); ok {
			user.UpdatedAt = updatedAt
		} else if updatedAtStr, ok := row["updated_at"].(string); ok {
			if t, err := time.Parse("2006-01-02 15:04:05", updatedAtStr); err == nil {
				user.UpdatedAt = t
			}
		}
		if deletedAt, ok := row["deleted_at"].(time.Time); ok {
			user.DeletedAt = &deletedAt
		} else if deletedAtStr, ok := row["deleted_at"].(string); ok && deletedAtStr != "" {
			if t, err := time.Parse("2006-01-02 15:04:05", deletedAtStr); err == nil {
				user.DeletedAt = &t
			}
		}

		// 用户详细信息字段
		if userName, ok := row["user_name"].(string); ok {
			user.UserName = userName
		}
		if phone, ok := row["phone"].(string); ok {
			user.Phone = phone
		}
		if email, ok := row["email"].(string); ok {
			user.Email = email
		}
		if nickname, ok := row["nickname"].(string); ok {
			user.Nickname = nickname
		}
		if avatarURL, ok := row["avatar_url"].(string); ok {
			user.AvatarURL = avatarURL
		}
		if bio, ok := row["bio"].(string); ok {
			user.Bio = bio
		}
		if userCreatedAt, ok := row["user_created_at"].(time.Time); ok {
			user.UserCreatedAt = userCreatedAt
		} else if userCreatedAtStr, ok := row["user_created_at"].(string); ok {
			if t, err := time.Parse("2006-01-02 15:04:05", userCreatedAtStr); err == nil {
				user.UserCreatedAt = t
			}
		}
		if userUpdatedAt, ok := row["user_updated_at"].(time.Time); ok {
			user.UserUpdatedAt = userUpdatedAt
		} else if userUpdatedAtStr, ok := row["user_updated_at"].(string); ok {
			if t, err := time.Parse("2006-01-02 15:04:05", userUpdatedAtStr); err == nil {
				user.UserUpdatedAt = t
			}
		}
		if lastLoginAt, ok := row["last_login_at"].(time.Time); ok {
			user.LastLoginAt = &lastLoginAt
		} else if lastLoginAtStr, ok := row["last_login_at"].(string); ok && lastLoginAtStr != "" {
			if t, err := time.Parse("2006-01-02 15:04:05", lastLoginAtStr); err == nil {
				user.LastLoginAt = &t
			}
		}
		if gender, ok := row["gender"].(string); ok {
			user.Gender = gender
		}
		if birthDate, ok := row["birth_date"].(time.Time); ok {
			user.BirthDate = &birthDate
		} else if birthDateStr, ok := row["birth_date"].(string); ok && birthDateStr != "" {
			if t, err := time.Parse("2006-01-02", birthDateStr); err == nil {
				user.BirthDate = &t
			}
		}
		// 私人助理功能开关
		paValue := row["personal_assistant_enabled"]
		if paValue != nil {
			switch v := paValue.(type) {
			case string:
				if v == "1" {
					user.PersonalAssistantEnabled = 1
				} else {
					user.PersonalAssistantEnabled = 0
				}
			case []byte:
				if string(v) == "1" {
					user.PersonalAssistantEnabled = 1
				} else {
					user.PersonalAssistantEnabled = 0
				}
			case int64:
				user.PersonalAssistantEnabled = int(v)
			case int:
				user.PersonalAssistantEnabled = v
			case float64:
				user.PersonalAssistantEnabled = int(v)
			default:
				user.PersonalAssistantEnabled = 0
			}
		} else {
			user.PersonalAssistantEnabled = 0
		}

		users = append(users, user)
	}

	return users, nil
}

// is11DigitNumber 判断字符串是否为 11 位纯数字
func is11DigitNumber(value string) bool {
	if len(value) != 11 {
		return false
	}

	for _, r := range value {
		if !unicode.IsDigit(r) {
			return false
		}
	}

	return true
}

// ImportFriends 批量导入好友
func ImportFriends(userCommunicationID string, friendCommunicationIDs []string) error {
	for _, friendCommunicationID := range friendCommunicationIDs {
		if friendCommunicationID == userCommunicationID {
			continue // 跳过自己
		}

		// 检查是否已经是好友
		exists, err := CheckFriend(userCommunicationID, friendCommunicationID)
		if err != nil {
			continue // 跳过错误
		}
		if exists {
			continue // 跳过已存在的好友
		}

		// 添加好友（不设置备注）
		if err := AddFriend(userCommunicationID, friendCommunicationID, ""); err != nil {
			// 记录错误但继续处理其他好友
			continue
		}
	}

	// 更新统计缓存
	updateFriendStats(userCommunicationID)

	return nil
}

// FollowUser 关注用户
func FollowUser(followerCommunicationID, followingCommunicationID string) error {
	service, err := getService()
	if err != nil {
		return fmt.Errorf("获取数据库服务失败: %v", err)
	}

	if followerCommunicationID == followingCommunicationID {
		return fmt.Errorf("不能关注自己")
	}

	// 检查是否已经关注
	exists, err := CheckFollow(followerCommunicationID, followingCommunicationID)
	if err != nil {
		return fmt.Errorf("检查关注关系失败: %v", err)
	}
	if exists {
		return fmt.Errorf("已经关注该用户")
	}

	// 插入关注关系
	query := `INSERT INTO follows (follower_communication_id, following_communication_id) VALUES (?, ?)`
	result := service.ExecuteDb(query, followerCommunicationID, followingCommunicationID)
	if !result.IsSuccess() {
		return fmt.Errorf("关注用户失败: %v", result.Error)
	}

	// 更新统计缓存
	updateFollowStats(followerCommunicationID)
	updateFollowStats(followingCommunicationID)

	return nil
}

// UnfollowUser 取消关注
func UnfollowUser(followerCommunicationID, followingCommunicationID string) error {
	service, err := getService()
	if err != nil {
		return fmt.Errorf("获取数据库服务失败: %v", err)
	}

	// 软删除关注关系
	now := time.Now()
	query := `UPDATE follows SET deleted_at = ? WHERE follower_communication_id = ? AND following_communication_id = ? AND deleted_at IS NULL`
	result := service.ExecuteDb(query, now, followerCommunicationID, followingCommunicationID)
	if !result.IsSuccess() {
		return fmt.Errorf("取消关注失败: %v", result.Error)
	}

	// 更新统计缓存
	updateFollowStats(followerCommunicationID)
	updateFollowStats(followingCommunicationID)

	return nil
}

// GetFollowing 获取关注列表
func GetFollowing(userCommunicationID string) ([]User, error) {
	service, err := getService()
	if err != nil {
		return nil, fmt.Errorf("获取数据库服务失败: %v", err)
	}

	query := `SELECT csu.user_communication_id, u.nickname, u.avatar_url 
		FROM follows f
		INNER JOIN communication_system_users csu ON f.following_communication_id = csu.user_communication_id
		INNER JOIN common.users u ON csu.user_id = u.user_id
		WHERE f.follower_communication_id = ? AND f.deleted_at IS NULL
		ORDER BY f.created_at DESC`

	result := service.QueryDb(query, userCommunicationID)
	if !result.IsSuccess() {
		return nil, fmt.Errorf("获取关注列表失败: %v", result.Error)
	}

	rows, ok := result.Data.([]map[string]interface{})
	if !ok {
		return []User{}, nil
	}

	users := make([]User, 0, len(rows))
	for _, row := range rows {
		user := User{}
		if userCommunicationID, ok := row["user_communication_id"].(string); ok {
			user.UserCommunicationID = userCommunicationID
		}
		if nickname, ok := row["nickname"].(string); ok {
			user.Nickname = nickname
		}
		if avatarURL, ok := row["avatar_url"].(string); ok {
			user.AvatarURL = avatarURL
		}
		users = append(users, user)
	}

	return users, nil
}

// GetFollowers 获取粉丝列表
func GetFollowers(userCommunicationID string) ([]User, error) {
	service, err := getService()
	if err != nil {
		return nil, fmt.Errorf("获取数据库服务失败: %v", err)
	}

	query := `SELECT csu.user_communication_id, u.nickname, u.avatar_url 
		FROM follows f
		INNER JOIN communication_system_users csu ON f.follower_communication_id = csu.user_communication_id
		INNER JOIN common.users u ON csu.user_id = u.user_id
		WHERE f.following_communication_id = ? AND f.deleted_at IS NULL
		ORDER BY f.created_at DESC`

	result := service.QueryDb(query, userCommunicationID)
	if !result.IsSuccess() {
		return nil, fmt.Errorf("获取粉丝列表失败: %v", result.Error)
	}

	rows, ok := result.Data.([]map[string]interface{})
	if !ok {
		return []User{}, nil
	}

	users := make([]User, 0, len(rows))
	for _, row := range rows {
		user := User{}
		if userCommunicationID, ok := row["user_communication_id"].(string); ok {
			user.UserCommunicationID = userCommunicationID
		}
		if nickname, ok := row["nickname"].(string); ok {
			user.Nickname = nickname
		}
		if avatarURL, ok := row["avatar_url"].(string); ok {
			user.AvatarURL = avatarURL
		}
		users = append(users, user)
	}

	return users, nil
}

// GetMutualFollows 获取互关列表
func GetMutualFollows(userCommunicationID string) ([]User, error) {
	service, err := getService()
	if err != nil {
		return nil, fmt.Errorf("获取数据库服务失败: %v", err)
	}

	query := `SELECT csu.user_communication_id, u.nickname, u.avatar_url 
		FROM follows f1
		INNER JOIN follows f2 ON f1.following_communication_id = f2.follower_communication_id AND f1.follower_communication_id = f2.following_communication_id
		INNER JOIN communication_system_users csu ON f1.following_communication_id = csu.user_communication_id
		INNER JOIN common.users u ON csu.user_id = u.user_id
		WHERE f1.follower_communication_id = ? AND f1.deleted_at IS NULL AND f2.deleted_at IS NULL
		ORDER BY f1.created_at DESC`

	result := service.QueryDb(query, userCommunicationID)
	if !result.IsSuccess() {
		return nil, fmt.Errorf("获取互关列表失败: %v", result.Error)
	}

	rows, ok := result.Data.([]map[string]interface{})
	if !ok {
		return []User{}, nil
	}

	users := make([]User, 0, len(rows))
	for _, row := range rows {
		user := User{}
		if userCommunicationID, ok := row["user_communication_id"].(string); ok {
			user.UserCommunicationID = userCommunicationID
		}
		if nickname, ok := row["nickname"].(string); ok {
			user.Nickname = nickname
		}
		if avatarURL, ok := row["avatar_url"].(string); ok {
			user.AvatarURL = avatarURL
		}
		users = append(users, user)
	}

	return users, nil
}

// CheckFollow 检查关注关系
func CheckFollow(followerCommunicationID, followingCommunicationID string) (bool, error) {
	service, err := getService()
	if err != nil {
		return false, fmt.Errorf("获取数据库服务失败: %v", err)
	}

	query := `SELECT COUNT(*) as count FROM follows WHERE follower_communication_id = ? AND following_communication_id = ? AND deleted_at IS NULL`
	result := service.QueryDb(query, followerCommunicationID, followingCommunicationID)
	if !result.IsSuccess() {
		return false, fmt.Errorf("检查关注关系失败: %v", result.Error)
	}

	rows, ok := result.Data.([]map[string]interface{})
	if !ok || len(rows) == 0 {
		return false, nil
	}

	count, ok := rows[0]["count"].(int64)
	if !ok {
		if countFloat, ok := rows[0]["count"].(float64); ok {
			count = int64(countFloat)
		} else {
			return false, nil
		}
	}

	return count > 0, nil
}

// GetFollowStats 获取关注统计
func GetFollowStats(userCommunicationID string) (FollowStats, error) {
	service, err := getService()
	if err != nil {
		return FollowStats{}, fmt.Errorf("获取数据库服务失败: %v", err)
	}

	// 先尝试从缓存表获取
	query := `SELECT follower_count, following_count, mutual_follow_count 
		FROM user_relationship_stats 
		WHERE user_communication_id = ?`
	result := service.QueryDb(query, userCommunicationID)

	var stats FollowStats
	if result.IsSuccess() {
		rows, ok := result.Data.([]map[string]interface{})
		if ok && len(rows) > 0 {
			row := rows[0]
			if followerCount, ok := row["follower_count"].(int64); ok {
				stats.FollowerCount = int(followerCount)
			} else if followerCount, ok := row["follower_count"].(float64); ok {
				stats.FollowerCount = int(followerCount)
			}
			if followingCount, ok := row["following_count"].(int64); ok {
				stats.FollowingCount = int(followingCount)
			} else if followingCount, ok := row["following_count"].(float64); ok {
				stats.FollowingCount = int(followingCount)
			}
			if mutualCount, ok := row["mutual_follow_count"].(int64); ok {
				stats.MutualFollowCount = int(mutualCount)
			} else if mutualCount, ok := row["mutual_follow_count"].(float64); ok {
				stats.MutualFollowCount = int(mutualCount)
			}
			return stats, nil
		}
	}

	// 如果缓存不存在，计算并更新
	return updateFollowStats(userCommunicationID)
}

// GetFriendStats 获取好友数量
func GetFriendStats(userCommunicationID string) (int, error) {
	service, err := getService()
	if err != nil {
		return 0, fmt.Errorf("获取数据库服务失败: %v", err)
	}

	// 先尝试从缓存表获取
	query := `SELECT friend_count FROM user_relationship_stats WHERE user_communication_id = ?`
	result := service.QueryDb(query, userCommunicationID)

	if result.IsSuccess() {
		rows, ok := result.Data.([]map[string]interface{})
		if ok && len(rows) > 0 {
			row := rows[0]
			if friendCount, ok := row["friend_count"].(int64); ok {
				return int(friendCount), nil
			} else if friendCount, ok := row["friend_count"].(float64); ok {
				return int(friendCount), nil
			}
		}
	}

	// 如果缓存不存在，计算并更新
	_, err = updateFriendStats(userCommunicationID)
	if err != nil {
		return 0, err
	}

	// 重新查询
	result = service.QueryDb(query, userCommunicationID)
	if result.IsSuccess() {
		rows, ok := result.Data.([]map[string]interface{})
		if ok && len(rows) > 0 {
			row := rows[0]
			if friendCount, ok := row["friend_count"].(int64); ok {
				return int(friendCount), nil
			} else if friendCount, ok := row["friend_count"].(float64); ok {
				return int(friendCount), nil
			}
		}
	}

	return 0, nil
}

// updateFriendStats 更新好友统计缓存
func updateFriendStats(userCommunicationID string) (int, error) {
	service, err := getService()
	if err != nil {
		return 0, fmt.Errorf("获取数据库服务失败: %v", err)
	}

	// 计算好友数量
	query := `SELECT COUNT(*) as count FROM friends WHERE user_communication_id = ? AND deleted_at IS NULL`
	result := service.QueryDb(query, userCommunicationID)
	if !result.IsSuccess() {
		return 0, fmt.Errorf("计算好友数量失败: %v", result.Error)
	}

	rows, ok := result.Data.([]map[string]interface{})
	if !ok || len(rows) == 0 {
		return 0, nil
	}

	var friendCount int
	if count, ok := rows[0]["count"].(int64); ok {
		friendCount = int(count)
	} else if count, ok := rows[0]["count"].(float64); ok {
		friendCount = int(count)
	}

	// 更新或插入统计缓存
	updateQuery := `INSERT INTO user_relationship_stats (user_communication_id, friend_count, updated_at) 
		VALUES (?, ?, NOW())
		ON DUPLICATE KEY UPDATE friend_count = ?, updated_at = NOW()`
	updateResult := service.ExecuteDb(updateQuery, userCommunicationID, friendCount, friendCount)
	if !updateResult.IsSuccess() {
		return friendCount, fmt.Errorf("更新好友统计失败: %v", updateResult.Error)
	}

	return friendCount, nil
}

// updateFollowStats 更新关注统计缓存
func updateFollowStats(userCommunicationID string) (FollowStats, error) {
	service, err := getService()
	if err != nil {
		return FollowStats{}, fmt.Errorf("获取数据库服务失败: %v", err)
	}

	// 计算粉丝数量
	followerQuery := `SELECT COUNT(*) as count FROM follows WHERE following_communication_id = ? AND deleted_at IS NULL`
	followerResult := service.QueryDb(followerQuery, userCommunicationID)

	var followerCount int
	if followerResult.IsSuccess() {
		rows, ok := followerResult.Data.([]map[string]interface{})
		if ok && len(rows) > 0 {
			if count, ok := rows[0]["count"].(int64); ok {
				followerCount = int(count)
			} else if count, ok := rows[0]["count"].(float64); ok {
				followerCount = int(count)
			}
		}
	}

	// 计算关注数量
	followingQuery := `SELECT COUNT(*) as count FROM follows WHERE follower_communication_id = ? AND deleted_at IS NULL`
	followingResult := service.QueryDb(followingQuery, userCommunicationID)

	var followingCount int
	if followingResult.IsSuccess() {
		rows, ok := followingResult.Data.([]map[string]interface{})
		if ok && len(rows) > 0 {
			if count, ok := rows[0]["count"].(int64); ok {
				followingCount = int(count)
			} else if count, ok := rows[0]["count"].(float64); ok {
				followingCount = int(count)
			}
		}
	}

	// 计算互关数量
	mutualQuery := `SELECT COUNT(*) as count 
		FROM follows f1
		INNER JOIN follows f2 ON f1.following_communication_id = f2.follower_communication_id AND f1.follower_communication_id = f2.following_communication_id
		WHERE f1.follower_communication_id = ? AND f1.deleted_at IS NULL AND f2.deleted_at IS NULL`
	mutualResult := service.QueryDb(mutualQuery, userCommunicationID)

	var mutualCount int
	if mutualResult.IsSuccess() {
		rows, ok := mutualResult.Data.([]map[string]interface{})
		if ok && len(rows) > 0 {
			if count, ok := rows[0]["count"].(int64); ok {
				mutualCount = int(count)
			} else if count, ok := rows[0]["count"].(float64); ok {
				mutualCount = int(count)
			}
		}
	}

	stats := FollowStats{
		FollowerCount:     followerCount,
		FollowingCount:    followingCount,
		MutualFollowCount: mutualCount,
	}

	// 更新或插入统计缓存
	updateQuery := `INSERT INTO user_relationship_stats 
		(user_communication_id, follower_count, following_count, mutual_follow_count, updated_at) 
		VALUES (?, ?, ?, ?, NOW())
		ON DUPLICATE KEY UPDATE 
		follower_count = ?, following_count = ?, mutual_follow_count = ?, updated_at = NOW()`
	updateResult := service.ExecuteDb(updateQuery, userCommunicationID, followerCount, followingCount, mutualCount,
		followerCount, followingCount, mutualCount)
	if !updateResult.IsSuccess() {
		return stats, fmt.Errorf("更新关注统计失败: %v", updateResult.Error)
	}

	return stats, nil
}
