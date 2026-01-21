// group_status_enum 包定义了群组状态的枚举常量
// 用于管理群组的运行状态，包括正常、禁用和解散
package group_status_enum

const (
	NORMAL   = iota // 正常状态，表示群组正常运行
	DISABLE         // 禁用状态，表示群组已被禁用，无法使用
	DISSOLVE        // 解散状态，表示群组已被解散，无法使用
)
