// message_status_enum 包定义了消息状态的枚举常量
// 用于管理消息的发送状态，包括未发送和已发送
package message_status_enum

const (
	Unsent = iota // 未发送状态，表示消息未被发送
	Sent          // 已发送状态，表示消息已被发送
)
