// contact_status_enum 包定义了联系人状态的枚举常量
// 用于管理用户之间的关系状态，包括正常、拉黑、删除、静音等状态
package contact_status_enum

const (
	NORMAL         = iota // 正常状态，表示联系人关系正常
	BE_BLACK              // 被拉黑状态，表示当前用户被对方拉黑
	BLACK                 // 拉黑对方状态，表示当前用户拉黑了对方
	BE_DELETE             // 被删除状态，表示当前用户被对方删除
	DELETE                // 删除对方状态，表示当前用户删除了对方
	SILENCE               // 静音状态，表示对联系人消息进行了静音处理
	QUIT_GROUP            // 退出群组状态，表示用户主动退出了群组
	KICK_OUT_GROUP        // 被踢出群组状态，表示用户被管理员踢出了群组
)
