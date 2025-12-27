package server

// 数据读取MCP服务器
import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strconv"

	_ "github.com/go-sql-driver/mysql"
)


// StorageboxDataReadingResponse Storagebox数据读取响应结构
type StorageboxDataReadingResponse struct {
	Data    []map[string]interface{} `json:"data"`
	Status  string                   `json:"status"`
	Message string                   `json:"message"`
	Count   int                      `json:"count,omitempty"`
}

// StorageboxDataReading 从Storagebox数据库读取数据
func StorageboxDataReading(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	// 获取查询参数
	query := r.URL.Query().Get("query")
	if query == "" {
		response := StorageboxDataReadingResponse{
			Data:    nil,
			Status:  "error",
			Message: "Query parameter 'query' is required",
		}
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(response)
		return
	}

	// 验证查询语句安全性（基础验证）
	if !isSafeQuery(query) {
		response := StorageboxDataReadingResponse{
			Data:    nil,
			Status:  "error",
			Message: "Unsafe query detected. Only SELECT queries are allowed.",
		}
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(response)
		return
	}

	// 连接数据库
	db, err := getStorageboxDB()
	if err != nil {
		response := StorageboxDataReadingResponse{
			Data:    nil,
			Status:  "error",
			Message: fmt.Sprintf("Failed to connect to database: %v", err),
		}
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(response)
		return
	}
	defer db.Close()

	// 执行查询
	rows, err := db.Query(query)
	if err != nil {
		response := StorageboxDataReadingResponse{
			Data:    nil,
			Status:  "error",
			Message: fmt.Sprintf("Failed to execute query: %v", err),
		}
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(response)
		return
	}
	defer rows.Close()

	// 获取列信息
	columns, err := rows.Columns()
	if err != nil {
		response := StorageboxDataReadingResponse{
			Data:    nil,
			Status:  "error",
			Message: fmt.Sprintf("Failed to get columns: %v", err),
		}
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(response)
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
		response := StorageboxDataReadingResponse{
			Data:    nil,
			Status:  "error",
			Message: fmt.Sprintf("Error iterating rows: %v", err),
		}
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(response)
		return
	}

	// 返回成功响应
	response := StorageboxDataReadingResponse{
		Data:    results,
		Status:  "success",
		Message: "Data retrieved successfully",
		Count:   count,
	}

	json.NewEncoder(w).Encode(response)
}
