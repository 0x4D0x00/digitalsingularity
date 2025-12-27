package networkscan

import (
	"database/sql"
	"fmt"
	"log"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"time"

	_ "github.com/go-sql-driver/mysql"
)

// DatabaseConfig 数据库配置结构体
type DatabaseConfig struct {
	Host     string
	Port     string
	User     string
	Password string
	Name     string
}

// StorageboxDBConfig Storagebox数据库配置
var StorageboxDBConfig = DatabaseConfig{
	Host:     "127.0.0.1",
	Port:     "3306",
	User:     "root",
	Password: "XXXXXXXXXXXXXXXXXXXXXXXXXX",
	Name:     "storagebox",
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

// 全局变量缓存高危端口列表
var riskPortsCache []RiskPort
var riskPortsLastUpdate int64

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
	return getDatabaseConnection(StorageboxDBConfig)
}

// loadRiskPorts 加载高危端口配置（带缓存）
func loadRiskPorts() ([]RiskPort, error) {
	// 检查缓存是否有效（5分钟缓存）
	now := time.Now().Unix()
	if riskPortsCache != nil && now-riskPortsLastUpdate < 300 {
		return riskPortsCache, nil
	}

	db, err := getStorageboxDB()
	if err != nil {
		return nil, fmt.Errorf("failed to connect to database: %v", err)
	}
	defer db.Close()

	query := "SELECT id, port, service_name, risk_level, description, is_active, category FROM risk_port WHERE is_active = 1 ORDER BY port"
	rows, err := db.Query(query)
	if err != nil {
		return nil, fmt.Errorf("failed to query risk ports: %v", err)
	}
	defer rows.Close()

	var ports []RiskPort
	for rows.Next() {
		var port RiskPort
		err := rows.Scan(&port.ID, &port.Port, &port.ServiceName, &port.RiskLevel, &port.Description, &port.IsActive, &port.Category)
		if err != nil {
			log.Printf("Error scanning risk port row: %v", err)
			continue
		}
		ports = append(ports, port)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating risk ports: %v", err)
	}

	// 更新缓存
	riskPortsCache = ports
	riskPortsLastUpdate = now

	log.Printf("Loaded %d risk ports from database", len(ports))
	return ports, nil
}

// getScanPorts 获取用于扫描的端口列表字符串
func getScanPorts() (string, error) {
	ports, err := loadRiskPorts()
	if err != nil {
		// 如果数据库查询失败，使用默认端口列表
		log.Printf("Failed to load risk ports from database, using default ports: %v", err)
		return "21,22,23,25,53,80,110,135,139,143,443,445,993,995,1433,1521,3306,3389,5432,6379,8080,8443", nil
	}

	if len(ports) == 0 {
		return "21,22,23,25,53,80,110,135,139,143,443,445,993,995,1433,1521,3306,3389,5432,6379,8080,8443", nil
	}

	var portStrings []string
	for _, port := range ports {
		portStrings = append(portStrings, strconv.Itoa(port.Port))
	}

	return strings.Join(portStrings, ","), nil
}

// scanSingleIP 对单个IP执行Nmap扫描并返回结果
func ScanSingleIP(ip string) *ScanResult {
	result := &ScanResult{IP: ip, Status: "unknown"}

	// 获取动态端口列表
	portsStr, err := getScanPorts()
	if err != nil {
		log.Printf("Failed to get scan ports, using defaults: %v", err)
		portsStr = "21,22,23,25,53,80,110,135,139,143,443,445,993,995,1433,1521,3306,3389,5432,6379,8080,8443"
	}

	// 使用Nmap进行TCP连接扫描，检测高危端口，输出XML格式便于解析
	// -sT (TCP Connect Scan) 不需要root权限
	cmd := exec.Command("nmap", "-sT", "-T4", "--max-retries", "2", "-p", portsStr, "--host-timeout", "30s", "-oX", "-", ip)

	// 获取stdout和stderr
	output, err := cmd.Output()
	if err != nil {
		// 尝试获取stderr信息
		if exitErr, ok := err.(*exec.ExitError); ok {
			stderr := string(exitErr.Stderr)
			log.Printf("Nmap scan failed for IP %s: exit code %d, stderr: %s", ip, exitErr.ExitCode(), stderr)

			// 检查是否是权限问题
			if strings.Contains(stderr, "requires root privileges") || strings.Contains(stderr, "permission denied") {
				log.Printf("Nmap scan failed for IP %s due to insufficient permissions. Need root or cap_net_raw capability.", ip)
				log.Printf("To fix: run 'sudo setcap cap_net_raw+eip ./your_program' on the compiled binary")
			}
		} else {
			log.Printf("Nmap scan failed for IP %s: %v", ip, err)
		}
		return result
	}

	// 解析XML输出，查找开放端口
	outputStr := string(output)
	if strings.Contains(outputStr, `state="up"`) {
		result.Status = "up"

		// 获取所有活跃的端口配置
		riskPorts, err := loadRiskPorts()
		if err != nil {
			log.Printf("Failed to load risk ports for parsing: %v", err)
			return result
		}

		// 动态检查每个配置的端口是否开放
		for _, portConfig := range riskPorts {
			portStr := strconv.Itoa(portConfig.Port)
			portPattern := fmt.Sprintf(`portid="%s"`, portStr)
			if strings.Contains(outputStr, portPattern) && strings.Contains(outputStr, `state="open"`) {
				result.Ports = append(result.Ports, portStr)
			}
		}
	} else {
		result.Status = "down"
	}

	log.Printf("Nmap scan completed for IP %s: status=%s, open_ports=%v", ip, result.Status, result.Ports)
	return result
}

// ScanIPList 并发扫描IP列表并返回结果
func ScanIPList(ips []string) []*ScanResult {
	if len(ips) == 0 {
		return nil
	}

	var wg sync.WaitGroup
	var results []*ScanResult
	var resultsMutex sync.Mutex

	// 限制并发数量为5，避免资源过度消耗
	semaphore := make(chan struct{}, 5)

	for _, ip := range ips {
		wg.Add(1)
		go func(targetIP string) {
			defer wg.Done()

			// 获取信号量
			semaphore <- struct{}{}
			defer func() { <-semaphore }()

			// 执行扫描并获取结果
			result := ScanSingleIP(targetIP)

			// 线程安全地添加到结果列表
			resultsMutex.Lock()
			results = append(results, result)
			resultsMutex.Unlock()
		}(ip)
	}

	wg.Wait()
	log.Printf("Completed Nmap scanning for %d IPs", len(ips))
	return results

}
