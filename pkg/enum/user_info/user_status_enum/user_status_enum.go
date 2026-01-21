// user_status_enum 包定义了用户状态的枚举常量
// 用于管理用户的账号状态，包括正常和禁用
package user_status_enum

const (
	NORMAL  = iota // 正常状态，表示用户账号正常可用
	DISABLE        // 禁用状态，表示用户账号已被禁用
)
