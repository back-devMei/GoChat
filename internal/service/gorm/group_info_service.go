package gorm

import (
	"encoding/json"
	"errors"
	"fmt"
	"gochat/internal/dao"
	"gochat/internal/dto/request"
	"gochat/internal/dto/respond"
	"gochat/internal/model"
	myredis "gochat/internal/service/redis"
	"gochat/pkg/constants"
	"gochat/pkg/enum/contact/contact_status_enum"
	"gochat/pkg/enum/contact/contact_type_enum"
	"gochat/pkg/enum/group_info/group_status_enum"
	"gochat/pkg/util/random"
	"gochat/pkg/zlog"
	"log"
	"time"

	"github.com/go-redis/redis/v8"
	"gorm.io/gorm"
)

type groupInfoService struct {
}

var GroupInfoService = new(groupInfoService)

// SaveGroup 保存群聊
//func (g *groupInfoService) SaveGroup(groupReq request.SaveGroupRequest) error {
//	var group model.GroupInfo
//	res := dao.GormDB.First(&group, "uuid = ?", groupReq.Uuid)
//	if res.Error != nil {
//		if errors.Is(res.Error, gorm.ErrRecordNotFound) {
//			// 创建群聊
//			group.Uuid = groupReq.Uuid
//			group.Name = groupReq.Name
//			group.Notice = groupReq.Notice
//			group.AddMode = groupReq.AddMode
//			group.Avatar = groupReq.Avatar
//			group.MemberCnt = 1
//			group.Members = append(group.Members, groupReq.OwnerId)
//			group.OwnerId = groupReq.OwnerId
//			group.CreatedAt = time.Now()
//			group.UpdatedAt = time.Now()
//			if res := dao.GormDB.Create(&group); res.Error != nil {
//				zlog.Error(res.Error.Error())
//				return res.Error
//			}
//			return nil
//		} else {
//			zlog.Error(res.Error.Error())
//			return res.Error
//		}
//	}
//	// 更新群聊
//	group.Uuid = groupReq.Uuid
//	group.Name = groupReq.Name
//	group.Notice = groupReq.Notice
//	group.AddMode = groupReq.AddMode
//	group.Avatar = groupReq.Avatar
//	group.MemberCnt = 1
//	group.Members = append(group.Members, groupReq.OwnerId)
//	group.OwnerId = groupReq.OwnerId
//	group.CreatedAt = time.Now()
//	group.UpdatedAt = time.Now()
//	return nil
//}

// CreateGroup 创建群聊
// 创建一个新的群聊，包含群信息初始化和群主加入群聊的联系人记录
// 参数: groupReq - 包含群聊创建所需信息的请求对象
// 返回值:
//   - string: 操作结果消息，成功或失败的具体描述
//   - int: 状态码，0表示成功，负数表示错误
func (g *groupInfoService) CreateGroup(groupReq request.CreateGroupRequest) (string, int) {
	// 初始化群聊信息对象，设置群聊基本属性
	group := model.GroupInfo{
		Uuid:      fmt.Sprintf("G%s", random.GetNowAndLenRandomString(11)), // 生成群聊唯一标识符，以"G"开头
		Name:      groupReq.Name,                                           // 群聊名称
		Notice:    groupReq.Notice,                                         // 群公告
		OwnerId:   groupReq.OwnerId,                                        // 群主ID
		MemberCnt: 1,                                                       // 初始成员数量为1（群主）
		AddMode:   groupReq.AddMode,                                        // 加群模式
		Avatar:    groupReq.Avatar,                                         // 群头像
		Status:    group_status_enum.NORMAL,                                // 群状态为正常
		CreatedAt: time.Now(),                                              // 创建时间
		UpdatedAt: time.Now(),                                              // 更新时间
	}

	// 初始化群成员列表，将群主作为第一个成员
	var members []string
	members = append(members, groupReq.OwnerId)

	// 将成员列表序列化为JSON格式存储
	var err error
	group.Members, err = json.Marshal(members)
	if err != nil {
		// 序列化失败，记录错误日志并返回系统错误
		zlog.Error(err.Error())
		return constants.SYSTEM_ERROR, -1
	}

	// 将群聊信息保存到数据库
	if res := dao.GormDB.Create(&group); res.Error != nil {
		// 数据库保存失败，记录错误日志并返回系统错误
		zlog.Error(res.Error.Error())
		return constants.SYSTEM_ERROR, -1
	}

	// 创建群主加入群聊的联系人记录，使群聊出现在群主的群聊列表中
	contact := model.UserContact{
		UserId:      groupReq.OwnerId,           // 用户ID（群主）
		ContactId:   group.Uuid,                 // 联系人ID（群聊UUID）
		ContactType: contact_type_enum.GROUP,    // 联系人类型为群聊
		Status:      contact_status_enum.NORMAL, // 状态为正常
		CreatedAt:   time.Now(),                 // 创建时间
		UpdatedAt:   time.Now(),                 // 更新时间
	}

	// 将联系人记录保存到数据库
	if res := dao.GormDB.Create(&contact); res.Error != nil {
		// 数据库保存失败，记录错误日志并返回系统错误
		zlog.Error(res.Error.Error())
		return constants.SYSTEM_ERROR, -1
	}

	// 清除群主的群聊列表缓存，确保新创建的群聊能及时显示
	if err := myredis.DelKeysWithPattern("contact_mygroup_list_" + groupReq.OwnerId); err != nil {
		// 缓存清除失败，记录错误日志（不影响主要流程）
		zlog.Error(err.Error())
	}

	// 返回创建成功的消息
	return "创建成功", 0
}

// LoadMyGroup 获取我创建的群聊
// 根据用户ID获取该用户创建的群聊列表，优先从Redis缓存获取，若缓存不存在则从数据库查询
// 参数: ownerId - 群主用户ID
// 返回值:
//   - string: 操作结果消息，成功或失败的具体描述
//   - []respond.LoadMyGroupRespond: 群聊信息列表响应对象
//   - int: 状态码，0表示成功，负数表示错误
func (g *groupInfoService) LoadMyGroup(ownerId string) (string, []respond.LoadMyGroupRespond, int) {
	// 尝试从Redis缓存中获取用户群聊列表
	rspString, err := myredis.GetKeyNilIsErr("contact_mygroup_list_" + ownerId)
	// 检查从Redis获取数据是否出错
	if err != nil {
		// 如果错误是"键不存在"（redis.Nil），说明缓存中没有该用户群聊列表
		if errors.Is(err, redis.Nil) {
			// 从数据库中查询该用户创建的所有群聊，按创建时间倒序排列
			var groupList []model.GroupInfo
			if res := dao.GormDB.Order("created_at DESC").Where("owner_id = ?", ownerId).Find(&groupList); res.Error != nil {
				// 数据库查询失败，记录错误并返回系统错误
				zlog.Error(res.Error.Error())
				return constants.SYSTEM_ERROR, nil, -1
			}

			// 构造群聊信息响应对象列表
			var groupListRsp []respond.LoadMyGroupRespond
			// 遍历查询到的群聊列表，为每个群聊构建响应对象
			for _, group := range groupList {
				// 创建单个群聊的响应对象
				groupListRsp = append(groupListRsp, respond.LoadMyGroupRespond{
					GroupId:   group.Uuid,   // 群聊UUID
					GroupName: group.Name,   // 群聊名称
					Avatar:    group.Avatar, // 群头像
				})
			}

			// 将响应对象列表序列化为JSON字符串
			rspString, err := json.Marshal(groupListRsp)
			if err != nil {
				// 序列化失败，记录错误
				zlog.Error(err.Error())
			}

			// 将序列化的群聊列表存入Redis缓存，设置过期时间
			if err := myredis.SetKeyEx("contact_mygroup_list_"+ownerId, string(rspString), time.Minute*constants.REDIS_TIMEOUT); err != nil {
				// 缓存存储失败，记录错误（不影响主要流程）
				zlog.Error(err.Error())
			}

			// 返回数据库查询结果
			return "获取成功", groupListRsp, 0
		} else {
			// 如果是其他Redis错误（不仅仅是键不存在），记录错误并返回系统错误
			zlog.Error(err.Error())
			return constants.SYSTEM_ERROR, nil, -1
		}
	}

	// 如果Redis中存在缓存数据，则将JSON字符串反序列化为响应对象列表
	var groupListRsp []respond.LoadMyGroupRespond
	if err := json.Unmarshal([]byte(rspString), &groupListRsp); err != nil {
		// JSON反序列化失败，记录错误
		zlog.Error(err.Error())
	}

	// 返回缓存中的群聊列表
	return "获取成功", groupListRsp, 0
}

// CheckGroupAddMode 检查群聊加入方式
// 根据群聊ID获取群聊的加入方式设置，优先从Redis缓存获取，若缓存不存在则从数据库查询
// 参数: groupId - 群聊唯一标识符
// 返回值:
//   - string: 操作结果消息，成功或失败的具体描述
//   - int8: 群聊加入方式（如允许任何人加入、需验证等）
//   - int: 状态码，0表示成功，负数表示错误
func (g *groupInfoService) CheckGroupAddMode(groupId string) (string, int8, int) {
	// 尝试从Redis缓存中获取群聊信息
	rspString, err := myredis.GetKeyNilIsErr("group_info_" + groupId)
	// 检查从Redis获取数据是否出错
	if err != nil {
		// 如果错误是"键不存在"（redis.Nil），说明缓存中没有该群聊信息
		if errors.Is(err, redis.Nil) {
			// 从数据库中查询群聊信息
			var group model.GroupInfo
			if res := dao.GormDB.First(&group, "uuid = ?", groupId); res.Error != nil {
				// 数据库查询失败，记录错误日志并返回系统错误
				zlog.Error(res.Error.Error())
				return constants.SYSTEM_ERROR, -1, -1
			}

			// 返回数据库查询到的群聊加入方式
			return "加群方式获取成功", group.AddMode, 0
		} else {
			// 如果是其他Redis错误（不仅仅是键不存在），记录错误并返回系统错误
			zlog.Error(err.Error())
			return constants.SYSTEM_ERROR, -1, -1
		}
	}

	// 如果Redis中存在缓存数据，则将JSON字符串反序列化为响应对象
	var rsp respond.GetGroupInfoRespond
	if err := json.Unmarshal([]byte(rspString), &rsp); err != nil {
		// JSON反序列化失败，记录错误
		zlog.Error(err.Error())
	}

	// 返回缓存中的群聊加入方式
	return "加群方式获取成功", rsp.AddMode, 0
}

// EnterGroupDirectly 直接进群
// 让指定用户直接加入群聊，适用于允许任何人加入的群聊
// 参数: ownerId - 群聊UUID
// 参数: contactId - 要加入群聊的用户UUID
// 返回值:
//   - string: 操作结果消息，成功或失败的具体描述
//   - int: 状态码，0表示成功，负数表示错误
func (g *groupInfoService) EnterGroupDirectly(ownerId, contactId string) (string, int) {
	// 查询群聊信息
	var group model.GroupInfo
	if res := dao.GormDB.First(&group, "uuid = ?", ownerId); res.Error != nil {
		// 数据库查询失败，记录错误日志并返回系统错误
		zlog.Error(res.Error.Error())
		return constants.SYSTEM_ERROR, -1
	}

	// 反序列化群成员列表
	var members []string
	if err := json.Unmarshal(group.Members, &members); err != nil {
		// 反序列化失败，记录错误日志并返回系统错误
		zlog.Error(err.Error())
		return constants.SYSTEM_ERROR, -1
	}

	// 将新用户添加到群成员列表
	members = append(members, contactId)

	// 重新序列化群成员列表
	if data, err := json.Marshal(members); err != nil {
		// 序列化失败，记录错误日志并返回系统错误
		zlog.Error(err.Error())
		return constants.SYSTEM_ERROR, -1
	} else {
		// 更新群聊的成员列表
		group.Members = data
	}

	// 增加群成员计数
	group.MemberCnt += 1

	// 保存更新后的群聊信息
	if res := dao.GormDB.Save(&group); res.Error != nil {
		// 数据库保存失败，记录错误日志并返回系统错误
		zlog.Error(res.Error.Error())
		return constants.SYSTEM_ERROR, -1
	}

	// 创建用户与群聊的联系人记录，使群聊出现在用户的群聊列表中
	newContact := model.UserContact{
		UserId:      contactId,                  // 用户ID
		ContactId:   ownerId,                    // 联系人ID（群聊UUID）
		ContactType: contact_type_enum.GROUP,    // 联系人类型为群聊
		Status:      contact_status_enum.NORMAL, // 状态为正常
		CreatedAt:   time.Now(),                 // 创建时间
		UpdatedAt:   time.Now(),                 // 更新时间
	}

	// 将联系人记录保存到数据库
	if res := dao.GormDB.Create(&newContact); res.Error != nil {
		// 数据库保存失败，记录错误日志并返回系统错误
		zlog.Error(res.Error.Error())
		return constants.SYSTEM_ERROR, -1
	}

	// 清除相关缓存，确保群聊成员列表和用户群聊列表及时更新
	// TODO: 以下缓存清除逻辑可根据实际需要启用
	if err := myredis.DelKeysWithPattern("group_info_" + contactId); err != nil {
		zlog.Error(err.Error())
	}
	if err := myredis.DelKeysWithPattern("groupmember_list_" + contactId); err != nil {
		zlog.Error(err.Error())
	}

	// 清除群聊会话列表缓存
	if err := myredis.DelKeysWithPattern("group_session_list_" + ownerId); err != nil {
		// 缓存清除失败，记录错误日志（不影响主要流程）
		zlog.Error(err.Error())
	}

	// 清除用户加入的群聊列表缓存
	if err := myredis.DelKeysWithPattern("my_joined_group_list_" + ownerId); err != nil {
		// 缓存清除失败，记录错误日志（不影响主要流程）
		zlog.Error(err.Error())
	}

	// TODO: 如需要，可取消下面的注释来清除会话缓存
	if err := myredis.DelKeysWithPattern("session_" + ownerId + "_" + contactId); err != nil {
		zlog.Error(err.Error())
	}

	// 返回进群成功的消息
	return "进群成功", 0
}

// LeaveGroup 退群
// 让指定用户退出群聊，处理相关的数据清理和状态更新
// 参数: userId - 退群用户的UUID
// 参数: groupId - 要退出的群聊UUID
// 返回值:
//   - string: 操作结果消息，成功或失败的具体描述
//   - int: 状态码，0表示成功，负数表示错误
func (g *groupInfoService) LeaveGroup(userId, groupId string) (string, int) {
	// 查询群聊信息
	var group model.GroupInfo
	if res := dao.GormDB.First(&group, "uuid = ?", groupId); res.Error != nil {
		// 数据库查询失败，记录错误日志并返回系统错误
		zlog.Error(res.Error.Error())
		return constants.SYSTEM_ERROR, -1
	}

	// 反序列化群成员列表
	var members []string
	if err := json.Unmarshal(group.Members, &members); err != nil {
		// 反序列化失败，记录错误日志并返回系统错误
		zlog.Error(err.Error())
		return constants.SYSTEM_ERROR, -1
	}

	// 从群成员列表中移除退群用户
	for i, member := range members {
		if member == userId {
			// 使用切片操作移除用户，保持其他成员不变
			members = append(members[:i], members[i+1:]...)
			break
		}
	}

	// 重新序列化群成员列表并更新群聊信息
	if data, err := json.Marshal(members); err != nil {
		// 序列化失败，记录错误日志并返回系统错误
		zlog.Error(err.Error())
		return constants.SYSTEM_ERROR, -1
	} else {
		// 更新群聊的成员列表
		group.Members = data
	}

	// 减少群成员计数
	group.MemberCnt -= 1

	// 保存更新后的群聊信息
	if res := dao.GormDB.Save(&group); res.Error != nil {
		// 数据库保存失败，记录错误日志并返回系统错误
		zlog.Error(res.Error.Error())
		return constants.SYSTEM_ERROR, -1
	}

	// 创建软删除的时间戳
	var deletedAt gorm.DeletedAt
	deletedAt.Time = time.Now()
	deletedAt.Valid = true
	// 软删除该用户与群聊之间的会话记录
	if res := dao.GormDB.Model(&model.Session{}).Where("send_id = ? AND receive_id = ?", userId, groupId).Update("deleted_at", deletedAt); res.Error != nil {
		// 会话删除失败，记录错误日志并返回系统错误
		zlog.Error(res.Error.Error())
		return constants.SYSTEM_ERROR, -1
	}

	// 更新用户与群聊的联系人记录状态为退群，并软删除该记录
	if res := dao.GormDB.Model(&model.UserContact{}).Where("user_id = ? AND contact_id = ?", userId, groupId).Updates(map[string]interface{}{
		"deleted_at": deletedAt,
		"status":     contact_status_enum.QUIT_GROUP, // 更新状态为退群
	}); res.Error != nil {
		// 联系人记录更新失败，记录错误日志并返回系统错误
		zlog.Error(res.Error.Error())
		return constants.SYSTEM_ERROR, -1
	}

	// 软删除相关的联系人申请记录
	if res := dao.GormDB.Model(&model.ContactApply{}).Where("contact_id = ? AND user_id = ?", groupId, userId).Update("deleted_at", deletedAt); res.Error != nil {
		// 申请记录删除失败，记录错误日志并返回系统错误
		zlog.Error(res.Error.Error())
		return constants.SYSTEM_ERROR, -1
	}

	// 清除相关缓存，确保群聊成员列表和用户群聊列表及时更新
	// TODO: 以下缓存清除逻辑可根据实际需要启用
	if err := myredis.DelKeysWithPattern("group_info_" + groupId); err != nil {
		zlog.Error(err.Error())
	}
	if err := myredis.DelKeysWithPattern("groupmember_list_" + groupId); err != nil {
		zlog.Error(err.Error())
	}

	// 清除用户群聊会话列表缓存
	if err := myredis.DelKeysWithPattern("group_session_list_" + userId); err != nil {
		// 缓存清除失败，记录错误日志（不影响主要流程）
		zlog.Error(err.Error())
	}

	// 清除用户加入的群聊列表缓存
	if err := myredis.DelKeysWithPattern("my_joined_group_list_" + userId); err != nil {
		// 缓存清除失败，记录错误日志（不影响主要流程）
		zlog.Error(err.Error())
	}

	// TODO: 如需要，可取消下面的注释来清除会话缓存
	if err := myredis.DelKeysWithPattern("session_" + userId + "_" + groupId); err != nil {
		zlog.Error(err.Error())
	}

	// 返回退群成功的消息
	return "退群成功", 0
}

// DismissGroup 解散群聊
// 由群主解散群聊，软删除群聊及相关数据（会话、联系人、申请记录）
// 参数: ownerId - 群主用户UUID
// 参数: groupId - 要解散的群聊UUID
// 返回值:
//   - string: 操作结果消息，成功或失败的具体描述
//   - int: 状态码，0表示成功，负数表示错误
func (g *groupInfoService) DismissGroup(ownerId, groupId string) (string, int) {
	// 创建软删除的时间戳
	var deletedAt gorm.DeletedAt
	deletedAt.Time = time.Now()
	deletedAt.Valid = true

	// 软删除群聊记录，标记为已删除
	if res := dao.GormDB.Model(&model.GroupInfo{}).Where("uuid = ?", groupId).Updates(
		map[string]interface{}{
			"deleted_at": deletedAt,      // 设置删除时间
			"updated_at": deletedAt.Time, // 更新时间戳
		}); res.Error != nil {
		// 群聊记录更新失败，记录错误日志并返回系统错误
		zlog.Error(res.Error.Error())
		return constants.SYSTEM_ERROR, -1
	}

	// 查询与该群聊相关的所有会话记录
	var sessionList []model.Session
	if res := dao.GormDB.Model(&model.Session{}).Where("receive_id = ?", groupId).Find(&sessionList); res.Error != nil {
		// 会话记录查询失败，记录错误日志并返回系统错误
		zlog.Error(res.Error.Error())
		return constants.SYSTEM_ERROR, -1
	}

	// 软删除所有与该群聊相关的会话记录
	for _, session := range sessionList {
		if res := dao.GormDB.Model(&session).Updates(
			map[string]interface{}{
				"deleted_at": deletedAt, // 设置删除时间
			}); res.Error != nil {
			// 会话记录更新失败，记录错误日志并返回系统错误
			zlog.Error(res.Error.Error())
			return constants.SYSTEM_ERROR, -1
		}
	}

	// 查询与该群聊相关的所有用户联系人记录
	var userContactList []model.UserContact
	if res := dao.GormDB.Model(&model.UserContact{}).Where("contact_id = ?", groupId).Find(&userContactList); res.Error != nil {
		// 用户联系人记录查询失败，记录错误日志并返回系统错误
		zlog.Error(res.Error.Error())
		return constants.SYSTEM_ERROR, -1
	}

	// 软删除所有与该群聊相关的用户联系人记录
	for _, userContact := range userContactList {
		if res := dao.GormDB.Model(&userContact).Update("deleted_at", deletedAt); res.Error != nil {
			// 用户联系人记录更新失败，记录错误日志并返回系统错误
			zlog.Error(res.Error.Error())
			return constants.SYSTEM_ERROR, -1
		}
	}

	// 查询与该群聊相关的所有联系人申请记录
	var contactApplys []model.ContactApply
	if res := dao.GormDB.Model(&contactApplys).Where("contact_id = ?", groupId).Find(&contactApplys); res.Error != nil {
		// 如果没有找到相关申请记录，返回成功消息
		if res.Error != gorm.ErrRecordNotFound {
			zlog.Info(res.Error.Error())
			return "无响应的申请记录需要删除", 0
		}
		// 其他错误情况，记录错误日志并返回系统错误
		zlog.Error(res.Error.Error())
		return constants.SYSTEM_ERROR, -1
	}

	// 软删除所有与该群聊相关的联系人申请记录
	for _, contactApply := range contactApplys {
		if res := dao.GormDB.Model(&contactApply).Update("deleted_at", deletedAt); res.Error != nil {
			// 联系人申请记录更新失败，记录错误日志并返回系统错误
			zlog.Error(res.Error.Error())
			return constants.SYSTEM_ERROR, -1
		}
	}

	// 清除相关缓存，确保群聊列表和会话列表及时更新
	// TODO: 以下缓存清除逻辑可根据实际需要启用
	if err := myredis.DelKeysWithPattern("group_info_" + groupId); err != nil {
		zlog.Error(err.Error())
	}
	if err := myredis.DelKeysWithPattern("groupmember_list_" + groupId); err != nil {
		zlog.Error(err.Error())
	}

	// 清除群主的群聊列表缓存
	if err := myredis.DelKeysWithPattern("contact_mygroup_list_" + ownerId); err != nil {
		// 缓存清除失败，记录错误日志（不影响主要流程）
		zlog.Error(err.Error())
	}

	// 清除群主的群聊会话列表缓存
	if err := myredis.DelKeysWithPattern("group_session_list_" + ownerId); err != nil {
		// 缓存清除失败，记录错误日志（不影响主要流程）
		zlog.Error(err.Error())
	}

	// 清除所有用户加入的群聊列表缓存
	if err := myredis.DelKeysWithPrefix("my_joined_group_list"); err != nil {
		// 缓存清除失败，记录错误日志（不影响主要流程）
		zlog.Error(err.Error())
	}

	// 返回解散群聊成功的消息
	return "解散群聊成功", 0
}

// GetGroupInfo 获取群聊详情
// 根据群聊ID获取群聊的详细信息，优先从Redis缓存获取，若缓存不存在则从数据库查询
// 参数: groupId - 群聊UUID
// 返回值:
//   - string: 操作结果消息，成功或失败的具体描述
//   - *respond.GetGroupInfoRespond: 群聊信息响应对象指针
//   - int: 状态码，0表示成功，负数表示错误
func (g *groupInfoService) GetGroupInfo(groupId string) (string, *respond.GetGroupInfoRespond, int) {
	// 尝试从Redis缓存中获取群聊信息
	rspString, err := myredis.GetKeyNilIsErr("group_info_" + groupId)
	// 检查从Redis获取数据是否出错
	if err != nil {
		// 如果错误是"键不存在"（redis.Nil），说明缓存中没有该群聊信息
		if errors.Is(err, redis.Nil) {
			// 从数据库中查询群聊信息
			var group model.GroupInfo
			if res := dao.GormDB.First(&group, "uuid = ?", groupId); res.Error != nil {
				// 数据库查询失败，记录错误日志并返回系统错误
				zlog.Error(res.Error.Error())
				return constants.SYSTEM_ERROR, nil, -1
			}

			// 构造群聊信息响应对象
			rsp := &respond.GetGroupInfoRespond{
				Uuid:      group.Uuid,      // 群聊UUID
				Name:      group.Name,      // 群聊名称
				Notice:    group.Notice,    // 群公告
				Avatar:    group.Avatar,    // 群头像
				MemberCnt: group.MemberCnt, // 群成员数量
				OwnerId:   group.OwnerId,   // 群主ID
				AddMode:   group.AddMode,   // 加群方式
				Status:    group.Status,    // 群状态
			}

			// 检查群聊是否已被软删除
			if group.DeletedAt.Valid {
				// 如果群聊已被删除，设置IsDeleted为true
				rsp.IsDeleted = true
			} else {
				// 如果群聊未被删除，设置IsDeleted为false
				rsp.IsDeleted = false
			}

			// TODO: 如果需要，可以将响应对象序列化并存入Redis缓存
			rspString, err := json.Marshal(rsp)
			if err != nil {
				zlog.Error(err.Error())
			}
			if err := myredis.SetKeyEx("group_info_"+groupId, string(rspString), time.Minute*constants.REDIS_TIMEOUT); err != nil {
				zlog.Error(err.Error())
			}

			// 返回数据库查询到的群聊信息
			return "获取成功", rsp, 0
		} else {
			// 如果是其他Redis错误（不仅仅是键不存在），记录错误并返回系统错误
			zlog.Error(err.Error())
			return constants.SYSTEM_ERROR, nil, -1
		}
	}

	// 如果Redis中存在缓存数据，则将JSON字符串反序列化为响应对象
	var rsp *respond.GetGroupInfoRespond
	if err := json.Unmarshal([]byte(rspString), &rsp); err != nil {
		// JSON反序列化失败，记录错误
		zlog.Error(err.Error())
	}

	// 返回缓存中的群聊信息
	return "获取成功", rsp, 0
}

// UpdateGroupInfo 更新群聊信息
// 根据请求参数更新群聊的基本信息（名称、加群方式、公告、头像），并同步更新相关的会话信息
// 参数: req - 更新群聊信息的请求对象
// 返回值:
//   - string: 操作结果消息，成功或失败的具体描述
//   - int: 状态码，0表示成功，负数表示错误
func (g *groupInfoService) UpdateGroupInfo(req request.UpdateGroupInfoRequest) (string, int) {
	// 查询要更新的群聊信息
	var group model.GroupInfo
	if res := dao.GormDB.First(&group, "uuid = ?", req.Uuid); res.Error != nil {
		// 数据库查询失败，记录错误日志并返回系统错误
		zlog.Error(res.Error.Error())
		return constants.SYSTEM_ERROR, -1
	}

	// 检查并更新群聊名称（如果提供了新名称）
	if req.Name != "" {
		group.Name = req.Name
	}

	// 检查并更新群聊加群方式（如果提供了新值，且不为-1）
	if req.AddMode != -1 {
		group.AddMode = req.AddMode
	}

	// 检查并更新群聊公告（如果提供了新公告）
	if req.Notice != "" {
		group.Notice = req.Notice
	}

	// 检查并更新群聊头像（如果提供了新头像）
	if req.Avatar != "" {
		group.Avatar = req.Avatar
	}

	// 保存更新后的群聊信息到数据库
	if res := dao.GormDB.Save(&group); res.Error != nil {
		// 数据库保存失败，记录错误日志并返回系统错误
		zlog.Error(res.Error.Error())
		return constants.SYSTEM_ERROR, -1
	}

	// 更新与该群聊相关的会话信息，确保会话中的群聊名称和头像与群聊信息保持一致
	var sessionList []model.Session
	if res := dao.GormDB.Where("receive_id = ?", req.Uuid).Find(&sessionList); res.Error != nil {
		// 会话记录查询失败，记录错误日志并返回系统错误
		zlog.Error(res.Error.Error())
		return constants.SYSTEM_ERROR, -1
	}

	// 遍历所有相关会话，更新会话中的群聊名称和头像
	for _, session := range sessionList {
		session.ReceiveName = group.Name // 更新接收方名称为新的群聊名称
		session.Avatar = group.Avatar    // 更新头像为新的群聊头像
		log.Println(session)             // 打印会话信息（调试用）
		// 保存更新后的会话信息
		if res := dao.GormDB.Save(&session); res.Error != nil {
			// 会话记录保存失败，记录错误日志并返回系统错误
			zlog.Error(res.Error.Error())
			return constants.SYSTEM_ERROR, -1
		}
	}

	// TODO: 可考虑清除相关缓存，如群聊信息缓存等
	if err := myredis.DelKeysWithPattern("group_info_" + req.Uuid); err != nil {
		zlog.Error(err.Error())
	}

	// 清除群聊成员列表缓存，确保下次获取时获取到最新数据
	if err := myredis.DelKeysWithPattern("group_memberlist_" + req.Uuid); err != nil {
		zlog.Error(err.Error())
	}

	// 清除群主的群聊列表缓存
	if err := myredis.DelKeysWithPattern("contact_mygroup_list_" + req.OwnerId); err != nil {
		zlog.Error(err.Error())
	}

	// 清除群成员的群聊列表缓存
	var members []string
	if err := json.Unmarshal(group.Members, &members); err == nil {
		for _, memberId := range members {
			if err := myredis.DelKeysWithPattern("contact_mygroup_list_" + memberId); err != nil {
				zlog.Error(err.Error())
			}
		}
	}

	// 返回更新成功的消息
	return "更新成功", 0
}

// GetGroupMemberList 获取群聊成员列表
// 根据群聊ID获取群聊成员列表，优先从Redis缓存获取，若缓存不存在则从数据库查询
// 参数: groupId - 群聊UUID
// 返回值:
//   - string: 操作结果消息，成功或失败的具体描述
//   - []respond.GetGroupMemberListRespond: 群聊成员信息响应对象列表
//   - int: 状态码，0表示成功，负数表示错误
func (g *groupInfoService) GetGroupMemberList(groupId string) (string, []respond.GetGroupMemberListRespond, int) {
	// 尝试从Redis缓存中获取群聊成员列表
	rspString, err := myredis.GetKeyNilIsErr("group_memberlist_" + groupId)
	// 检查从Redis获取数据是否出错
	if err != nil {
		// 如果错误是"键不存在"（redis.Nil），说明缓存中没有该群聊成员列表
		if errors.Is(err, redis.Nil) {
			// 从数据库中查询群聊信息
			var group model.GroupInfo
			if res := dao.GormDB.First(&group, "uuid = ?", groupId); res.Error != nil {
				// 数据库查询失败，记录错误日志并返回系统错误
				zlog.Error(res.Error.Error())
				return constants.SYSTEM_ERROR, nil, -1
			}

			// 反序列化群聊成员列表（群成员以UUID数组形式存储）
			var members []string
			if err := json.Unmarshal(group.Members, &members); err != nil {
				// 反序列化失败，记录错误日志并返回系统错误
				zlog.Error(err.Error())
				return constants.SYSTEM_ERROR, nil, -1
			}

			// 构造群聊成员信息响应对象列表
			var rspList []respond.GetGroupMemberListRespond
			// 遍历群成员UUID列表，查询每个成员的详细信息
			for _, member := range members {
				// 查询单个用户的信息
				var user model.UserInfo
				if res := dao.GormDB.First(&user, "uuid = ?", member); res.Error != nil {
					// 用户信息查询失败，记录错误日志并返回系统错误
					zlog.Error(res.Error.Error())
					return constants.SYSTEM_ERROR, nil, -1
				}

				// 将用户信息添加到响应对象列表中
				rspList = append(rspList, respond.GetGroupMemberListRespond{
					UserId:   user.Uuid,     // 用户UUID
					Nickname: user.Nickname, // 用户昵称
					Avatar:   user.Avatar,   // 用户头像
				})
			}

			// TODO: 如果需要，可以将响应对象列表序列化并存入Redis缓存
			rspString, err := json.Marshal(rspList)
			if err != nil {
				zlog.Error(err.Error())
			}
			if err := myredis.SetKeyEx("group_memberlist_"+groupId, string(rspString), time.Minute*constants.REDIS_TIMEOUT); err != nil {
				zlog.Error(err.Error())
			}
			// 返回数据库查询到的群聊成员列表
			return "获取群聊成员列表成功", rspList, 0
		} else {
			// 如果是其他Redis错误（不仅仅是键不存在），记录错误并返回系统错误
			zlog.Error(err.Error())
			return constants.SYSTEM_ERROR, nil, -1
		}
	}

	// 如果Redis中存在缓存数据，则将JSON字符串反序列化为响应对象列表
	var rsp []respond.GetGroupMemberListRespond
	if err := json.Unmarshal([]byte(rspString), &rsp); err != nil {
		// JSON反序列化失败，记录错误
		zlog.Error(err.Error())
	}

	// 返回缓存中的群聊成员列表
	return "获取群聊成员列表成功", rsp, 0
}

// RemoveGroupMembers 移除群聊成员
// 根据请求参数从群聊中移除指定的成员，并清理相关数据
// 参数: req - 移除群成员的请求对象，包含群聊ID、要移除的用户ID列表和操作者ID
// 返回值:
//   - string: 操作结果消息，成功或失败的具体描述
//   - int: 状态码，0表示成功，-1表示系统错误，-2表示不能移除群主
func (g *groupInfoService) RemoveGroupMembers(req request.RemoveGroupMembersRequest) (string, int) {
	// 查询群聊信息
	var group model.GroupInfo
	if res := dao.GormDB.First(&group, "uuid = ?", req.GroupId); res.Error != nil {
		// 数据库查询失败，记录错误日志并返回系统错误
		zlog.Error(res.Error.Error())
		return constants.SYSTEM_ERROR, -1
	}

	// 反序列化群聊成员列表
	var members []string
	if err := json.Unmarshal(group.Members, &members); err != nil {
		// 反序列化失败，记录错误日志并返回系统错误
		zlog.Error(err.Error())
		return constants.SYSTEM_ERROR, -1
	}

	// 创建软删除的时间戳
	var deletedAt gorm.DeletedAt
	deletedAt.Time = time.Now()
	deletedAt.Valid = true

	// 打印调试信息，显示要移除的用户ID列表和群主ID
	log.Println(req.UuidList, req.OwnerId)

	// 遍历要移除的用户ID列表
	for _, uuid := range req.UuidList {
		// 检查是否尝试移除群主，如果是则返回错误
		if req.OwnerId == uuid {
			return "不能移除群主", -2
		}
		// 从成员列表中移除指定用户
		for i, member := range members {
			if member == uuid {
				// 使用切片操作移除成员，保持其他成员不变
				members = append(members[:i], members[i+1:]...)
			}
		}
		// 减少群成员计数
		group.MemberCnt -= 1

		// 软删除该用户与群聊之间的会话记录
		if res := dao.GormDB.Model(&model.Session{}).Where("send_id = ? AND receive_id = ?", uuid, req.GroupId).Update("deleted_at", deletedAt); res.Error != nil {
			// 会话记录删除失败，记录错误日志并返回系统错误
			zlog.Error(res.Error.Error())
			return constants.SYSTEM_ERROR, -1
		}

		// 软删除该用户与群聊的联系人记录
		if res := dao.GormDB.Model(&model.UserContact{}).Where("user_id = ? AND contact_id = ?", uuid, req.GroupId).Update("deleted_at", deletedAt); res.Error != nil {
			// 联系人记录删除失败，记录错误日志并返回系统错误
			zlog.Error(res.Error.Error())
			return constants.SYSTEM_ERROR, -1
		}

		// 软删除相关的联系人申请记录
		if res := dao.GormDB.Model(&model.ContactApply{}).Where("user_id = ? AND contact_id = ?", uuid, req.GroupId).Update("deleted_at", deletedAt); res.Error != nil {
			// 申请记录删除失败，记录错误日志并返回系统错误
			zlog.Error(res.Error.Error())
			return constants.SYSTEM_ERROR, -1
		}
	}

	// 重新序列化更新后的成员列表
	group.Members, _ = json.Marshal(members)
	// 保存更新后的群聊信息到数据库
	if res := dao.GormDB.Save(&group); res.Error != nil {
		// 数据库保存失败，记录错误日志并返回系统错误
		zlog.Error(res.Error.Error())
		return constants.SYSTEM_ERROR, -1
	}

	// TODO: 以下缓存清除逻辑可根据实际需要启用
	if err := myredis.DelKeysWithPattern("group_info_" + req.GroupId); err != nil {
		zlog.Error(err.Error())
	}
	if err := myredis.DelKeysWithPattern("groupmember_list_" + req.GroupId); err != nil {
		zlog.Error(err.Error())
	}

	// 清除所有群聊会话列表缓存
	if err := myredis.DelKeysWithPrefix("group_session_list"); err != nil {
		// 缓存清除失败，记录错误日志（不影响主要流程）
		zlog.Error(err.Error())
	}

	// 清除所有用户加入的群聊列表缓存
	if err := myredis.DelKeysWithPrefix("my_joined_group_list"); err != nil {
		// 缓存清除失败，记录错误日志（不影响主要流程）
		zlog.Error(err.Error())
	}

	// 返回移除群聊成员成功的消息
	return "移除群聊成员成功", 0
}

// GetGroupInfoList 获取群聊列表 - 管理员
// 为管理员提供获取系统中所有群聊信息的功能，不使用Redis缓存以避免频繁更新的复杂性
// 参数: 无参数，此方法专为管理员设计
// 返回值:
//   - string: 操作结果消息，成功或失败的具体描述
//   - []respond.GetGroupListRespond: 群聊信息响应对象列表
//   - int: 状态码，0表示成功，负数表示错误
func (g *groupInfoService) GetGroupInfoList() (string, []respond.GetGroupListRespond, int) {
	// 查询数据库中所有群聊信息（包括已软删除的记录）
	var groupList []model.GroupInfo
	// 使用 Unscoped() 方法查询包括已软删除的数据在内的所有记录
	if res := dao.GormDB.Unscoped().Find(&groupList); res.Error != nil {
		// 数据库查询失败，记录错误日志并返回系统错误
		zlog.Error(res.Error.Error())
		return constants.SYSTEM_ERROR, nil, -1
	}

	// 初始化群聊信息响应对象列表
	var rsp []respond.GetGroupListRespond
	// 遍历所有群聊信息，转换为响应对象
	for _, group := range groupList {
		// 构造单个群聊的响应对象
		rp := respond.GetGroupListRespond{
			Uuid:    group.Uuid,    // 群聊UUID
			Name:    group.Name,    // 群聊名称
			OwnerId: group.OwnerId, // 群主ID
			Status:  group.Status,  // 群聊状态
		}

		// 检查群聊是否已被软删除
		if group.DeletedAt.Valid {
			// 如果群聊已被删除，设置IsDeleted为true
			rp.IsDeleted = true
		} else {
			// 如果群聊未被删除，设置IsDeleted为false
			rp.IsDeleted = false
		}

		// 将响应对象添加到列表中
		rsp = append(rsp, rp)
	}

	// 返回获取成功的消息和群聊列表
	return "获取成功", rsp, 0
}

// DeleteGroups 删除列表中群聊 - 管理员
// 批量删除指定的群聊及其相关的数据，包括会话、联系人和申请记录
// 参数: uuidList - 要删除的群聊UUID列表
// 返回值:
//   - string: 操作结果消息，成功或失败的具体描述
//   - int: 状态码，0表示成功，负数表示错误
func (g *groupInfoService) DeleteGroups(uuidList []string) (string, int) {
	// 遍历要删除的群聊UUID列表
	for _, uuid := range uuidList {
		// 创建软删除的时间戳
		var deletedAt gorm.DeletedAt
		deletedAt.Time = time.Now()
		deletedAt.Valid = true

		// 软删除群聊记录
		if res := dao.GormDB.Model(&model.GroupInfo{}).Where("uuid = ?", uuid).Update("deleted_at", deletedAt); res.Error != nil {
			// 群聊记录更新失败，记录错误日志并返回系统错误
			zlog.Error(res.Error.Error())
			return constants.SYSTEM_ERROR, -1
		}

		// 删除与该群聊相关的会话记录
		var sessionList []model.Session
		if res := dao.GormDB.Model(&model.Session{}).Where("receive_id = ?", uuid).Find(&sessionList); res.Error != nil {
			// 会话记录查询失败，记录错误日志并返回系统错误
			zlog.Error(res.Error.Error())
			return constants.SYSTEM_ERROR, -1
		}

		// 遍历会话列表，软删除每条会话记录
		for _, session := range sessionList {
			if res := dao.GormDB.Model(&session).Update("deleted_at", deletedAt); res.Error != nil {
				// 会话记录更新失败，记录错误日志并返回系统错误
				zlog.Error(res.Error.Error())
				return constants.SYSTEM_ERROR, -1
			}
		}

		// 删除与该群聊相关的联系人记录
		var userContactList []model.UserContact
		if res := dao.GormDB.Model(&model.UserContact{}).Where("contact_id = ?", uuid).Find(&userContactList); res.Error != nil {
			// 联系人记录查询失败，记录错误日志并返回系统错误
			zlog.Error(res.Error.Error())
			return constants.SYSTEM_ERROR, -1
		}

		// 遍历联系人列表，软删除每条联系人记录
		for _, userContact := range userContactList {
			if res := dao.GormDB.Model(&userContact).Update("deleted_at", deletedAt); res.Error != nil {
				// 联系人记录更新失败，记录错误日志并返回系统错误
				zlog.Error(res.Error.Error())
				return constants.SYSTEM_ERROR, -1
			}
		}

		// 删除与该群聊相关的联系人申请记录
		var contactApplys []model.ContactApply
		if res := dao.GormDB.Model(&contactApplys).Where("contact_id = ?", uuid).Find(&contactApplys); res.Error != nil {
			// 如果没有找到相关的申请记录，只是记录信息并不算错误
			if res.Error != gorm.ErrRecordNotFound {
				zlog.Info(res.Error.Error())
				return "无响应的申请记录需要删除", 0
			}

			// 其他错误情况，记录错误日志并返回系统错误
			zlog.Error(res.Error.Error())
			return constants.SYSTEM_ERROR, -1
		}

		// 遍历申请记录列表，软删除每条申请记录
		for _, contactApply := range contactApplys {
			if res := dao.GormDB.Model(&contactApply).Update("deleted_at", deletedAt); res.Error != nil {
				// 申请记录更新失败，记录错误日志并返回系统错误
				zlog.Error(res.Error.Error())
				return constants.SYSTEM_ERROR, -1
			}
		}
	}

	// TODO: 以下缓存清除逻辑可根据实际需要启用
	for _, uuid := range uuidList {
		if err := myredis.DelKeysWithPattern("group_info_" + uuid); err != nil {
			zlog.Error(err.Error())
		}
		if err := myredis.DelKeysWithPattern("groupmember_list_" + uuid); err != nil {
			zlog.Error(err.Error())
		}
	}

	// 清除所有用户的群聊列表缓存
	if err := myredis.DelKeysWithPrefix("contact_mygroup_list"); err != nil {
		// 缓存清除失败，记录错误日志（不影响主要流程）
		zlog.Error(err.Error())
	}

	// 清除所有群聊会话列表缓存
	if err := myredis.DelKeysWithPrefix("group_session_list"); err != nil {
		// 缓存清除失败，记录错误日志（不影响主要流程）
		zlog.Error(err.Error())
	}

	// 注意：这里重复清除了一次群聊会话列表缓存
	if err := myredis.DelKeysWithPrefix("group_session_list"); err != nil {
		// 缓存清除失败，记录错误日志（不影响主要流程）
		zlog.Error(err.Error())
	}

	// 返回删除群聊成功的消息
	return "解散/删除群聊成功", 0
}

// SetGroupsStatus 设置群聊是否启用
// 批量设置群聊的状态（如启用、禁用等），当设置为禁用状态时同时删除相关的会话记录
// 参数: uuidList - 要设置的群聊UUID列表
// 参数: status - 要设置的状态值（如启用、禁用、解散等）
// 返回值:
//   - string: 操作结果消息，成功或失败的具体描述
//   - int: 状态码，0表示成功，负数表示错误
func (g *groupInfoService) SetGroupsStatus(uuidList []string, status int8) (string, int) {
	// 创建软删除的时间戳
	var deletedAt gorm.DeletedAt
	deletedAt.Time = time.Now()
	deletedAt.Valid = true

	// 遍历要设置状态的群聊UUID列表
	for _, uuid := range uuidList {
		// 更新群聊状态
		if res := dao.GormDB.Model(&model.GroupInfo{}).Where("uuid = ?", uuid).Update("status", status); res.Error != nil {
			// 群聊状态更新失败，记录错误日志并返回系统错误
			zlog.Error(res.Error.Error())
			return constants.SYSTEM_ERROR, -1
		}

		// 如果设置为禁用状态，则同时删除相关的会话记录
		if status == group_status_enum.DISABLE {
			// 查询与该群聊相关的所有会话记录
			var sessionList []model.Session
			if res := dao.GormDB.Model(&sessionList).Where("receive_id = ?", uuid).Find(&sessionList); res.Error != nil {
				// 会话记录查询失败，记录错误日志并返回系统错误
				zlog.Error(res.Error.Error())
				return constants.SYSTEM_ERROR, -1
			}

			// 遍历会话列表，软删除每条会话记录
			for _, session := range sessionList {
				if res := dao.GormDB.Model(&session).Update("deleted_at", deletedAt); res.Error != nil {
					// 会话记录更新失败，记录错误日志并返回系统错误
					zlog.Error(res.Error.Error())
					return constants.SYSTEM_ERROR, -1
				}
			}
		}
	}

	// TODO: 以下缓存清除逻辑可根据实际需要启用
	for _, uuid := range uuidList {
		if err := myredis.DelKeysWithPattern("group_info_" + uuid); err != nil {
			zlog.Error(err.Error())
		}
	}

	// 返回设置成功的消息
	return "设置成功", 0
}
