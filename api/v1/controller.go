// Package v1 包含API的第一版控制器函数
package v1

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// JsonBack 统一的JSON响应处理函数
// 参数说明:
// - c: Gin框架的上下文对象，用于处理HTTP请求和响应
// - message: 响应消息文本
// - ret: 返回状态码，用于控制响应类型
// - data: 响应数据，可选参数
// 根据不同的ret值返回不同的HTTP响应格式
func JsonBack(c *gin.Context, message string, ret int, data interface{}) {
	switch ret {
	// ret为0表示操作成功
	case 0:
		if data != nil {
			// 如果有数据则返回包含数据的成功响应
			c.JSON(http.StatusOK, gin.H{
				"code":    200,     // HTTP状态码200，业务状态码200表示成功
				"message": message, // 响应消息
				"data":    data,    // 响应数据
			})
		} else {
			// 如果没有数据则返回不含数据的成功响应
			c.JSON(http.StatusOK, gin.H{
				"code":    200,     // HTTP状态码200，业务状态码200表示成功
				"message": message, // 响应消息
			})
		}
	// ret为-2表示客户端错误（如参数错误等）
	case -2:
		c.JSON(http.StatusOK, gin.H{
			"code":    400,     // 业务状态码400表示客户端错误
			"message": message, // 错误消息
		})
	// ret为-1表示服务器错误（如内部异常等）
	case -1:
		c.JSON(http.StatusOK, gin.H{
			"code":    500,     // 业务状态码500表示服务器错误
			"message": message, // 错误消息
		})
	}
}
