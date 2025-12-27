// SilicoID 包入口
// 提供 AI 模型拦截器的便捷访问

package silicoid

import (
	"digitalsingularity/backend/silicoid/interceptor"
)

// Interceptor 是拦截器接口
type Interceptor = interceptor.SilicoIDInterceptor

// CreateInterceptor 创建一个拦截器实例
func CreateInterceptor() *Interceptor {
	return interceptor.CreateInterceptor()
} 