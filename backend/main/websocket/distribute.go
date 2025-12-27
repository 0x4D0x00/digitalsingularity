package websocket

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/gorilla/websocket"
	"digitalsingularity/backend/common/security/asymmetricencryption/decrypt"
)

// 处理主服务WebSocket连接
func handleMainServiceConnection(w http.ResponseWriter, r *http.Request) {
	// 升级HTTP连接为WebSocket
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		logger.Printf("升级为WebSocket连接失败: %v", err)
		return
	}
	defer conn.Close()

	// 客户端地址
	clientAddr := conn.RemoteAddr().String()
	logger.Printf("新的主服务WebSocket连接: %s", clientAddr)
	
	// 注册连接到连接管理器
	if connectionManager != nil {
		connectionManager.RegisterConnection(conn)
		defer connectionManager.UnregisterConnection(conn)
	}

	// 设置Pong处理器，用于接收客户端响应心跳
	conn.SetPongHandler(func(string) error {
		// 收到Pong消息时，延长读取 deadline
		conn.SetReadDeadline(time.Now().Add(70 * time.Second))
		return nil
	})

	// 设置初始读取超时（70秒，比心跳间隔长）
	conn.SetReadDeadline(time.Now().Add(70 * time.Second))

	// 启动心跳 goroutine，每60秒发送一次 Ping
	pingTicker := time.NewTicker(60 * time.Second)
	defer pingTicker.Stop()

	// 用于通知心跳 goroutine 连接已关闭
	done := make(chan struct{})
	defer close(done)

	go func() {
		for {
			select {
			case <-pingTicker.C:
				// 发送 Ping 消息
				if err := conn.WriteControl(websocket.PingMessage, []byte{}, time.Now().Add(10*time.Second)); err != nil {
					logger.Printf("[心跳] 发送Ping失败 (%s): %v", clientAddr, err)
					return
				}
			case <-done:
				return
			}
		}
	}()

	// 认证状态
	authenticated := false
	var userID string
	connectionID := generateConnectionID() // 生成连接ID用于日志跟踪

	// 处理客户端消息
	for {
		messageType, message, err := conn.ReadMessage()
		if err != nil {
			// 区分正常关闭和异常关闭
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				logger.Printf("WebSocket连接异常关闭 (%s): %v", clientAddr, err)
			} else {
				// 正常关闭或客户端主动断开
				if websocket.IsCloseError(err, websocket.CloseNormalClosure, websocket.CloseGoingAway) {
					logger.Printf("WebSocket连接正常关闭 (%s): %v", clientAddr, err)
				} else {
					// 检查是否是超时错误
					if netErr, ok := err.(interface{ Timeout() bool }); ok && netErr.Timeout() {
						logger.Printf("WebSocket读取超时 (%s): %v (可能是网络问题或客户端无响应)", clientAddr, err)
					} else {
						logger.Printf("WebSocket读取消息失败 (%s): %v (可能是连接已断开)", clientAddr, err)
					}
				}
			}
			break
		}

		// 每次成功读取消息后，延长读取 deadline（支持长时间传输数据）
		conn.SetReadDeadline(time.Now().Add(70 * time.Second))

		// 第一层：根据消息类型分类处理
		switch messageType {
		case websocket.BinaryMessage:
			// 处理二进制消息（音频数据）
			if !authenticated {
				logger.Printf("[WS:%s] 收到二进制数据但连接未认证", connectionID)
				continue
			}
			
			if err := handleSpeechAudioData(conn, connectionID, userID, message); err != nil {
				logger.Printf("[WS:%s] 处理音频数据失败: %v", connectionID, err)
			}
			continue

		case websocket.TextMessage:
			// 处理文本消息（JSON）
			logger.Printf("[WS:%s] 收到主服务消息(%s): %s", connectionID, clientAddr, string(message))
			
			if len(message) == 0 {
				continue
			}
			
			var messageData map[string]interface{}
			if err := json.Unmarshal(message, &messageData); err != nil {
				logger.Printf("[WS:%s] 解析消息失败: %v", connectionID, err)
				continue
			}
			
			// 检查是否有ciphertext（加密消息）
			ciphertext, hasCiphertext := messageData["ciphertext"].(string)
			if hasCiphertext && ciphertext != "" {
				// 加密消息（其他业务都应该是加密的）
				logger.Printf("收到加密消息，长度: %d", len(ciphertext))
				
				// 解密消息
				info := map[string]string{
					"filePath": "config",
					"userName": "server",
				}
				
				decryptedData, err := decrypt.AsymmetricDecryptService(ciphertext, info)
				if err != nil {
					logger.Printf("解密消息失败: %v", err)
					errorResponse := map[string]interface{}{
						"type": "error",
						"data": map[string]interface{}{
							"status": "error",
							"message": fmt.Sprintf("消息解密失败: %v", err),
						},
					}
					responseBytes, _ := json.Marshal(errorResponse)
					conn.WriteMessage(messageType, responseBytes)
					continue
				}
				
				logger.Printf("消息解密成功，解密后长度: %d", len(decryptedData))
				
				// 解析解密后的数据
				if err := json.Unmarshal([]byte(decryptedData), &messageData); err != nil {
					logger.Printf("解析解密后的消息失败: %v", err)
					errorResponse := map[string]interface{}{
						"type": "error",
						"data": map[string]interface{}{
							"status": "error",
							"message": "解密后的消息格式错误",
						},
					}
					responseBytes, _ := json.Marshal(errorResponse)
					conn.WriteMessage(messageType, responseBytes)
					continue
				}
				
				logger.Printf("解密后的消息已解析，类型: %v", messageData["type"])
			} else {
				// 没有ciphertext，检查是否是允许明文的消息类型
				// auth消息必须在认证前发送，所以允许明文
				// 语音消息也可以明文
				if msgTypeVal, ok := messageData["type"].(string); ok {
					isPlaintextAllowed := msgTypeVal == "auth" || 
						msgTypeVal == "ping" ||
						msgTypeVal == "speech_interaction_start" || 
						msgTypeVal == "speech_interaction_end"
					
					if !isPlaintextAllowed {
						// 非允许明文的消息类型必须是加密的
						logger.Printf("错误: 收到非语音明文消息，类型: %v", msgTypeVal)
						errorResponse := map[string]interface{}{
							"type": "error",
							"data": map[string]interface{}{
								"status": "error",
								"message": "非语音消息必须加密",
							},
						}
						responseBytes, _ := json.Marshal(errorResponse)
						conn.WriteMessage(messageType, responseBytes)
						continue
					}
					
					// 明文消息（auth或语音消息）
					if msgData, ok := messageData["data"].(map[string]interface{}); ok {
						if action, ok := msgData["action"].(string); ok {
							logger.Printf("收到明文消息，类型: %v, action: %v", msgTypeVal, action)
						} else {
							logger.Printf("收到明文消息，类型: %v", msgTypeVal)
						}
					} else {
						logger.Printf("收到明文消息，类型: %v", msgTypeVal)
					}
				}
			}
			
			// 提取消息类型并处理业务逻辑
			msgType, _ := messageData["type"].(string)
			if msgType != "" {
				switch msgType {
					case "auth":
						// 处理认证请求
						if authData, ok := messageData["data"].(map[string]interface{}); ok {
							if authToken, ok := authData["auth_token"].(string); ok {
								// 验证authToken
								if authTokenService == nil {
									logger.Printf("错误: authTokenService未初始化")
									authResponse := map[string]interface{}{
										"type": "auth_failed",
										"data": map[string]interface{}{
											"status": "error",
											"message": "服务未就绪",
										},
									}
									responseBytes, _ := json.Marshal(authResponse)
									conn.WriteMessage(messageType, responseBytes)
									continue
								}
								
								logger.Printf("收到认证请求，验证authToken...")
								
								// 使用authTokenService验证authToken
								valid, result := authTokenService.VerifyAuthToken(authToken)
								if !valid {
									errMsg := "认证失败"
									if msg, ok := result.(string); ok {
										errMsg = msg
									}
									logger.Printf("AuthToken验证失败: %s", errMsg)
									authResponse := map[string]interface{}{
										"type": "auth_failed",
										"data": map[string]interface{}{
											"status": "error",
											"message": errMsg,
										},
									}
									responseBytes, _ := json.Marshal(authResponse)
									conn.WriteMessage(messageType, responseBytes)
									continue
								}
								
								// AuthToken验证成功，从payload中获取用户ID
								payload, ok := result.(map[string]interface{})
								if !ok {
									logger.Printf("AuthToken验证返回数据格式错误")
									authResponse := map[string]interface{}{
										"type": "auth_failed",
										"data": map[string]interface{}{
											"status": "error",
											"message": "认证失败: 数据格式错误",
										},
									}
									responseBytes, _ := json.Marshal(authResponse)
									conn.WriteMessage(messageType, responseBytes)
									continue
								}
								
								// 从payload中提取userId（注意：authTokenService返回的是"userId"而不是"user_id"）
								if uid, ok := payload["userId"].(string); ok {
									userID = uid
								} else {
									userID = "unknown_user"
									logger.Printf("警告: 无法从authToken中提取userId")
								}
								
								authenticated = true
								logger.Printf("用户 %s 认证成功", userID)
								
								// 注册用户连接到在线通知系统
								RegisterUserConnection(userID, conn)
								
								// 拉取并发送离线通知
								sendOfflineNotifications(userID, conn)
								
								// 发送认证成功响应
								authResponse := map[string]interface{}{
									"type": "auth_success",
									"data": map[string]interface{}{
										"status":  "success",
										"message": "认证成功",
									},
								}
								responseBytes, _ := json.Marshal(authResponse)
								conn.WriteMessage(messageType, responseBytes)
								continue
							}
						}
						
						// 认证失败（未提供authToken或格式错误）
						authResponse := map[string]interface{}{
							"type": "auth_failed",
							"data": map[string]interface{}{
								"status": "error",
								"message": "认证失败: 缺少auth_token或格式错误",
							},
						}
						responseBytes, _ := json.Marshal(authResponse)
						conn.WriteMessage(messageType, responseBytes)
						continue
						
					case "ping":
						// 处理心跳
						if authenticated {
							pongResponse := map[string]interface{}{
								"type": "pong",
								"data": map[string]interface{}{
									"timestamp": fmt.Sprintf("%d", getCurrentTimestamp()),
								},
							}
							responseBytes, _ := json.Marshal(pongResponse)
							conn.WriteMessage(messageType, responseBytes)
						}
						continue
						
					// 注意：check_db_version 已迁移到HTTP接口，请使用 HTTP: type="aiBasicPlatform", operation="check_db_version"
						
					// 注意：以下消息类型已迁移到HTTP接口，请使用HTTP API：
					// - check_version -> HTTP: type="aiBasicPlatform", operation="check_version"
					// - user_token -> HTTP: type="userStatus", operation="user_token"
					// - logout -> HTTP: type="userStatus", operation="logout"
					// - deactivation -> HTTP: type="userStatus", operation="deactivation"
					// - modify_username/modify_nickname/modify_mobile/modify_email -> HTTP: type="userInfo", operation="modify_*"
						
					case "silicoid":
						if !authenticated {
							response := map[string]interface{}{
								"type": "error",
								"data": map[string]interface{}{
									"status":  "error",
									"message": "请先进行认证",
								},
							}
							responseBytes, _ := json.Marshal(response)
							conn.WriteMessage(messageType, responseBytes)
							continue
						}

						actionVal, ok := messageData["action"].(string)
						if !ok {
							response := map[string]interface{}{
								"type": "error",
								"data": map[string]interface{}{
									"status":  "error",
									"message": "缺少Silicoid action",
								},
							}
							responseBytes, _ := json.Marshal(response)
							conn.WriteMessage(messageType, responseBytes)
							continue
						}

						action := strings.TrimSpace(actionVal)
						if action == "" {
							response := map[string]interface{}{
								"type": "error",
								"data": map[string]interface{}{
									"status":  "error",
									"message": "Silicoid action不能为空",
								},
							}
							responseBytes, _ := json.Marshal(response)
							conn.WriteMessage(messageType, responseBytes)
							continue
						}

						switch action {
						case "chat":
							logger.Printf("用户 %s 发起Silicoid聊天请求", userID)

							// 提取用户公钥（如果有）
							userPublicKey := ""
							// 处理 data 可能是字符串或 map 的情况
							if dataStr, ok := messageData["data"].(string); ok {
								// data 是 JSON 字符串，解析它
								var chatData map[string]interface{}
								if err := json.Unmarshal([]byte(dataStr), &chatData); err == nil {
									// 优先使用新的字段名 userPublicKeyHex，向后兼容 userPublicKeyBase64
									if pubKey, ok := chatData["userPublicKeyHex"].(string); ok {
										userPublicKey = pubKey
									} else if pubKey, ok := chatData["userPublicKeyBase64"].(string); ok {
										userPublicKey = pubKey
									}
								}
							} else if msgData, ok := messageData["data"].(map[string]interface{}); ok {
								// data 已经是 map
								// 优先使用新的字段名 userPublicKeyHex，向后兼容 userPublicKeyBase64
								if pubKey, ok := msgData["userPublicKeyHex"].(string); ok {
									userPublicKey = pubKey
								} else if pubKey, ok := msgData["userPublicKeyBase64"].(string); ok {
									userPublicKey = pubKey
								}
							}

							// 在后台goroutine中处理聊天
							go toAIProcessNonStreamingChat(conn, userID, messageData, userPublicKey)

							// 立即发送处理开始确认
							response := map[string]interface{}{
								"type": "ai_processing_started",
								"data": map[string]interface{}{
									"status": "success",
									"message": "AI处理已开始",
								},
							}
							responseBytes, _ := json.Marshal(response)
							conn.WriteMessage(messageType, responseBytes)
						default:
							response := map[string]interface{}{
								"type": "error",
								"data": map[string]interface{}{
									"status":  "error",
									"message": fmt.Sprintf("未知的Silicoid action: %s", action),
								},
							}
							responseBytes, _ := json.Marshal(response)
							conn.WriteMessage(messageType, responseBytes)
						}
						continue
						
					case "speech_system":
						if !authenticated {
							response := map[string]interface{}{
								"type": "error",
								"data": map[string]interface{}{
									"status":  "error",
									"message": "请先进行认证",
								},
							}
							responseBytes, _ := json.Marshal(response)
							conn.WriteMessage(messageType, responseBytes)
							continue
						}

						actionVal, ok := messageData["action"].(string)
						if !ok {
							response := map[string]interface{}{
								"type": "error",
								"data": map[string]interface{}{
									"status":  "error",
									"message": "缺少语音action",
								},
							}
							responseBytes, _ := json.Marshal(response)
							conn.WriteMessage(messageType, responseBytes)
							continue
						}

						action := strings.TrimSpace(actionVal)
						if action == "" {
							response := map[string]interface{}{
								"type": "error",
								"data": map[string]interface{}{
									"status":  "error",
									"message": "语音action不能为空",
								},
							}
							responseBytes, _ := json.Marshal(response)
							conn.WriteMessage(messageType, responseBytes)
							continue
						}

						switch action {
						case "speech_interaction_start":
							if err := handleSpeechInteractionStart(conn, connectionID, userID, messageData); err != nil {
								logger.Printf("[WS:%s] 处理语音交互开始失败: %v", connectionID, err)
							}
						case "speech_interaction_end":
							if err := handleSpeechInteractionEnd(conn, connectionID, userID); err != nil {
								logger.Printf("[WS:%s] 处理语音交互结束失败: %v", connectionID, err)
							}
						default:
							response := map[string]interface{}{
								"type": "error",
								"data": map[string]interface{}{
									"status":  "error",
									"message": fmt.Sprintf("未知的语音action: %s", action),
								},
							}
							responseBytes, _ := json.Marshal(response)
							conn.WriteMessage(messageType, responseBytes)
						}
						continue

					case "relationship_management":
						// 处理关系管理相关消息
						if authenticated {
							handleRelationshipManagementMessage(conn, connectionID, userID, messageData)
						} else {
							response := map[string]interface{}{
								"type": "error",
								"data": map[string]interface{}{
									"status": "error",
									"message": "请先进行认证",
								},
							}
							responseBytes, _ := json.Marshal(response)
							conn.WriteMessage(messageType, responseBytes)
						}
						continue

					case "communication_system":
						// 处理通信系统消息（聊天消息）
						if authenticated {
							toBackendCommunicationSystemMessage(conn, connectionID, userID, messageData)
						} else {
							response := map[string]interface{}{
								"type": "error",
								"data": map[string]interface{}{
									"status": "error",
									"message": "请先进行认证",
								},
							}
							responseBytes, _ := json.Marshal(response)
							conn.WriteMessage(messageType, responseBytes)
						}
						continue

					case "response":
						// 统一的 response 类型，用于承载多类回传
						if !authenticated {
							resp := map[string]interface{}{
								"type": "error",
								"data": map[string]interface{}{
									"status":  "error",
									"message": "请先进行认证",
								},
							}
							responseBytes, _ := json.Marshal(resp)
							conn.WriteMessage(messageType, responseBytes)
							continue
						}

						// 检查是否为 client executor 的回传
						if rc, ok := messageData["response_category"].(string); ok && rc == "client_executor_result" {
							go handleClientExecutorResult(conn, connectionID, userID, messageData)
							continue
						}
						// 其他 response 类型可继续扩展
						continue
					default:
						// 处理其他未知消息类型
						if authenticated {
							// 已认证用户的消息处理
							response := map[string]interface{}{
								"type": "message_processed",
								"data": map[string]interface{}{
									"status": "success",
									"message": fmt.Sprintf("已处理消息: %s", msgType),
								},
							}
							responseBytes, _ := json.Marshal(response)
							conn.WriteMessage(messageType, responseBytes)
						} else {
							// 未认证用户
							response := map[string]interface{}{
								"type": "error",
								"data": map[string]interface{}{
									"status": "error",
									"message": "请先进行认证",
								},
							}
							responseBytes, _ := json.Marshal(response)
							conn.WriteMessage(messageType, responseBytes)
						}
						continue
					}
				}
			
		default:
			// 未知的消息类型
			logger.Printf("[WS:%s] 收到未知类型的消息: %v", connectionID, messageType)
			continue
		}
	}

	// 连接关闭时，清理所有相关的会话
	cleanupConnSessions(conn)
	
	// 清理语音会话
	removeSpeechSession(conn)
	
	// 注销用户连接到在线通知系统
	UnregisterUserConnection(conn)
	
	logger.Printf("主服务WebSocket连接已关闭: %s", clientAddr)
}

// cleanupConnSessions 清理连接相关的所有会话
// 目前主要用于扩展性，未来如果有其他类型的会话需要清理，可以在这里添加
func cleanupConnSessions(conn *websocket.Conn) {
	// 目前所有会话清理都在各自的地方处理（如 removeSpeechSession）
	// 此函数保留用于未来扩展其他类型的会话清理
	logger.Printf("清理连接会话: %s", conn.RemoteAddr().String())
}

// handleClientExecutorResult 处理前端回传的 client_executor_result（路由分发）
func handleClientExecutorResult(conn *websocket.Conn, connectionID string, userID string, messageData map[string]interface{}) {
	// 不信任外部传入的 userID，优先使用服务器维护的连接->userID 映射
	if uid, ok := GetUserIDByConnection(conn); ok {
		userID = uid
	} else {
		if userID == "" {
			userID = "unknown_user"
		}
	}

	logger.Printf("路由 client_executor_result 到 silicoid (user=%s)", userID)

	// 调用silicoid处理业务逻辑
	go toAIProcessClientExecutorResult(conn, userID, messageData, "")
}

// sendOfflineNotifications 发送用户的离线通知
func sendOfflineNotifications(userID string, conn *websocket.Conn) {
	// 获取未读离线通知（限制100条）
	notifications, err := GetUserUnreadNotifications(userID, 100)
	if err != nil {
		logger.Printf("[WS] 获取离线通知失败 (用户: %s): %v", userID, err)
		return
	}

	if len(notifications) == 0 {
		return
	}

	logger.Printf("[WS] 用户 %s 有 %d 条离线通知", userID, len(notifications))

	// 发送离线通知
	notificationIDs := make([]int64, 0, len(notifications))
	for _, notif := range notifications {
		// 构建通知消息
		notificationMsg := map[string]interface{}{
			"type": notif.NotificationType,
			"data": notif.NotificationData,
		}

		// 添加通知ID，用于标记已读
		if notif.ID > 0 {
			notificationMsg["notification_id"] = notif.ID
		}

		notificationBytes, err := json.Marshal(notificationMsg)
		if err != nil {
			logger.Printf("[WS] 序列化离线通知失败: %v", err)
			continue
		}

		// 发送通知
		if err := conn.WriteMessage(websocket.TextMessage, notificationBytes); err != nil {
			logger.Printf("[WS] 发送离线通知失败: %v", err)
			break
		}

		notificationIDs = append(notificationIDs, notif.ID)
	}

	// 标记所有已发送的通知为已读
	if len(notificationIDs) > 0 {
		if err := MarkUserNotificationsAsRead(userID, notificationIDs); err != nil {
			logger.Printf("[WS] 标记通知为已读失败 (用户: %s): %v", userID, err)
		} else {
			logger.Printf("[WS] 已标记 %d 条通知为已读 (用户: %s)", len(notificationIDs), userID)
		}
	}
}

// generateConnectionID 生成连接ID（用于日志跟踪）
func generateConnectionID() string {
	// 使用时间戳生成一个简短的ID
	return fmt.Sprintf("%d", time.Now().UnixNano()%100000000)
}