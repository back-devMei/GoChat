package random

import (
	"math"
	"math/rand"
	"strconv"
	"time"
)

// 预定义常用长度的最小值，提高性能
var minValues = map[int]int{
	1: 0,
	2: 10,
	3: 100,
	4: 1000,
	5: 10000,
	6: 100000,
	7: 1000000,
	8: 10000000,
	9: 100000000,
}

/* GetRandomInt 生成指定长度的随机整数
 * 参数:
 *	len: 要生成的随机数的位数
 * 返回值:
 *	int: 生成的指定长度的随机整数
 * 示例:
 *	GetRandomInt(6) // 返回 100000-999999 之间的随机整数
 */
func GetRandomInt(len int) int {
	// 边界检查
	if len <= 0 {
		return 0
	}
	if len > 9 { // 避免 int 溢出
		len = 9
	}

	// 优先使用预定义的值，提高性能
	if min, ok := minValues[len]; ok {
		return rand.Intn(9*min) + min
	}

	// 对于未预定义的长度，使用原方法
	min := int(math.Pow(10, float64(len-1)))
	return rand.Intn(9*min) + min
}

/* GetNowAndLenRandomString 生成包含当前日期和指定长度随机数的字符串
 * 参数:
 *	len: 随机数部分的位数
 * 返回值:
 *	string: 生成的字符串，格式为 "年月日+随机数"
 * 示例:
 *	GetNowAndLenRandomString(6) // 返回类似 "20260122123456" 的字符串
 */
func GetNowAndLenRandomString(len int) string {
	return time.Now().Format("20060102") + strconv.Itoa(GetRandomInt(len))
}
