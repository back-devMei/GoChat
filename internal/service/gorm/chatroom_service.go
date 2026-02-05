package gorm

import "gochat/internal/dto/respond"

// chatRoomService 聊天室服务结构体
// 作用：提供聊天室相关的操作方法

type chatRoomService struct {
}

// ChatRoomService 全局聊天室服务实例
// 用途：供其他模块调用聊天室相关功能
var ChatRoomService = new(chatRoomService)

// chatRoomKey 聊天室键结构体
// 用途：作为chatRooms map的键，唯一标识一个聊天室
// ownerId: 聊天室所有者ID
// contactId: 联系人ID（可能是用户ID或群组ID）

type chatRoomKey struct {
	ownerId   string // 聊天室所有者ID
	contactId string // 联系人ID
}

// chatRooms 全局聊天室联系人列表存储
// 类型：map[chatRoomKey][]string，键为聊天室标识，值为联系人ID列表
// 用途：在内存中存储聊天室联系人列表，避免数据库查询，提高性能
// 注意：这是一个内存存储，服务重启后数据会丢失
var chatRooms = make(map[chatRoomKey][]string)

// GetCurContactListInChatRoom 获取当前聊天室联系人列表
// 参数：
//
//	ownerId: 聊天室所有者ID
//	contactId: 联系人ID（用于标识特定聊天室）
//
// 返回值：
//
//	string: 操作结果消息
//	[]respond.GetCurContactListInChatRoomRespond: 联系人列表响应
//	int: 错误码，0表示成功
//
// 功能：
//  1. 根据ownerId和contactId构建聊天室键
//  2. 从内存中的chatRooms map获取对应聊天室的联系人列表
//  3. 将联系人ID转换为响应格式
//  4. 返回操作结果、联系人列表和错误码
//
// 使用场景：
//
//	用户获取当前聊天室的所有成员列表
//
// 设计特点：
//   - 不需要查询数据库，直接从内存获取，提高性能
//   - 适合处理聊天室这种需要实时访问的临时数据
//   - 服务重启后数据会丢失，需要重新构建
func (c *chatRoomService) GetCurContactListInChatRoom(ownerId string, contactId string) (string, []respond.GetCurContactListInChatRoomRespond, int) {
	var rspList []respond.GetCurContactListInChatRoomRespond
	// 遍历对应聊天室的联系人列表
	for _, contactId := range chatRooms[chatRoomKey{ownerId, contactId}] {
		// 将每个联系人ID转换为响应格式
		rspList = append(rspList, respond.GetCurContactListInChatRoomRespond{
			ContactId: contactId,
		})
	}

	// 返回成功消息、联系人列表和错误码0
	return "获取聊天室联系人列表成功", rspList, 0
}
