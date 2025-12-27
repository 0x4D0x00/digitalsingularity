package server

import (
	"strings"
)

// isSafeQuery 验证SQL查询是否安全（基础验证）
// 只允许SELECT查询，并禁止危险的关键字
func isSafeQuery(query string) bool {
	upperQuery := strings.ToUpper(strings.TrimSpace(query))

	// 只允许SELECT查询
	if !strings.HasPrefix(upperQuery, "SELECT") {
		return false
	}

	// 禁止危险的关键字
	dangerousKeywords := []string{
		"TRUNCATE", "EXEC", "EXECUTE", "UNION ALL",
		"INTO OUTFILE", "LOAD_FILE", "BENCHMARK",
	}

	for _, keyword := range dangerousKeywords {
		if strings.Contains(upperQuery, keyword) {
			return false
		}
	}

	return true
}
