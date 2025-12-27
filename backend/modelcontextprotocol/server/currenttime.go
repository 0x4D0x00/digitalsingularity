package server

// 当前时间MCP服务器

import (
	"encoding/json"
	"net/http"
	"time"
)

// CurrentTimeResponse 当前时间响应结构
type CurrentTimeResponse struct {
	Time    string `json:"time"`
	Message string `json:"message"`
}

func CurrentTime(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	response := CurrentTimeResponse{
		Time:    time.Now().Format(time.RFC3339),
		Message: "Current time retrieved successfully",
	}

	json.NewEncoder(w).Encode(response)
}