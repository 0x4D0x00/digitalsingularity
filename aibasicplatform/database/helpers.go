package database

import (
	"strconv"
)

// getIntValue 从接口值中获取int值（兼容多种数值类型）
func getIntValue(val interface{}) int {
	switch v := val.(type) {
	case int:
		return v
	case int32:
		return int(v)
	case int64:
		return int(v)
	case float64:
		return int(v)
	case string:
		if v == "" {
			return 0
		}
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
		// 尝试解析为浮点数再转换
		if f, err := strconv.ParseFloat(v, 64); err == nil {
			return int(f)
		}
	case []byte:
		s := string(v)
		if s == "" {
			return 0
		}
		if n, err := strconv.Atoi(s); err == nil {
			return n
		}
		if f, err := strconv.ParseFloat(s, 64); err == nil {
			return int(f)
		}
	}
	return 0
}

// getInt64Value 从接口值中获取int64值（兼容多种类型），第二个返回值表示是否成功解析
func getInt64Value(val interface{}) (int64, bool) {
	switch v := val.(type) {
	case int64:
		return v, true
	case int:
		return int64(v), true
	case int32:
		return int64(v), true
	case float64:
		return int64(v), true
	case string:
		if v == "" {
			return 0, false
		}
		if n, err := strconv.ParseInt(v, 10, 64); err == nil {
			return n, true
		}
		if f, err := strconv.ParseFloat(v, 64); err == nil {
			return int64(f), true
		}
	case []byte:
		s := string(v)
		if s == "" {
			return 0, false
		}
		if n, err := strconv.ParseInt(s, 10, 64); err == nil {
			return n, true
		}
		if f, err := strconv.ParseFloat(s, 64); err == nil {
			return int64(f), true
		}
	}
	return 0, false
}


