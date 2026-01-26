// Package sms 提供短信验证码服务，封装了阿里云短信服务的调用逻辑
package sms

import (
	"fmt"
	"gochat/internal/config"
	"gochat/internal/service/redis"
	"gochat/pkg/constants"
	"gochat/pkg/util/random"
	"gochat/pkg/zlog"
	"strconv"
	"time"

	openapi "github.com/alibabacloud-go/darabonba-openapi/v2/client"
	dysmsapi20170525 "github.com/alibabacloud-go/dysmsapi-20170525/v4/client"
	util "github.com/alibabacloud-go/tea-utils/v2/service"
	"github.com/alibabacloud-go/tea/tea"
)

// smsClient 全局短信服务客户端实例，用于复用连接
var smsClient *dysmsapi20170525.Client

// createClient 使用AK&SK初始化账号Client
// 通过配置文件获取阿里云访问凭证，创建短信服务客户端实例
// 返回值:
//   - *dysmsapi20170525.Client: 阿里云短信服务客户端
//   - error: 错误信息，成功时为nil
func createClient() (result *dysmsapi20170525.Client, err error) {
	// 工程代码泄露可能会导致 AccessKey 泄露，并威胁账号下所有资源的安全性。以下代码示例仅供参考。
	// 建议使用更安全的 STS 方式，更多鉴权访问方式请参见：https://help.aliyun.com/document_detail/378661.html。

	// 从配置文件获取阿里云访问密钥
	accessKeyID := config.GetConfig().AuthCodeConfig.AccessKeyID
	accessKeySecret := config.GetConfig().AuthCodeConfig.AccessKeySecret

	// 检查是否已有客户端实例，避免重复创建
	if smsClient == nil {
		// 配置客户端参数
		config := &openapi.Config{
			// 必填，请确保代码运行环境设置了环境变量 ALIBABA_CLOUD_ACCESS_KEY_ID。
			AccessKeyId: tea.String(accessKeyID),
			// 必填，请确保代码运行环境设置了环境变量 ALIBABA_CLOUD_ACCESS_KEY_SECRET。
			AccessKeySecret: tea.String(accessKeySecret),
		}
		// Endpoint 请参考 https://api.aliyun.com/product/Dysmsapi
		config.Endpoint = tea.String("dysmsapi.aliyuncs.com")

		// 创建短信服务客户端
		smsClient, err = dysmsapi20170525.NewClient(config)
	}

	// 返回客户端实例和错误信息
	return smsClient, err
}

// VerificationCode 发送短信验证码
// 为指定手机号生成并发送验证码，使用Redis缓存防止频繁发送
// 参数:
//   - telephone: 目标手机号码
//
// 返回值:
//   - string: 操作结果消息(成功或失败的具体描述)
//   - int: 操作状态码(0表示成功，负数表示不同类型的错误)
func VerificationCode(telephone string) (string, int) {
	// 创建阿里云短信服务客户端
	client, err := createClient()
	if err != nil {
		zlog.Error(err.Error())
		return constants.SYSTEM_ERROR, -1
	}

	// 构建Redis键名，用于存储验证码
	key := "auth_code_" + telephone

	// 从Redis中获取已存在的验证码
	code, err := redis.GetKey(key)
	if err != nil {
		zlog.Error(err.Error())
		return constants.SYSTEM_ERROR, -1
	}

	// 检查是否已存在未过期的验证码
	if code != "" {
		// 验证码未过期，不允许重复发送
		message := "目前还不能发送验证码，请输入已发送的验证码"
		zlog.Info(message)
		return message, -2
	}

	// 验证码已过期或不存在，生成新的6位数字验证码
	code = strconv.Itoa(random.GetRandomInt(6))
	fmt.Println(code) // 用于调试，打印生成的验证码

	// 将新验证码存储到Redis，设置1分钟有效期
	err = redis.SetKeyEx(key, code, time.Minute) // 1分钟有效
	if err != nil {
		zlog.Error(err.Error())
		return constants.SYSTEM_ERROR, -1
	}

	// 构建发送短信请求参数
	sendSmsRequest := &dysmsapi20170525.SendSmsRequest{
		SignName:      tea.String("阿里云短信测试"),                     // 短信签名名称
		TemplateCode:  tea.String("SMS_154950909"),               // 短信模板CODE
		PhoneNumbers:  tea.String(telephone),                     // 接收短信的手机号
		TemplateParam: tea.String("{\"code\":\"" + code + "\"}"), // 短信模板变量，将验证码嵌入JSON格式
	}

	// 设置运行时参数
	runtime := &util.RuntimeOptions{}

	// 目前使用的是测试专用签名，签名必须是"阿里云短信测试"，模板code为"SMS_154950909"
	// 发送短信验证码
	rsp, err := client.SendSmsWithOptions(sendSmsRequest, runtime)
	if err != nil {
		zlog.Error(err.Error())
		return constants.SYSTEM_ERROR, -1
	}

	// 记录发送响应信息
	zlog.Info(*util.ToJSONString(rsp))

	// 返回成功消息
	return "验证码发送成功，请及时在对应电话查收短信", 0
}
