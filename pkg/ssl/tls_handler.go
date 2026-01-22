// TlsHandler 是一个 Gin 中间件，用于处理 HTTPS 重定向
// 它会检查请求是否通过 HTTPS 访问，如果不是，则会重定向到 HTTPS 地址
// host 是服务器的主机名，port 是服务器的端口号
package ssl

import (
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/unrolled/secure"

	"gochat/pkg/zlog"
)

/* TlsHandler 创建一个 Gin 中间件，用于处理 HTTPS 重定向
 *
 * 参数:
 *	host: 服务器的主机名（域名），如 "example.com"，不是 IP 地址
 *	port: 服务器的 HTTPS 端口号，通常为 443
 * 返回值:
 *	gin.HandlerFunc: 一个 Gin 中间件函数，用于处理 HTTPS 重定向
 *
 * 工作原理:
 *  1. 创建一个 secure 中间件，配置为启用 SSL 重定向
 *  2. 设置 SSLHost 为传入的主机名和端口号
 *  3. 处理当前请求，如果不是 HTTPS 请求，则重定向到 HTTPS 地址
 *  4. 如果处理过程中出现错误，则记录错误并终止请求
 *  5. 如果没有错误，则继续处理请求
 *
 * 使用示例:
 *	router.Use(ssl.TlsHandler("baidu.com", 443))
 */
func TlsHandler(host string, port int) gin.HandlerFunc {
	return func(c *gin.Context) {
		secureMiddleware := secure.New(secure.Options{
			SSLRedirect: true,
			SSLHost:     host + ":" + strconv.Itoa(port),
		})
		err := secureMiddleware.Process(c.Writer, c.Request)

		// 如果处理过程中出现错误，则记录错误并终止请求
		if err != nil {
			zlog.Fatal(err.Error())
			return
		}

		c.Next()
	}
}
