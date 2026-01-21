// message_type_enum 包定义了消息类型的枚举常量
// 用于管理消息的类型，包括文本、语音、文件和通话
package message_type_enum

const (
	Text         = iota // 文本消息类型
	Voice               // 语音消息类型
	File                // 文件消息类型
	AudioOrVideo        // 通话消息类型
)
