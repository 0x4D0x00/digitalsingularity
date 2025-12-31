package server

// Storagebox IP存储MCP服务器
import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"strings"

	"digitalsingularity/backend/common/utils/datahandle"
	"digitalsingularity/backend/modelcontextprotocol/server/cybersecurity/network"
	_ "github.com/go-sql-driver/mysql"
)

// StorageboxDataService 处理Storagebox相关的数据服务
// 参考 backend/silicoid/database/service.go 的设计模式
type StorageboxDataService struct {
	dataService *datahandle.CommonReadWriteService
	dbName      string
}

// NewStorageboxDataService 创建Storagebox数据服务实例
func NewStorageboxDataService(dataService *datahandle.CommonReadWriteService) *StorageboxDataService {
	return &StorageboxDataService{
		dataService: dataService,
		dbName:      "storagebox",
	}
}

// GetDatabaseName 返回数据库名
func (s *StorageboxDataService) GetDatabaseName() string {
	return s.dbName
}

// 包级别的默认服务实例
var defaultStorageboxService *StorageboxDataService

// init 初始化默认的服务实例
func init() {
	dataService, err := datahandle.NewCommonReadWriteService("storagebox")
	if err != nil {
		log.Printf("Failed to create default storagebox data service: %v", err)
		return
	}
	defaultStorageboxService = NewStorageboxDataService(dataService)
}


// DatabaseConfig 数据库配置结构体
type DatabaseConfig struct {
	Host     string
	Port     string
	User     string
	Password string
	Name     string
}

// getStorageboxDBConfig 获取Storagebox数据库配置
func getStorageboxDBConfig() DatabaseConfig {
	// 使用默认的服务实例，参考silicoid/database/service.go的简洁模式
	if defaultStorageboxService == nil || defaultStorageboxService.dataService == nil {
		log.Printf("Storagebox data service not available")
		return DatabaseConfig{
			Host:     "localhost",
			Port:     "3306",
			User:     "root",
			Password: "",
			Name:     "storagebox",
		}
	}

	// 通过服务实例获取配置，简洁明了
	config := defaultStorageboxService.dataService.GetDbConfig()
	return DatabaseConfig{
		Host:     config["host"].(string),
		Port:     fmt.Sprintf("%v", config["port"]),
		User:     config["user"].(string),
		Password: config["password"].(string),
		Name:     "storagebox",
	}
}

// getDatabaseConnection 通用数据库连接函数
func getDatabaseConnection(config DatabaseConfig) (*sql.DB, error) {
	// 构建连接字符串
	dsn := fmt.Sprintf("%s:%s@tcp(%s:%s)/%s?charset=utf8mb4&parseTime=True&loc=Local",
		config.User, config.Password, config.Host, config.Port, config.Name)

	// 连接数据库
	db, err := sql.Open("mysql", dsn)
	if err != nil {
		return nil, fmt.Errorf("failed to open database connection: %v", err)
	}

	// 测试连接
	if err := db.Ping(); err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to ping database: %v", err)
	}

	return db, nil
}

// getStorageboxDB 获取Storagebox数据库连接
func getStorageboxDB() (*sql.DB, error) {
	config := getStorageboxDBConfig()
	return getDatabaseConnection(config)
}

// StorageboxIPStorageRequest IP存储请求结构
type StorageboxIPStorageRequest struct {
	IP          []string `json:"ip"`
	Description string   `json:"description,omitempty"`
}

// StorageboxIPStorageResponse IP存储响应结构
type StorageboxIPStorageResponse struct {
	Status     string `json:"status"`
	Message    string `json:"message"`
	StoredIPs  []string `json:"stored_ips,omitempty"`
	ErrorIPs   []string `json:"error_ips,omitempty"`
}

// StorageboxIPPortStorageRequest IP+端口存储请求结构
type StorageboxIPPortStorageRequest struct {
	IPPortList []IPPortItem `json:"ip_port_list"`
	Service    string       `json:"service,omitempty"`
}

// IPPortItem IP和端口组合
type IPPortItem struct {
	IP    string   `json:"ip"`
	Ports []string `json:"ports"`
}

// StorageboxIPPortStorageResponse IP+端口存储响应结构
type StorageboxIPPortStorageResponse struct {
	Status      string `json:"status"`
	Message     string `json:"message"`
	StoredItems []IPPortItem `json:"stored_items,omitempty"`
	ErrorItems  []IPPortItem `json:"error_items,omitempty"`
}

// StorageboxIPStorage Storagebox数据库操作服务器
func StorageboxIPStorage(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	// 只允许POST请求
	if r.Method != http.MethodPost {
		response := map[string]interface{}{
			"status":  "error",
			"message": "Method not allowed. Use POST.",
		}
		w.WriteHeader(http.StatusMethodNotAllowed)
		json.NewEncoder(w).Encode(response)
		return
	}

	// 解析MCP工具调用请求
	var mcpReq map[string]interface{}
	if err := json.NewDecoder(r.Body).Decode(&mcpReq); err != nil {
		mcpResponse := map[string]interface{}{
			"id": mcpReq["id"],
			"error": map[string]interface{}{
				"code":    -32700,
				"message": fmt.Sprintf("Invalid JSON: %v", err),
			},
		}
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(mcpResponse)
		return
	}

	// 获取请求ID
	requestID, _ := mcpReq["id"].(string)
	if requestID == "" {
		requestID = "unknown"
	}

	// 获取参数部分（MCP协议格式）
	params, ok := mcpReq["params"].(map[string]interface{})
	if !ok {
		mcpResponse := map[string]interface{}{
			"id": requestID,
			"error": map[string]interface{}{
				"code":    -32600,
				"message": "Missing params in MCP request",
			},
		}
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(mcpResponse)
		return
	}

	// 获取工具名称
	toolName, ok := params["name"].(string)
	if !ok {
		mcpResponse := map[string]interface{}{
			"id": requestID,
			"error": map[string]interface{}{
				"code":    -32600,
				"message": "Missing tool name in params",
			},
		}
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(mcpResponse)
		return
	}

	// 获取工具参数
	args, ok := params["arguments"].(map[string]interface{})
	if !ok {
		mcpResponse := map[string]interface{}{
			"id": requestID,
			"error": map[string]interface{}{
				"code":    -32600,
				"message": "Missing arguments in params",
			},
		}
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(mcpResponse)
		return
	}

	// 根据工具名称分发处理
	switch toolName {
	case "mcp_storagebox_ip_address":
		handleIPAddressStorage(w, args, requestID)
	case "mcp_storagebox_ip_port":
		handleIPPortStorage(w, args, requestID)
	case "mcp_query_storagebox_data":
		handleDataQueryStorage(w, args, requestID)
	default:
		mcpResponse := map[string]interface{}{
			"id": requestID,
			"error": map[string]interface{}{
				"code":    -32601,
				"message": fmt.Sprintf("Unknown tool: %s", toolName),
			},
		}
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(mcpResponse)
	}
}

// handleIPAddressStorage 处理IP地址存储
func handleIPAddressStorage(w http.ResponseWriter, args map[string]interface{}, requestID string) {
	// 解析请求体
	var req StorageboxIPStorageRequest
	if ipData, ok := args["ip"].([]interface{}); ok {
		req.IP = make([]string, len(ipData))
		for i, ip := range ipData {
			if ipStr, ok := ip.(string); ok {
				req.IP[i] = ipStr
			}
		}
	}

	if desc, ok := args["description"].(string); ok {
		req.Description = desc
	}

	// 验证IP数组
	if len(req.IP) == 0 {
		mcpResponse := map[string]interface{}{
			"id": requestID,
			"error": map[string]interface{}{
				"code":    -32602,
				"message": "IP array cannot be empty",
			},
		}
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(mcpResponse)
		return
	}

	// 连接数据库
	db, err := getStorageboxDB()
	if err != nil {
		log.Printf("Database connection error: %v", err)
		mcpResponse := map[string]interface{}{
			"id": requestID,
			"error": map[string]interface{}{
				"code":    -32603,
				"message": "Database connection failed",
			},
		}
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(mcpResponse)
		return
	}
	defer db.Close()

	// 先进行Nmap扫描
	var scanResults []*network.ScanResult
	if len(req.IP) == 1 {
		// 单个IP使用同步扫描
		scanResults = []*network.ScanResult{network.ScanSingleIP(req.IP[0])}
	} else {
		// 多个IP使用并发扫描
		scanResults = network.ScanIPList(req.IP)
	}

	// 存储IP地址和发现的端口信息
	var storedIPs []string
	var errorIPs []string

	for _, ip := range req.IP {
		// 验证IP格式（基础验证）
		if !isValidIP(ip) {
			errorIPs = append(errorIPs, ip)
			continue
		}

		// 插入到IP地址表
		_, err := db.Exec(`
			INSERT INTO ip_addresses (ip_address, description, created_at, updated_at)
			VALUES (?, ?, NOW(), NOW())
			ON DUPLICATE KEY UPDATE
				description = VALUES(description),
				updated_at = NOW()
		`, ip, req.Description)

		if err != nil {
			log.Printf("Failed to store IP %s: %v", ip, err)
			errorIPs = append(errorIPs, ip)
			continue
		}

		storedIPs = append(storedIPs, ip)
		log.Printf("Successfully stored IP: %s", ip)

		// 存储发现的开放端口
		for _, result := range scanResults {
			if result.IP == ip && len(result.Ports) > 0 {
				for _, port := range result.Ports {
					_, err := db.Exec(`
						INSERT INTO ip_ports (ip_address, port, service, created_at, updated_at)
						VALUES (?, ?, '', NOW(), NOW())
						ON DUPLICATE KEY UPDATE
							updated_at = NOW()
					`, ip, port)

					if err != nil {
						log.Printf("Failed to store port %s for IP %s: %v", port, ip, err)
					} else {
						log.Printf("Successfully stored port %s for IP %s", port, ip)
					}
				}
				break
			}
		}
	}

	// 构建MCP响应 - 只报告存储结果，不提扫描相关信息
	mcpResponse := map[string]interface{}{
		"id": requestID,
		"result": map[string]interface{}{
			"stored_ips": storedIPs,
			"error_ips":  errorIPs,
		},
	}

	if len(errorIPs) == 0 {
		mcpResponse["result"].(map[string]interface{})["status"] = "success"
		mcpResponse["result"].(map[string]interface{})["message"] = fmt.Sprintf("Successfully stored %d IP addresses and their ports", len(storedIPs))
	} else if len(storedIPs) > 0 {
		mcpResponse["result"].(map[string]interface{})["status"] = "partial_success"
		mcpResponse["result"].(map[string]interface{})["message"] = fmt.Sprintf("Stored %d IP addresses and their ports, failed to store %d IP addresses", len(storedIPs), len(errorIPs))
	} else {
		mcpResponse["result"].(map[string]interface{})["status"] = "error"
		mcpResponse["result"].(map[string]interface{})["message"] = "Failed to store any IP addresses"
		w.WriteHeader(http.StatusInternalServerError)
	}

	json.NewEncoder(w).Encode(mcpResponse)
}

// handleIPPortStorage 处理IP+端口存储
func handleIPPortStorage(w http.ResponseWriter, args map[string]interface{}, requestID string) {
	// 解析请求体
	var req StorageboxIPPortStorageRequest
	if ipPortData, ok := args["ip_port_list"].([]interface{}); ok {
		req.IPPortList = make([]IPPortItem, len(ipPortData))
		for i, item := range ipPortData {
			if itemMap, ok := item.(map[string]interface{}); ok {
				var ipPortItem IPPortItem
				if ip, ok := itemMap["ip"].(string); ok {
					ipPortItem.IP = ip
				}
				if portsData, ok := itemMap["ports"].([]interface{}); ok {
					ipPortItem.Ports = make([]string, len(portsData))
					for j, port := range portsData {
						if portStr, ok := port.(string); ok {
							ipPortItem.Ports[j] = portStr
						} else if portNum, ok := port.(float64); ok {
							ipPortItem.Ports[j] = fmt.Sprintf("%.0f", portNum)
						}
					}
				}
				req.IPPortList[i] = ipPortItem
			}
		}
	}

	if service, ok := args["service"].(string); ok {
		req.Service = service
	}

	// 验证数据
	if len(req.IPPortList) == 0 {
		mcpResponse := map[string]interface{}{
			"id": requestID,
			"error": map[string]interface{}{
				"code":    -32602,
				"message": "IP-Port list cannot be empty",
			},
		}
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(mcpResponse)
		return
	}

	// 连接数据库
	db, err := getStorageboxDB()
	if err != nil {
		log.Printf("Database connection error: %v", err)
		mcpResponse := map[string]interface{}{
			"id": requestID,
			"error": map[string]interface{}{
				"code":    -32603,
				"message": "Database connection failed",
			},
		}
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(mcpResponse)
		return
	}
	defer db.Close()

	// 提取需要扫描的IP列表
	var ipsToScan []string
	ipItemMap := make(map[string]IPPortItem)
	for _, item := range req.IPPortList {
		if isValidIP(item.IP) {
			ipsToScan = append(ipsToScan, item.IP)
			ipItemMap[item.IP] = item
		}
	}

	// 先进行Nmap扫描
	var scanResults []*network.ScanResult
	if len(ipsToScan) == 1 {
		// 单个IP使用同步扫描
		scanResults = []*network.ScanResult{network.ScanSingleIP(ipsToScan[0])}
	} else {
		// 多个IP使用并发扫描
		scanResults = network.ScanIPList(ipsToScan)
	}

	// 存储IP+端口数据
	var storedItems []IPPortItem
	var errorItems []IPPortItem

	for _, item := range req.IPPortList {
		// 验证IP格式
		if !isValidIP(item.IP) {
			errorItems = append(errorItems, item)
			continue
		}

		// 存储用户指定的每个端口
		allPortsStored := true
		for _, port := range item.Ports {
			// 插入到表2（假设表名为 ip_ports）
			_, err := db.Exec(`
				INSERT INTO ip_ports (ip_address, port, service, created_at, updated_at)
				VALUES (?, ?, ?, NOW(), NOW())
				ON DUPLICATE KEY UPDATE
					service = VALUES(service),
					updated_at = NOW()
			`, item.IP, port, req.Service)

			if err != nil {
				log.Printf("Failed to store IP-Port %s:%s: %v", item.IP, port, err)
				allPortsStored = false
				break
			}
		}

		// 存储扫描发现的其他开放端口（排除用户已经指定的端口）
		for _, result := range scanResults {
			if result.IP == item.IP {
				for _, port := range result.Ports {
					// 检查是否已经在用户指定的端口列表中
					portExists := false
					for _, userPort := range item.Ports {
						if userPort == port {
							portExists = true
							break
						}
					}

					if !portExists {
						_, err := db.Exec(`
							INSERT INTO ip_ports (ip_address, port, service, created_at, updated_at)
							VALUES (?, ?, 'auto-detected', NOW(), NOW())
							ON DUPLICATE KEY UPDATE
								updated_at = NOW()
						`, item.IP, port)

						if err != nil {
							log.Printf("Failed to store auto-detected port %s for IP %s: %v", port, item.IP, err)
						} else {
							log.Printf("Successfully stored auto-detected port %s for IP %s", port, item.IP)
						}
					}
				}
				break
			}
		}

		if allPortsStored {
			storedItems = append(storedItems, item)
			log.Printf("Successfully stored IP-Ports: %s -> %v", item.IP, item.Ports)
		} else {
			errorItems = append(errorItems, item)
		}
	}

	// 构建MCP响应
	mcpResponse := map[string]interface{}{
		"id": requestID,
		"result": map[string]interface{}{
			"stored_items": storedItems,
			"error_items":  errorItems,
		},
	}

	if len(errorItems) == 0 {
		mcpResponse["result"].(map[string]interface{})["status"] = "success"
		mcpResponse["result"].(map[string]interface{})["message"] = fmt.Sprintf("Successfully stored %d IP-Port combinations", len(storedItems))
	} else if len(storedItems) > 0 {
		mcpResponse["result"].(map[string]interface{})["status"] = "partial_success"
		mcpResponse["result"].(map[string]interface{})["message"] = fmt.Sprintf("Stored %d combinations, failed %d combinations", len(storedItems), len(errorItems))
	} else {
		mcpResponse["result"].(map[string]interface{})["status"] = "error"
		mcpResponse["result"].(map[string]interface{})["message"] = "Failed to store any IP-Port combinations"
		w.WriteHeader(http.StatusInternalServerError)
	}

	json.NewEncoder(w).Encode(mcpResponse)
}

// handleDataQueryStorage 处理数据查询存储
func handleDataQueryStorage(w http.ResponseWriter, args map[string]interface{}, requestID string) {
	// 解析查询参数
	query, ok := args["query"].(string)
	if !ok || query == "" {
		mcpResponse := map[string]interface{}{
			"id": requestID,
			"error": map[string]interface{}{
				"code":    -32602,
				"message": "Query parameter is required and must be a string",
			},
		}
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(mcpResponse)
		return
	}

	// 验证查询语句安全性（基础验证）
	if !isSafeQuery(query) {
		mcpResponse := map[string]interface{}{
			"id": requestID,
			"error": map[string]interface{}{
				"code":    -32602,
				"message": "Unsafe query detected. Only SELECT queries are allowed.",
			},
		}
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(mcpResponse)
		return
	}

	// 连接数据库
	db, err := getStorageboxDB()
	if err != nil {
		log.Printf("Database connection error: %v", err)
		mcpResponse := map[string]interface{}{
			"id": requestID,
			"error": map[string]interface{}{
				"code":    -32603,
				"message": "Database connection failed",
			},
		}
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(mcpResponse)
		return
	}
	defer db.Close()

	// 执行查询
	rows, err := db.Query(query)
	if err != nil {
		log.Printf("Query execution error: %v", err)
		mcpResponse := map[string]interface{}{
			"id": requestID,
			"error": map[string]interface{}{
				"code":    -32603,
				"message": fmt.Sprintf("Query execution failed: %v", err),
			},
		}
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(mcpResponse)
		return
	}
	defer rows.Close()

	// 获取列信息
	columns, err := rows.Columns()
	if err != nil {
		log.Printf("Failed to get columns: %v", err)
		mcpResponse := map[string]interface{}{
			"id": requestID,
			"error": map[string]interface{}{
				"code":    -32603,
				"message": fmt.Sprintf("Failed to get columns: %v", err),
			},
		}
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(mcpResponse)
		return
	}

	// 读取所有行
	var results []map[string]interface{}
	count := 0

	for rows.Next() {
		// 创建值的切片
		values := make([]interface{}, len(columns))
		valuePtrs := make([]interface{}, len(columns))
		for i := range values {
			valuePtrs[i] = &values[i]
		}

		// 扫描行
		if err := rows.Scan(valuePtrs...); err != nil {
			log.Printf("Error scanning row: %v", err)
			continue
		}

		// 构建结果映射
		row := make(map[string]interface{})
		for i, col := range columns {
			val := values[i]
			if val != nil {
				// 处理不同类型的值
				switch v := val.(type) {
				case []byte:
					// 尝试转换为字符串，如果是数字则转换为相应类型
					str := string(v)
					if intVal, err := strconv.Atoi(str); err == nil {
						row[col] = intVal
					} else if floatVal, err := strconv.ParseFloat(str, 64); err == nil {
						row[col] = floatVal
					} else {
						row[col] = str
					}
				default:
					row[col] = v
				}
			} else {
				row[col] = nil
			}
		}

		results = append(results, row)
		count++
	}

	// 检查遍历错误
	if err := rows.Err(); err != nil {
		log.Printf("Error iterating rows: %v", err)
		mcpResponse := map[string]interface{}{
			"id": requestID,
			"error": map[string]interface{}{
				"code":    -32603,
				"message": fmt.Sprintf("Error iterating rows: %v", err),
			},
		}
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(mcpResponse)
		return
	}

	// 构建MCP响应
	mcpResponse := map[string]interface{}{
		"id": requestID,
		"result": map[string]interface{}{
			"data":    results,
			"count":   count,
			"status":  "success",
			"message": fmt.Sprintf("Query executed successfully, returned %d rows", count),
		},
	}

	json.NewEncoder(w).Encode(mcpResponse)
}

// RiskPort 高危端口配置
type RiskPort struct {
	ID          int
	Port        int
	ServiceName string
	RiskLevel   string
	Description string
	IsActive    bool
	Category    string
}

// ScanResult Nmap扫描结果
type ScanResult struct {
	IP     string
	Ports  []string
	Status string // "up", "down", "unknown"
}


// isValidIP 验证IP地址格式（基础验证）
func isValidIP(ip string) bool {
	parts := strings.Split(ip, ".")
	if len(parts) != 4 {
		return false
	}

	for _, part := range parts {
		num, err := strconv.Atoi(part)
		if err != nil || num < 0 || num > 255 {
			return false
		}
	}

	return true
}

