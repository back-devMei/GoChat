package request

// ApplyContactRequest 申请联系人请求结构体
type ApplyContactRequest struct {
	OwnerId   string `json:"owner_id"`   // 操作者的用户ID
	ContactId string `json:"contact_id"` // 联系人ID（用户或群聊）
	Message   string `json:"message"`    // 申请消息
}
