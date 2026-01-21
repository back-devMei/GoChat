// add_mode_enum 包定义了群组添加成员的模式枚举常量
// 用于指定群组添加成员时的操作模式，包括直接添加和审核添加
package add_mode_enum

const (
	DIRECT = iota // 直接添加模式，表示直接将成员添加到群组
	AUDIT         // 审核添加模式，表示需要审核后才能将成员添加到群组
)
