package server

// 当前天气MCP服务器
import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

// CurrentWeatherResponse 当前天气响应结构
type CurrentWeatherResponse struct {
	Weather string `json:"weather"`
	Location string `json:"location,omitempty"`
	Message  string `json:"message"`
}

func CurrentWeather(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	// 获取客户端IP地址作为位置参考
	clientIP := r.RemoteAddr
	if forwarded := r.Header.Get("X-Forwarded-For"); forwarded != "" {
		clientIP = forwarded
	}

	// 尝试从wttr.in获取天气信息
	resp, err := http.Get("http://wttr.in?format=%C+%t")
	if err != nil {
		// 如果获取失败，返回默认响应
		response := CurrentWeatherResponse{
			Weather:  "Unable to fetch weather data",
			Location: "Unknown",
			Message:  "Weather service temporarily unavailable",
		}
		json.NewEncoder(w).Encode(response)
		return
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		response := CurrentWeatherResponse{
			Weather:  "Unable to read weather data",
			Location: "Unknown",
			Message:  "Weather service error",
		}
		json.NewEncoder(w).Encode(response)
		return
	}

	weatherStr := string(body)
	response := CurrentWeatherResponse{
		Weather:  weatherStr,
		Location: fmt.Sprintf("IP: %s", clientIP),
		Message:  "Weather data retrieved successfully",
	}

	json.NewEncoder(w).Encode(response)
}