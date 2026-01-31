package respond

// LoadMyJoinedGroupRespond 加载我加入的群聊响应结构体
type LoadMyJoinedGroupRespond struct {
	GroupId   string `json:"group_id"`
	GroupName string `json:"group_name"`
	Avatar    string `json:"avatar"`
}
