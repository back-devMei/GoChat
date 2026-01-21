// contact_apply_status_enum 包定义了联系人申请状态的枚举常量
// 用于管理好友申请的处理状态，包括待处理、同意、拒绝和拉黑
package contact_apply_status_enum

const (
	PENDING = iota // 待处理状态，表示联系人申请正在等待处理
	AGREE          // 同意状态，表示联系人申请被同意
	REFUSE         // 拒绝状态，表示联系人申请被拒绝
	BLACK          // 拉黑状态，表示联系人申请被拉黑
)
