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
	"gochat/pkg/enum/contact_apply/contact_apply_status_enum"
	"gochat/pkg/enum/group_info/group_status_enum"
	"gochat/pkg/enum/user_info/user_status_enum"
	"gochat/pkg/util/random"
	"gochat/pkg/zlog"
	"log"
	"time"

	"github.com/go-redis/redis/v8"
	"gorm.io/gorm"
)

type userContactService struct {
}

var UserContactService = new(userContactService)

// GetUserList 获取用户列表
// 获取指定用户的联系人列表（仅包含用户类型的联系人），优先从Redis缓存获取，若缓存不存在则从数据库查询
// 关于用户被禁用的问题，这里查到的是所有联系人，如果被禁用或被拉黑会以弹窗的形式提醒，无法打开会话框；如果被删除，是搜索不到该联系人的。
// 参数: ownerId - 用户ID，表示要获取哪个用户的联系人列表
// 返回值:
//   - string: 操作结果消息，成功或失败的具体描述
//   - []respond.MyUserListRespond: 用户信息响应对象列表
//   - int: 状态码，0表示成功，-1表示系统错误
func (u *userContactService) GetUserList(ownerId string) (string, []respond.MyUserListRespond, int) {
	// 尝试从Redis缓存中获取用户联系人列表
	rspString, err := myredis.GetKeyNilIsErr("contact_user_list_" + ownerId)
	// 检查从Redis获取数据是否出错
	if err != nil {
		// 如果错误是"键不存在"（redis.Nil），说明缓存中没有该用户的联系人列表
		if errors.Is(err, redis.Nil) {
			// 从数据库查询用户联系人列表
			var contactList []model.UserContact
			// 查询该用户的所有联系人（排除状态为4的已删除联系人），按创建时间倒序排列
			if res := dao.GormDB.Order("created_at DESC").Where("user_id = ? AND status != 4", ownerId).Find(&contactList); res.Error != nil {
				// 如果查询结果为空（记录未找到），返回提示信息
				if errors.Is(res.Error, gorm.ErrRecordNotFound) {
					message := "目前不存在联系人"
					zlog.Info(message)
					return message, nil, 0
				} else {
					// 其他数据库错误，记录错误日志并返回系统错误
					zlog.Error(res.Error.Error())
					return constants.SYSTEM_ERROR, nil, -1
				}
			}

			// 构造用户信息响应对象列表
			var userListRsp []respond.MyUserListRespond
			// 遍历联系人列表，筛选出用户类型的联系人
			for _, contact := range contactList {
				// 检查联系人类型是否为用户
				if contact.ContactType == contact_type_enum.USER {
					// 查询用户详细信息
					var user model.UserInfo
					if res := dao.GormDB.First(&user, "uuid = ?", contact.ContactId); res.Error != nil {
						// 用户信息查询失败，记录错误日志并返回系统错误
						zlog.Error(res.Error.Error())
						return constants.SYSTEM_ERROR, nil, -1
					}

					// 将用户信息添加到响应对象列表
					userListRsp = append(userListRsp, respond.MyUserListRespond{
						UserId:   user.Uuid,     // 用户UUID
						UserName: user.Nickname, // 用户昵称
						Avatar:   user.Avatar,   // 用户头像
					})
				}
			}

			// 将响应对象列表序列化为JSON字符串
			rspString, err := json.Marshal(userListRsp)
			if err != nil {
				// 序列化失败，记录错误日志
				zlog.Error(err.Error())
			}

			// 将序列化的用户列表存入Redis缓存，设置过期时间
			if err := myredis.SetKeyEx("contact_user_list_"+ownerId, string(rspString), time.Minute*constants.REDIS_TIMEOUT); err != nil {
				// 缓存存储失败，记录错误日志（不影响主要流程）
				zlog.Error(err.Error())
			}

			// 返回数据库查询结果
			return "获取用户列表成功", userListRsp, 0
		} else {
			// 如果是其他Redis错误（不仅仅是键不存在），记录错误日志
			zlog.Error(err.Error())
			return constants.SYSTEM_ERROR, nil, -1
		}
	}

	// 如果Redis中存在缓存数据，则将JSON字符串反序列化为响应对象列表
	var rsp []respond.MyUserListRespond
	if err := json.Unmarshal([]byte(rspString), &rsp); err != nil {
		// JSON反序列化失败，记录错误日志
		zlog.Error(err.Error())
	}

	// 返回缓存中的用户列表
	return "获取用户列表成功", rsp, 0
}

// LoadMyJoinedGroup 获取我加入的群聊
// 功能：获取指定用户加入的群聊列表（不包括自己创建的群）
// 参数：ownerId - 用户ID
// 返回值:
//   - string: 操作结果消息，成功或失败的具体描述
//   - []respond.LoadMyJoinedGroupRespond: 加入的群聊信息响应对象列表
//   - int: 状态码，0表示成功，-1表示系统错误
func (u *userContactService) LoadMyJoinedGroup(ownerId string) (string, []respond.LoadMyJoinedGroupRespond, int) {
	// 尝试从Redis缓存中获取用户加入的群聊列表
	rspString, err := myredis.GetKeyNilIsErr("my_joined_group_list_" + ownerId)
	// 检查从Redis获取数据是否出错
	if err != nil {
		// 如果错误是"键不存在"（redis.Nil），说明缓存中没有该用户的加入群聊列表
		if errors.Is(err, redis.Nil) {
			// 从数据库查询用户联系人列表
			var contactList []model.UserContact
			// 查询该用户的所有联系人记录（排除状态为6的退群和状态为7的被踢出群聊的记录），按创建时间倒序排列
			// 状态6表示用户主动退群，状态7表示被管理员踢出群聊
			if res := dao.GormDB.Order("created_at DESC").Where("user_id = ? AND status != 6 AND status != 7", ownerId).Find(&contactList); res.Error != nil {
				// 不存在不是业务问题，用Info，return 0
				if errors.Is(res.Error, gorm.ErrRecordNotFound) {
					// 如果没有找到任何记录，说明用户没有加入任何群聊
					message := "目前不存在加入的群聊"
					zlog.Info(message)
					return message, nil, 0
				} else {
					// 其他数据库错误，记录错误日志并返回系统错误
					zlog.Error(res.Error.Error())
					return constants.SYSTEM_ERROR, nil, -1
				}
			}

			// 构建群聊信息列表
			var groupList []model.GroupInfo
			// 遍历联系人列表，筛选出群聊类型的联系人
			for _, contact := range contactList {
				// 检查联系人ID是否以'G'开头，以确定是否为群聊
				if contact.ContactId[0] == 'G' {
					// 获取群聊信息
					var group model.GroupInfo
					// 根据联系人ID查询群聊详细信息
					if res := dao.GormDB.First(&group, "uuid = ?", contact.ContactId); res.Error != nil {
						// 群聊信息查询失败，记录错误日志并返回系统错误
						zlog.Error(res.Error.Error())
						return constants.SYSTEM_ERROR, nil, -1
					}

					// 群没被删除，同时群主不是自己
					// 群主删除或admin删除群聊，status为7，即被踢出群聊，所以不用判断群是否被删除，删除了到不了这步
					// 过滤掉用户自己创建的群聊（避免在"加入的群聊"列表中显示自己创建的群）
					if group.OwnerId != ownerId {
						// 将符合条件的群聊信息添加到列表中
						groupList = append(groupList, group)
					}
				}
			}

			// 构造群聊列表响应对象
			var groupListRsp []respond.LoadMyJoinedGroupRespond
			// 遍历群聊信息列表，构造响应对象
			for _, group := range groupList {
				groupListRsp = append(groupListRsp, respond.LoadMyJoinedGroupRespond{
					GroupId:   group.Uuid,   // 群聊UUID
					GroupName: group.Name,   // 群聊名称
					Avatar:    group.Avatar, // 群聊头像
				})
			}

			// 将响应对象列表序列化为JSON字符串
			rspString, err := json.Marshal(groupListRsp)
			if err != nil {
				// 序列化失败，记录错误日志
				zlog.Error(err.Error())
			}

			// 将序列化的群聊列表存入Redis缓存，设置过期时间
			if err := myredis.SetKeyEx("my_joined_group_list_"+ownerId, string(rspString), time.Minute*constants.REDIS_TIMEOUT); err != nil {
				// 缓存存储失败，记录错误日志（不影响主要流程）
				zlog.Error(err.Error())
			}

			// 返回数据库查询结果
			return "获取加入群成功", groupListRsp, 0
		} else {
			// 如果是其他Redis错误（不仅仅是键不存在），记录错误日志并返回系统错误
			zlog.Error(err.Error())
			return constants.SYSTEM_ERROR, nil, -1
		}
	}

	// 如果Redis中存在缓存数据，则将JSON字符串反序列化为响应对象列表
	var rsp []respond.LoadMyJoinedGroupRespond
	if err := json.Unmarshal([]byte(rspString), &rsp); err != nil {
		// 反序列化失败，记录错误日志
		zlog.Error(err.Error())
	}

	// 返回缓存中的数据
	return "获取加入群成功", rsp, 0
}

// GetContactInfo 获取联系人信息
// 功能：根据联系人ID获取联系人详细信息（支持用户和群聊）
// 参数：contactId - 联系人ID（以'G'开头表示群聊，否则表示用户）
// 返回值:
//   - string: 操作结果消息，成功或失败的具体描述
//   - respond.GetContactInfoRespond: 联系人信息响应对象
//   - int: 状态码，0表示成功，-1表示系统错误，-2表示联系人被禁用
func (u *userContactService) GetContactInfo(contactId string) (string, respond.GetContactInfoRespond, int) {
	// 尝试从Redis缓存中获取联系人信息
	rspString, err := myredis.GetKeyNilIsErr("contact_info_" + contactId)
	if err != nil {
		if errors.Is(err, redis.Nil) {
			// 缓存中不存在，从数据库查询
			if contactId[0] == 'G' {
				// 处理群聊信息
				var group model.GroupInfo
				if res := dao.GormDB.First(&group, "uuid = ?", contactId); res.Error != nil {
					zlog.Error(res.Error.Error())
					return constants.SYSTEM_ERROR, respond.GetContactInfoRespond{}, -1
				}

				// 检查群聊是否被禁用
				if group.Status != group_status_enum.DISABLE {
					// 构造群聊信息响应对象
					response := respond.GetContactInfoRespond{
						ContactId:        group.Uuid,      // 群聊UUID
						ContactName:      group.Name,      // 群聊名称
						ContactAvatar:    group.Avatar,    // 群聊头像
						ContactNotice:    group.Notice,    // 群聊公告
						ContactAddMode:   group.AddMode,   // 群聊添加方式
						ContactMembers:   group.Members,   // 群聊成员列表
						ContactMemberCnt: group.MemberCnt, // 群聊成员数量
						ContactOwnerId:   group.OwnerId,   // 群聊拥有者ID
					}

					// 将群聊信息存入Redis缓存
					groupJson, err := json.Marshal(response)
					if err != nil {
						zlog.Error(err.Error())
					} else {
						if err := myredis.SetKeyEx("contact_info_"+contactId, string(groupJson), time.Minute*constants.REDIS_TIMEOUT); err != nil {
							zlog.Error(err.Error())
						}
					}

					return "获取联系人信息成功", response, 0
				} else {
					zlog.Error("该群聊处于禁用状态")
					return "该群聊处于禁用状态", respond.GetContactInfoRespond{}, -2
				}
			} else {
				// 处理用户信息
				var user model.UserInfo
				if res := dao.GormDB.First(&user, "uuid = ?", contactId); res.Error != nil {
					zlog.Error(res.Error.Error())
					return constants.SYSTEM_ERROR, respond.GetContactInfoRespond{}, -1
				}
				log.Println(user)
				if user.Status != user_status_enum.DISABLE {
					// 构造用户信息响应对象
					response := respond.GetContactInfoRespond{
						ContactId:        user.Uuid,      // 用户UUID
						ContactName:      user.Nickname,  // 用户昵称
						ContactAvatar:    user.Avatar,    // 用户头像
						ContactBirthday:  user.Birthday,  // 用户生日
						ContactEmail:     user.Email,     // 用户邮箱
						ContactPhone:     user.Telephone, // 用户电话
						ContactGender:    user.Gender,    // 用户性别
						ContactSignature: user.Signature, // 用户签名
					}

					// 将用户信息存入Redis缓存
					userJson, err := json.Marshal(response)
					if err != nil {
						zlog.Error(err.Error())
					} else {
						if err := myredis.SetKeyEx("contact_info_"+contactId, string(userJson), time.Minute*constants.REDIS_TIMEOUT); err != nil {
							zlog.Error(err.Error())
						}
					}

					return "获取联系人信息成功", response, 0
				} else {
					zlog.Info("该用户处于禁用状态")
					return "该用户处于禁用状态", respond.GetContactInfoRespond{}, -2
				}
			}
		} else {
			// Redis连接错误
			zlog.Error(err.Error())
			return constants.SYSTEM_ERROR, respond.GetContactInfoRespond{}, -1
		}
	}

	// 从缓存获取数据成功，反序列化为响应对象
	var cachedResponse respond.GetContactInfoRespond
	if err := json.Unmarshal([]byte(rspString), &cachedResponse); err != nil {
		zlog.Error(err.Error())
		return constants.SYSTEM_ERROR, respond.GetContactInfoRespond{}, -1
	}

	return "获取联系人信息成功", cachedResponse, 0
}

// DeleteContact 删除联系人（只包含用户）
// 功能：删除指定用户的联系人关系（双向删除），包括用户之间的联系人记录、会话记录和申请记录
// 参数：ownerId - 当前用户ID，contactId - 要删除的联系人ID
// 返回值:
//   - string: 操作结果消息，成功或失败的具体描述
//   - int: 状态码，0表示成功，-1表示系统错误
func (u *userContactService) DeleteContact(ownerId, contactId string) (string, int) {
	// 创建软删除时间戳，用于标记记录为已删除状态
	var deletedAt gorm.DeletedAt
	deletedAt.Time = time.Now()
	deletedAt.Valid = true

	// 更新当前用户的联系人记录为已删除状态，并设置状态为DELETE（主动删除）
	// 这条记录表示ownerId用户删除了contactId联系人
	if res := dao.GormDB.Model(&model.UserContact{}).Where("user_id = ? AND contact_id = ?", ownerId, contactId).Updates(map[string]interface{}{
		"deleted_at": deletedAt,                  // 设置软删除时间
		"status":     contact_status_enum.DELETE, // 设置状态为主动删除
	}); res.Error != nil {
		zlog.Error(res.Error.Error())
		return constants.SYSTEM_ERROR, -1
	}
	// 更新对方用户的联系人记录为已删除状态，并设置状态为BE_DELETE（被删除）
	// 这条记录表示contactId用户的联系人记录中，被ownerId用户删除
	if res := dao.GormDB.Model(&model.UserContact{}).Where("user_id = ? AND contact_id = ?", contactId, ownerId).Updates(map[string]interface{}{
		"deleted_at": deletedAt,                     // 设置软删除时间
		"status":     contact_status_enum.BE_DELETE, // 设置状态为被删除
	}); res.Error != nil {
		zlog.Error(res.Error.Error())
		return constants.SYSTEM_ERROR, -1
	}

	// 删除当前用户到联系人的会话记录（软删除）
	// 清理ownerId用户发送给contactId用户的会话数据
	if res := dao.GormDB.Model(&model.Session{}).Where("send_id = ? AND receive_id = ?", ownerId, contactId).Update("deleted_at", deletedAt); res.Error != nil {
		zlog.Error(res.Error.Error())
		return constants.SYSTEM_ERROR, -1
	}
	// 删除联系人到当前用户的会话记录（软删除）
	// 清理contactId用户发送给ownerId用户的会话数据
	if res := dao.GormDB.Model(&model.Session{}).Where("send_id = ? AND receive_id = ?", contactId, ownerId).Update("deleted_at", deletedAt); res.Error != nil {
		zlog.Error(res.Error.Error())
		return constants.SYSTEM_ERROR, -1
	}

	// 联系人添加的记录得删，这样之后再添加就看新的申请记录，如果申请记录结果是拉黑就没法再添加，如果是拒绝可以再添加
	// 删除联系人申请记录（当前用户相关的申请）
	if res := dao.GormDB.Model(&model.ContactApply{}).Where("contact_id = ? AND user_id = ?", ownerId, contactId).Update("deleted_at", deletedAt); res.Error != nil {
		zlog.Error(res.Error.Error())
		return constants.SYSTEM_ERROR, -1
	}
	// 删除联系人申请记录（对方用户相关的申请）
	if res := dao.GormDB.Model(&model.ContactApply{}).Where("contact_id = ? AND user_id = ?", contactId, ownerId).Update("deleted_at", deletedAt); res.Error != nil {
		zlog.Error(res.Error.Error())
		return constants.SYSTEM_ERROR, -1
	}

	// 清除当前用户的联系人列表缓存
	// 使用通配符删除以"contact_user_list_"+ownerId开头的缓存键
	if err := myredis.DelKeysWithPattern("contact_user_list_" + ownerId); err != nil {
		zlog.Error(err.Error())
	}
	// 清除被删除联系人的联系人列表缓存
	// 使用通配符删除以"contact_user_list_"+contactId开头的缓存键
	if err := myredis.DelKeysWithPattern("contact_user_list_" + contactId); err != nil {
		zlog.Error(err.Error())
	}

	return "删除联系人成功", 0
}

// ApplyContact 申请添加联系人
// 功能：允许用户申请添加其他用户或群聊为联系人
// 参数：req - 包含申请信息的请求对象，包含用户ID、联系人ID和申请消息
// 返回值:
//   - string: 操作结果消息，成功或失败的具体描述
//   - int: 状态码，0表示成功，-1表示系统错误，-2表示业务验证失败
func (u *userContactService) ApplyContact(req request.ApplyContactRequest) (string, int) {
	// 判断联系人ID的第一个字符来区分是用户还是群聊
	switch req.ContactId[0] {
	case 'U':
		// 处理添加用户的情况
		var user model.UserInfo
		// 查询目标用户是否存在
		if res := dao.GormDB.First(&user, "uuid = ?", req.ContactId); res.Error != nil {
			if errors.Is(res.Error, gorm.ErrRecordNotFound) {
				zlog.Error("用户不存在")
				return "用户不存在", -2
			} else {
				zlog.Error(res.Error.Error())
				return constants.SYSTEM_ERROR, -1
			}
		}

		// 检查目标用户是否被禁用
		if user.Status == user_status_enum.DISABLE {
			zlog.Info("用户已被禁用")
			return "用户已被禁用", -2
		}

		// 查找是否已有申请记录
		var contactApply model.ContactApply
		if res := dao.GormDB.Where("user_id = ? AND contact_id = ?", req.OwnerId, req.ContactId).First(&contactApply); res.Error != nil {
			if errors.Is(res.Error, gorm.ErrRecordNotFound) {
				// 如果没有找到申请记录，则创建新的申请
				contactApply = model.ContactApply{
					Uuid:        fmt.Sprintf("A%s", random.GetNowAndLenRandomString(11)), // 生成唯一的申请记录ID
					UserId:      req.OwnerId,                                             // 申请人ID
					ContactId:   req.ContactId,                                           // 目标联系人ID
					ContactType: contact_type_enum.USER,                                  // 联系人类型为用户
					Status:      contact_apply_status_enum.PENDING,                       // 申请状态为待处理
					Message:     req.Message,                                             // 申请消息
					LastApplyAt: time.Now(),                                              // 最近申请时间
				}

				// 在数据库中创建申请记录
				if res := dao.GormDB.Create(&contactApply); res.Error != nil {
					zlog.Error(res.Error.Error())
					return constants.SYSTEM_ERROR, -1
				}

				// 清除接收方的新联系人申请列表缓存
				if err := myredis.DelKeysWithPattern("new_contact_list_" + req.ContactId); err != nil {
					zlog.Error(err.Error())
				}
			} else {
				// 数据库查询错误
				zlog.Error(res.Error.Error())
				return constants.SYSTEM_ERROR, -1
			}
		}

		// 如果存在申请记录，先看看有没有被拉黑
		if contactApply.Status == contact_apply_status_enum.BLACK {
			// 如果已经被对方拉黑，则不能发起申请
			return "对方已将你拉黑", -2
		}

		// 更新申请记录的时间和状态
		contactApply.LastApplyAt = time.Now()
		contactApply.Status = contact_apply_status_enum.PENDING

		// 保存更新后的申请记录
		if res := dao.GormDB.Save(&contactApply); res.Error != nil {
			zlog.Error(res.Error.Error())
			return constants.SYSTEM_ERROR, -1
		}

		// 清除接收方的新联系人申请列表缓存
		if err := myredis.DelKeysWithPattern("new_contact_list_" + req.ContactId); err != nil {
			zlog.Error(err.Error())
		}

		return "申请成功", 0
	case 'G':
		// 处理添加群聊的情况
		var group model.GroupInfo
		// 查询目标群聊是否存在
		if res := dao.GormDB.First(&group, "uuid = ?", req.ContactId); res.Error != nil {
			if errors.Is(res.Error, gorm.ErrRecordNotFound) {
				zlog.Error("群聊不存在")
				return "群聊不存在", -2
			} else {
				zlog.Error(res.Error.Error())
				return constants.SYSTEM_ERROR, -1
			}
		}

		// 检查群聊是否被禁用
		if group.Status == group_status_enum.DISABLE {
			zlog.Info("群聊已被禁用")
			return "群聊已被禁用", -2
		}

		// 查找是否已有申请记录
		var contactApply model.ContactApply
		if res := dao.GormDB.Where("user_id = ? AND contact_id = ?", req.OwnerId, req.ContactId).First(&contactApply); res.Error != nil {
			if errors.Is(res.Error, gorm.ErrRecordNotFound) {
				// 如果没有找到申请记录，则创建新的申请
				contactApply = model.ContactApply{
					Uuid:        fmt.Sprintf("A%s", random.GetNowAndLenRandomString(11)), // 生成唯一的申请记录ID
					UserId:      req.OwnerId,                                             // 申请人ID
					ContactId:   req.ContactId,                                           // 目标群聊ID
					ContactType: contact_type_enum.GROUP,                                 // 联系人类型为群聊
					Status:      contact_apply_status_enum.PENDING,                       // 申请状态为待处理
					Message:     req.Message,                                             // 申请消息
					LastApplyAt: time.Now(),                                              // 最近申请时间
				}

				// 在数据库中创建申请记录
				if res := dao.GormDB.Create(&contactApply); res.Error != nil {
					zlog.Error(res.Error.Error())
					return constants.SYSTEM_ERROR, -1
				}

				// 清除接收方的新联系人申请列表缓存
				if err := myredis.DelKeysWithPattern("new_contact_list_" + req.ContactId); err != nil {
					zlog.Error(err.Error())
				}
			} else {
				// 数据库查询错误
				zlog.Error(res.Error.Error())
				return constants.SYSTEM_ERROR, -1
			}
		}

		// 更新申请记录的时间
		contactApply.LastApplyAt = time.Now()

		// 保存更新后的申请记录
		if res := dao.GormDB.Save(&contactApply); res.Error != nil {
			zlog.Error(res.Error.Error())
			return constants.SYSTEM_ERROR, -1
		}

		// 清除接收方的新联系人申请列表缓存
		if err := myredis.DelKeysWithPattern("new_contact_list_" + req.ContactId); err != nil {
			zlog.Error(err.Error())
		}

		return "申请成功", 0
	default:
		// 联系人ID格式不正确，既不是用户也不是群聊
		return "用户/群聊不存在", -2
	}
}

// GetNewContactList 获取新的联系人申请列表
// 功能：获取指定用户收到的待处理联系人申请列表，优先从Redis缓存获取，若缓存不存在则从数据库查询
// 参数：ownerId - 用户ID，表示要获取哪个用户的待处理申请
// 返回值:
//   - string: 操作结果消息，成功或失败的具体描述
//   - []respond.NewContactListRespond: 新的联系人申请响应对象列表
//   - int: 状态码，0表示成功，-1表示系统错误
func (u *userContactService) GetNewContactList(ownerId string) (string, []respond.NewContactListRespond, int) {
	// 尝试从Redis缓存中获取用户的新联系人申请列表
	rspString, err := myredis.GetKeyNilIsErr("new_contact_list_" + ownerId)
	// 检查从Redis获取数据是否出错
	if err != nil {
		// 如果错误是"键不存在"（redis.Nil），说明缓存中没有该用户的新联系人申请列表
		if errors.Is(err, redis.Nil) {
			// 从数据库查询新联系人申请列表
			var contactApplyList []model.ContactApply
			if res := dao.GormDB.Where("contact_id = ? AND status = ?", ownerId, contact_apply_status_enum.PENDING).Find(&contactApplyList); res.Error != nil {
				if errors.Is(res.Error, gorm.ErrRecordNotFound) {
					zlog.Info("没有在申请的联系人")
					return "没有在申请的联系人", nil, 0
				} else {
					zlog.Error(res.Error.Error())
					return constants.SYSTEM_ERROR, nil, -1
				}
			}

			// 初始化响应对象列表
			var rsp []respond.NewContactListRespond
			// 遍历所有待处理的申请记录
			// 所有contact都没被删除
			for _, contactApply := range contactApplyList {
				// 查询申请用户的详细信息
				var user model.UserInfo
				if res := dao.GormDB.First(&user, "uuid = ?", contactApply.UserId); res.Error != nil {
					return constants.SYSTEM_ERROR, nil, -1
				}

				// 处理申请消息，如果为空则显示默认文本
				var message string
				if contactApply.Message == "" {
					message = "申请理由：无"
				} else {
					message = "申请理由：" + contactApply.Message
				}

				// 创建响应对象，直接使用用户信息初始化
				newContact := respond.NewContactListRespond{
					ContactId:     user.Uuid,     // 用户UUID
					ContactName:   user.Nickname, // 用户昵称
					ContactAvatar: user.Avatar,   // 用户头像
					Message:       message,       // 申请消息
				}

				// 将响应对象添加到结果列表
				rsp = append(rsp, newContact)
			}

			// 将响应对象列表序列化为JSON字符串
			rspString, err := json.Marshal(rsp)
			if err != nil {
				// 序列化失败，记录错误日志
				zlog.Error(err.Error())
			}

			// 将序列化的申请列表存入Redis缓存，设置过期时间
			if err := myredis.SetKeyEx("new_contact_list_"+ownerId, string(rspString), time.Minute*constants.REDIS_TIMEOUT); err != nil {
				// 缓存存储失败，记录错误日志（不影响主要流程）
				zlog.Error(err.Error())
			}

			// 返回数据库查询结果
			return "获取成功", rsp, 0
		} else {
			// 如果是其他Redis错误（不仅仅是键不存在），记录错误日志
			zlog.Error(err.Error())
			return constants.SYSTEM_ERROR, nil, -1
		}
	}

	// 如果Redis中存在缓存数据，则将JSON字符串反序列化为响应对象列表
	var rsp []respond.NewContactListRespond
	if err := json.Unmarshal([]byte(rspString), &rsp); err != nil {
		// JSON反序列化失败，记录错误日志
		zlog.Error(err.Error())
	}

	// 返回缓存中的数据
	return "获取成功", rsp, 0
}

// PassContactApply 通过联系人申请
// 功能：处理联系人申请的通过操作，支持用户间添加好友和用户加入群聊两种场景
// 参数：ownerId - 接收申请的用户ID或群聊ID，contactId - 发起申请的用户ID
// 返回值:
//   - string: 操作结果消息，成功或失败的具体描述
//   - int: 状态码，0表示成功，-1表示系统错误，-2表示业务验证失败
func (u *userContactService) PassContactApply(ownerId string, contactId string) (string, int) {
	// 查询联系人申请记录
	// ownerId 如果是用户的话就是登录用户，如果是群聊的话就是群聊id
	var contactApply model.ContactApply
	if res := dao.GormDB.Where("contact_id = ? AND user_id = ?", ownerId, contactId).First(&contactApply); res.Error != nil {
		zlog.Error(res.Error.Error())
		return constants.SYSTEM_ERROR, -1
	}

	// 判断ownerId是用户还是群聊
	if ownerId[0] == 'U' {
		// 处理用户间添加好友的场景
		var user model.UserInfo
		// 查询被添加用户的详细信息
		if res := dao.GormDB.Where("uuid = ?", contactId).Find(&user); res.Error != nil {
			zlog.Error(res.Error.Error())
		}

		// 检查被添加用户是否被禁用
		if user.Status == user_status_enum.DISABLE {
			zlog.Error("用户已被禁用")
			return "用户已被禁用", -2
		}

		// 更新申请记录状态为已同意
		contactApply.Status = contact_apply_status_enum.AGREE
		if res := dao.GormDB.Save(&contactApply); res.Error != nil {
			zlog.Error(res.Error.Error())
			return constants.SYSTEM_ERROR, -1
		}

		// 清除申请人（contactId）的新联系人申请列表缓存
		if err := myredis.DelKeysWithPattern("new_contact_list_" + contactId); err != nil {
			zlog.Error(err.Error())
		}

		// 创建双向的用户联系人关系记录（ownerId -> contactId）
		newContact := model.UserContact{
			UserId:      ownerId,                    // 用户ID
			ContactId:   contactId,                  // 联系人ID
			ContactType: contact_type_enum.USER,     // 联系人类型为用户
			Status:      contact_status_enum.NORMAL, // 状态为正常
			CreatedAt:   time.Now(),                 // 创建时间
			UpdatedAt:   time.Now(),                 // 更新时间
		}
		// 在数据库中创建联系人记录
		if res := dao.GormDB.Create(&newContact); res.Error != nil {
			zlog.Error(res.Error.Error())
			return constants.SYSTEM_ERROR, -1
		}

		// 创建反向的用户联系人关系记录（contactId -> ownerId）
		anotherContact := model.UserContact{
			UserId:      contactId,                  // 用户ID
			ContactId:   ownerId,                    // 联系人ID
			ContactType: contact_type_enum.USER,     // 联系人类型为用户
			Status:      contact_status_enum.NORMAL, // 状态为正常
			CreatedAt:   newContact.CreatedAt,       // 创建时间
			UpdatedAt:   newContact.UpdatedAt,       // 更新时间
		}
		// 在数据库中创建反向联系人记录
		if res := dao.GormDB.Create(&anotherContact); res.Error != nil {
			zlog.Error(res.Error.Error())
			return constants.SYSTEM_ERROR, -1
		}

		// 清除接收方（ownerId）的联系人列表缓存
		if err := myredis.DelKeysWithPattern("contact_user_list_" + ownerId); err != nil {
			zlog.Error(err.Error())
		}

		return "已添加该联系人", 0
	} else {
		// 处理用户加入群聊的场景
		var group model.GroupInfo
		// 查询群聊信息
		if res := dao.GormDB.Where("uuid = ?", ownerId).Find(&group); res.Error != nil {
			zlog.Error(res.Error.Error())
		}

		// 检查群聊是否被禁用
		if group.Status == group_status_enum.DISABLE {
			zlog.Error("群聊已被禁用")
			return "群聊已被禁用", -2
		}

		// 更新申请记录状态为已同意
		contactApply.Status = contact_apply_status_enum.AGREE
		if res := dao.GormDB.Save(&contactApply); res.Error != nil {
			zlog.Error(res.Error.Error())
			return constants.SYSTEM_ERROR, -1
		}

		// 清除申请人（contactId）的新联系人申请列表缓存
		if err := myredis.DelKeysWithPattern("new_contact_list_" + contactId); err != nil {
			zlog.Error(err.Error())
		}

		// 群聊就只用创建一个UserContact，因为一个UserContact足以表达双方的状态
		// 创建用户加入群聊的记录
		newContact := model.UserContact{
			UserId:      contactId,                  // 用户ID
			ContactId:   ownerId,                    // 群聊ID
			ContactType: contact_type_enum.GROUP,    // 联系人类型为群聊
			Status:      contact_status_enum.NORMAL, // 状态为正常
			CreatedAt:   time.Now(),                 // 创建时间
			UpdatedAt:   time.Now(),                 // 更新时间
		}

		// 在数据库中创建用户群聊关系记录
		if res := dao.GormDB.Create(&newContact); res.Error != nil {
			zlog.Error(res.Error.Error())
			return constants.SYSTEM_ERROR, -1
		}

		// 更新群聊成员列表
		var members []string
		if err := json.Unmarshal(group.Members, &members); err != nil {
			zlog.Error(err.Error())
			return constants.SYSTEM_ERROR, -1
		}
		// 将新成员添加到成员列表
		members = append(members, contactId)
		// 更新群聊成员数量
		group.MemberCnt = len(members)
		// 序列化成员列表
		group.Members, _ = json.Marshal(members)
		// 保存群聊信息更新
		if res := dao.GormDB.Save(&group); res.Error != nil {
			zlog.Error(res.Error.Error())
			return constants.SYSTEM_ERROR, -1
		}

		// 清除新成员（contactId）的群聊列表缓存
		if err := myredis.DelKeysWithPattern("my_joined_group_list_" + contactId); err != nil {
			zlog.Error(err.Error())
		}

		return "已通过加群申请", 0
	}
}

// BlackContact 拉黑联系人
// 功能：将指定联系人加入黑名单，包括更新双方的联系人状态和删除相关会话记录
// 参数：ownerId - 执行拉黑操作的用户ID，contactId - 被拉黑的联系人ID
// 返回值:
//   - string: 操作结果消息，成功或失败的具体描述
//   - int: 状态码，0表示成功，-1表示系统错误
func (u *userContactService) BlackContact(ownerId string, contactId string) (string, int) {
	// 更新当前用户对被拉黑联系人的状态为"拉黑"
	// 即：ownerId用户将contactId拉黑
	if res := dao.GormDB.Model(&model.UserContact{}).Where("user_id = ? AND contact_id = ?", ownerId, contactId).Updates(map[string]interface{}{
		"status":    contact_status_enum.BLACK, // 设置状态为拉黑
		"update_at": time.Now(),                // 更新时间
	}); res.Error != nil {
		zlog.Error(res.Error.Error())
		return constants.SYSTEM_ERROR, -1
	}

	// 更新被拉黑联系人对当前用户的状态为"被拉黑"
	// 即：contactId用户被ownerId拉黑
	if res := dao.GormDB.Model(&model.UserContact{}).Where("user_id = ? AND contact_id = ?", contactId, ownerId).Updates(map[string]interface{}{
		"status":    contact_status_enum.BE_BLACK, // 设置状态为被拉黑
		"update_at": time.Now(),                   // 更新时间
	}); res.Error != nil {
		zlog.Error(res.Error.Error())
		return constants.SYSTEM_ERROR, -1
	}

	// 删除双方之间的会话记录（软删除）
	// 创建软删除时间戳
	var deletedAt gorm.DeletedAt
	deletedAt.Time = time.Now()
	deletedAt.Valid = true

	// 删除从ownerId到contactId的会话记录
	if res := dao.GormDB.Model(&model.Session{}).Where("send_id = ? AND receive_id = ?", ownerId, contactId).Update("deleted_at", deletedAt); res.Error != nil {
		zlog.Error(res.Error.Error())
		return constants.SYSTEM_ERROR, -1
	}

	return "已拉黑该联系人", 0
}

// CancelBlackContact 取消拉黑联系人
// 功能：取消将指定联系人加入黑名单，恢复双方的正常联系人状态
// 参数：ownerId - 执行取消拉黑操作的用户ID，contactId - 被取消拉黑的联系人ID
// 返回值:
//   - string: 操作结果消息，成功或失败的具体描述
//   - int: 状态码，0表示成功，-1表示系统错误，-2表示业务验证失败
func (u *userContactService) CancelBlackContact(ownerId string, contactId string) (string, int) {
	// 因为前端的设定，这里需要判断一下ownerId和contactId是不是有拉黑和被拉黑的状态
	// 查询当前用户对指定联系人的拉黑状态
	var blackContact model.UserContact
	if res := dao.GormDB.Where("user_id = ? AND contact_id = ?", ownerId, contactId).First(&blackContact); res.Error != nil {
		zlog.Error(res.Error.Error())
		return constants.SYSTEM_ERROR, -1
	}

	// 检查当前用户是否真的拉黑了该联系人
	if blackContact.Status != contact_status_enum.BLACK {
		return "未拉黑该联系人，无需解除拉黑", -2
	}

	// 查询被取消拉黑的联系人对该用户的拉黑状态
	var beBlackContact model.UserContact
	if res := dao.GormDB.Where("user_id = ? AND contact_id = ?", contactId, ownerId).First(&beBlackContact); res.Error != nil {
		zlog.Error(res.Error.Error())
		return constants.SYSTEM_ERROR, -1
	}

	// 检查该联系人是否真的被拉黑了
	if beBlackContact.Status != contact_status_enum.BE_BLACK {
		return "该联系人未被拉黑，无需解除拉黑", -2
	}

	// 更新双方的联系人状态为正常
	blackContact.Status = contact_status_enum.NORMAL   // 当前用户对联系人的状态设为正常
	beBlackContact.Status = contact_status_enum.NORMAL // 联系人对当前用户的状态设为正常

	// 保存当前用户对联系人的状态更新
	if res := dao.GormDB.Save(&blackContact); res.Error != nil {
		zlog.Error(res.Error.Error())
		return constants.SYSTEM_ERROR, -1
	}

	// 保存联系人对当前用户的状态更新
	if res := dao.GormDB.Save(&beBlackContact); res.Error != nil {
		zlog.Error(res.Error.Error())
		return constants.SYSTEM_ERROR, -1
	}

	return "已解除拉黑该联系人", 0
}

// GetAddGroupList 获取新的加群列表
// 功能：获取指定群聊的待处理加群申请列表，仅群主有权调用此接口
// 参数：groupId - 群聊ID，表示要获取哪个群聊的待处理加群申请
// 返回值:
//   - string: 操作结果消息，成功或失败的具体描述
//   - []respond.AddGroupListRespond: 加群申请响应对象列表
//   - int: 状态码，0表示成功，-1表示系统错误
//
// 前端已经判断调用接口的用户是群主，也只有群主才能调用这个接口
func (u *userContactService) GetAddGroupList(groupId string) (string, []respond.AddGroupListRespond, int) {
	// 查询数据库中指定群聊的待处理加群申请记录
	var contactApplyList []model.ContactApply
	if res := dao.GormDB.Where("contact_id = ? AND status = ?", groupId, contact_apply_status_enum.PENDING).Find(&contactApplyList); res.Error != nil {
		if errors.Is(res.Error, gorm.ErrRecordNotFound) {
			zlog.Info("没有在申请的联系人")
			return "没有在申请的联系人", nil, 0
		} else {
			zlog.Error(res.Error.Error())
			return constants.SYSTEM_ERROR, nil, -1
		}
	}

	// 初始化响应对象列表
	var rsp []respond.AddGroupListRespond
	// 遍历所有待处理的加群申请记录
	for _, contactApply := range contactApplyList {
		// 查询申请加群的用户详细信息
		var user model.UserInfo
		if res := dao.GormDB.First(&user, "uuid = ?", contactApply.UserId); res.Error != nil {
			return constants.SYSTEM_ERROR, nil, -1
		}

		// 处理申请消息，如果为空则显示默认文本
		var message string
		if contactApply.Message == "" {
			message = "申请理由：无"
		} else {
			message = "申请理由：" + contactApply.Message
		}

		// 创建响应对象，使用用户信息和申请信息初始化
		newContact := respond.AddGroupListRespond{
			ContactId:     user.Uuid,     // 申请用户的UUID
			ContactName:   user.Nickname, // 申请用户的昵称
			ContactAvatar: user.Avatar,   // 申请用户的头像
			Message:       message,       // 申请消息
		}
		// 将响应对象添加到结果列表
		rsp = append(rsp, newContact)
	}
	// 返回成功的响应
	return "获取成功", rsp, 0
}

// RefuseContactApply 拒绝联系人申请
// 功能：处理联系人申请的拒绝操作，支持拒绝用户间好友申请和拒绝用户加入群聊申请
// 参数：ownerId - 接收申请的用户ID或群聊ID，contactId - 发起申请的用户ID
// 返回值:
//   - string: 操作结果消息，成功或失败的具体描述
//   - int: 状态码，0表示成功，-1表示系统错误
func (u *userContactService) RefuseContactApply(ownerId string, contactId string) (string, int) {
	// ownerId 如果是用户的话就是登录用户，如果是群聊的话就是群聊id
	// 查询联系人申请记录
	var contactApply model.ContactApply
	if res := dao.GormDB.Where("contact_id = ? AND user_id = ?", ownerId, contactId).First(&contactApply); res.Error != nil {
		zlog.Error(res.Error.Error())
		return constants.SYSTEM_ERROR, -1
	}

	// 更新申请记录状态为已拒绝
	contactApply.Status = contact_apply_status_enum.REFUSE
	if res := dao.GormDB.Save(&contactApply); res.Error != nil {
		zlog.Error(res.Error.Error())
		return constants.SYSTEM_ERROR, -1
	}

	// 清除申请人（contactId）的新联系人申请列表缓存
	if err := myredis.DelKeysWithPattern("new_contact_list_" + contactId); err != nil {
		zlog.Error(err.Error())
	}

	// 根据ownerId的类型判断是拒绝好友申请还是拒绝加群申请
	if ownerId[0] == 'U' {
		// 如果ownerId以'U'开头，表示拒绝的是用户间的好友申请
		return "已拒绝该联系人申请", 0
	} else {
		// 如果ownerId不是以'U'开头，表示拒绝的是加入群聊的申请
		return "已拒绝该加群申请", 0
	}
}

// BlackApply 拉黑申请
// 功能：将指定用户的联系人申请标记为拉黑状态，阻止该用户再次发送申请
// 参数：ownerId - 接收申请的用户ID或群聊ID，contactId - 发起申请的用户ID
// 返回值:
//   - string: 操作结果消息，成功或失败的具体描述
//   - int: 状态码，0表示成功，-1表示系统错误
func (u *userContactService) BlackApply(ownerId string, contactId string) (string, int) {
	// 查询指定的联系人申请记录
	var contactApply model.ContactApply
	if res := dao.GormDB.Where("contact_id = ? AND user_id = ?", ownerId, contactId).First(&contactApply); res.Error != nil {
		zlog.Error(res.Error.Error())
		return constants.SYSTEM_ERROR, -1
	}

	// 将申请记录的状态更新为拉黑状态
	contactApply.Status = contact_apply_status_enum.BLACK
	if res := dao.GormDB.Save(&contactApply); res.Error != nil {
		zlog.Error(res.Error.Error())
		return constants.SYSTEM_ERROR, -1
	}

	// 清除申请人（contactId）的新联系人申请列表缓存，确保下次获取时能反映最新状态
	if err := myredis.DelKeysWithPattern("new_contact_list_" + contactId); err != nil {
		zlog.Error(err.Error())
	}

	return "已拉黑该申请", 0
}
