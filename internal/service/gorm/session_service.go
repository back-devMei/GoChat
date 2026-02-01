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
	"gochat/pkg/enum/group_info/group_status_enum"
	"gochat/pkg/enum/user_info/user_status_enum"
	"gochat/pkg/util/random"
	"gochat/pkg/zlog"
	"time"

	"github.com/go-redis/redis/v8"
	"gorm.io/gorm"
)

type sessionService struct {
}

var SessionService = new(sessionService)

// CreateSession 创建会话
// 功能：创建一个新的会话记录，支持用户与用户之间或用户与群聊之间的会话
// 参数：req - 包含创建会话所需信息的请求对象，包括发送者ID和接收者ID
// 返回值:
//   - string: 操作结果消息，成功或失败的具体描述
//   - string: 会话UUID，创建成功时返回会话唯一标识
//   - int: 状态码，0表示成功，-1表示系统错误，-2表示业务验证失败
func (s *sessionService) CreateSession(req request.CreateSessionRequest) (string, string, int) {
	// 查询发送者用户信息，验证发送者是否存在
	var user model.UserInfo
	if res := dao.GormDB.Where("uuid = ?", req.SendId).First(&user); res.Error != nil {
		zlog.Error(res.Error.Error())
		return constants.SYSTEM_ERROR, "", -1
	}

	// 创建会话对象并初始化基本信息
	var session model.Session
	session.Uuid = fmt.Sprintf("S%s", random.GetNowAndLenRandomString(11)) // 生成会话唯一标识，以'S'开头
	session.SendId = req.SendId                                            // 设置发送者ID
	session.ReceiveId = req.ReceiveId                                      // 设置接收者ID
	session.CreatedAt = time.Now()                                         // 设置创建时间

	// 根据接收者ID的第一个字符判断接收者类型（用户或群聊）
	if req.ReceiveId[0] == 'U' {
		// 处理接收者为用户的情况
		var receiveUser model.UserInfo
		// 查询接收用户的信息
		if res := dao.GormDB.Where("uuid = ?", req.ReceiveId).First(&receiveUser); res.Error != nil {
			zlog.Error(res.Error.Error())
			return constants.SYSTEM_ERROR, "", -1
		}

		// 检查接收用户是否被禁用
		if receiveUser.Status == user_status_enum.DISABLE {
			zlog.Error("该用户被禁用了")
			return "该用户被禁用了", "", -2
		} else {
			// 设置会话中的接收者姓名和头像
			session.ReceiveName = receiveUser.Nickname
			session.Avatar = receiveUser.Avatar
		}
	} else {
		// 处理接收者为群聊的情况
		var receiveGroup model.GroupInfo
		// 查询接收群聊的信息
		if res := dao.GormDB.Where("uuid = ?", req.ReceiveId).First(&receiveGroup); res.Error != nil {
			zlog.Error(res.Error.Error())
			return constants.SYSTEM_ERROR, "", -1
		}

		// 检查接收群聊是否被禁用
		if receiveGroup.Status == group_status_enum.DISABLE {
			zlog.Error("该群聊被禁用了")
			return "该群聊被禁用了", "", -2
		} else {
			// 设置会话中的接收群聊名称和头像
			session.ReceiveName = receiveGroup.Name
			session.Avatar = receiveGroup.Avatar
		}
	}

	// 在数据库中创建会话记录
	if res := dao.GormDB.Create(&session); res.Error != nil {
		zlog.Error(res.Error.Error())
		return constants.SYSTEM_ERROR, "", -1
	}

	// 清除发送者的群会话列表缓存，确保下次获取时能反映最新状态
	if err := myredis.DelKeysWithPattern("group_session_list_" + req.SendId); err != nil {
		zlog.Error(err.Error())
	}

	// 清除发送者的会话列表缓存，确保下次获取时能反映最新状态
	if err := myredis.DelKeysWithPattern("session_list_" + req.SendId); err != nil {
		zlog.Error(err.Error())
	}

	return "会话创建成功", session.Uuid, 0
}

// OpenSession 打开会话
// 功能：打开或创建一个会话，优先从Redis缓存获取，若缓存不存在则从数据库查询或创建
// 参数：req - 包含打开会话所需信息的请求对象，包括发送者ID和接收者ID
// 返回值:
//   - string: 操作结果消息，成功或失败的具体描述
//   - string: 会话UUID，会话唯一标识
//   - int: 状态码，0表示成功，-1表示系统错误
func (s *sessionService) OpenSession(req request.OpenSessionRequest) (string, string, int) {
	// 尝试从Redis缓存中获取会话信息，缓存键格式为"session_{sendId}_{receiveId}"
	rspString, err := myredis.GetKeyWithPrefixNilIsErr("session_" + req.SendId + "_" + req.ReceiveId)
	if err != nil {
		if errors.Is(err, redis.Nil) {
			// 缓存中不存在会话信息，从数据库查询
			var session model.Session
			if res := dao.GormDB.Where("send_id = ? and receive_id = ?", req.SendId, req.ReceiveId).First(&session); res.Error != nil {
				if errors.Is(res.Error, gorm.ErrRecordNotFound) {
					// 数据库中也不存在该会话记录，创建新的会话
					zlog.Info("会话没有找到，将新建会话")
					createReq := request.CreateSessionRequest{
						SendId:    req.SendId,
						ReceiveId: req.ReceiveId,
					}

					// 调用CreateSession方法创建新会话
					return s.CreateSession(createReq)
				}
			}

			// TODO: 以下代码被注释，可能用于将来将查询到的会话信息存入Redis缓存
			rspString, err := json.Marshal(session)
			if err != nil {
				zlog.Error(err.Error())
			}

			if err := myredis.SetKeyEx("session_"+req.SendId+"_"+req.ReceiveId+"_"+session.Uuid, string(rspString), time.Minute*constants.REDIS_TIMEOUT); err != nil {
				zlog.Error(err.Error())
			}

			return "会话创建成功", session.Uuid, 0
		} else {
			// Redis连接或其他错误
			zlog.Error(err.Error())
			return constants.SYSTEM_ERROR, "", -1
		}
	}

	// 从Redis缓存获取会话信息成功，将JSON字符串反序列化为Session对象
	var session model.Session
	if err := json.Unmarshal([]byte(rspString), &session); err != nil {
		zlog.Error(err.Error())
	}

	return "会话创建成功", session.Uuid, 0
}

// GetUserSessionList 获取用户会话列表
// 功能：获取指定用户与其他用户之间的会话列表，优先从Redis缓存获取，若缓存不存在则从数据库查询
// 参数：ownerId - 用户ID，表示要获取哪个用户的会话列表
// 返回值:
//   - string: 操作结果消息，成功或失败的具体描述
//   - []respond.UserSessionListRespond: 用户会话列表响应对象数组
//   - int: 状态码，0表示成功，-1表示系统错误
func (s *sessionService) GetUserSessionList(ownerId string) (string, []respond.UserSessionListRespond, int) {
	// 尝试从Redis缓存中获取用户会话列表
	rspString, err := myredis.GetKeyNilIsErr("session_list_" + ownerId)
	if err != nil {
		if errors.Is(err, redis.Nil) {
			// 缓存中不存在，从数据库查询用户的所有会话记录，按创建时间倒序排列
			var sessionList []model.Session
			if res := dao.GormDB.Order("created_at DESC").Where("send_id = ?", ownerId).Find(&sessionList); res.Error != nil {
				if errors.Is(res.Error, gorm.ErrRecordNotFound) {
					// 没有找到任何会话记录
					zlog.Info("未创建用户会话")
					return "未创建用户会话", nil, 0
				} else {
					// 数据库查询错误
					zlog.Error(res.Error.Error())
					return constants.SYSTEM_ERROR, nil, -1
				}
			}

			// 创建用户会话列表响应对象数组
			var sessionListRsp []respond.UserSessionListRespond
			// 遍历会话列表，筛选出与用户之间的会话（接收者ID以'U'开头表示用户）
			for _, session := range sessionList {
				if session.ReceiveId[0] == 'U' {
					// 创建用户会话响应对象，包含会话ID、头像、用户ID和用户名
					sessionListRsp = append(sessionListRsp, respond.UserSessionListRespond{
						SessionId: session.Uuid,        // 会话UUID
						Avatar:    session.Avatar,      // 接收方头像
						UserId:    session.ReceiveId,   // 接收方用户ID
						Username:  session.ReceiveName, // 接收方用户名
					})
				}
			}

			// 将用户会话列表响应对象数组序列化为JSON字符串
			rspString, err := json.Marshal(sessionListRsp)
			if err != nil {
				// 序列化失败，记录错误日志
				zlog.Error(err.Error())
			}

			// 将序列化的会话列表存入Redis缓存，设置过期时间
			if err := myredis.SetKeyEx("session_list_"+ownerId, string(rspString), time.Minute*constants.REDIS_TIMEOUT); err != nil {
				// 缓存存储失败，记录错误日志
				zlog.Error(err.Error())
			}

			return "获取成功", sessionListRsp, 0
		} else {
			// Redis连接错误
			zlog.Error(err.Error())
			return constants.SYSTEM_ERROR, nil, -1
		}
	}

	// 从Redis缓存获取数据成功，将JSON字符串反序列化为响应对象数组
	var rsp []respond.UserSessionListRespond
	if err := json.Unmarshal([]byte(rspString), &rsp); err != nil {
		// 反序列化失败，记录错误日志
		zlog.Error(err.Error())
	}

	return "获取成功", rsp, 0
}

// GetGroupSessionList 获取群聊会话列表
// 功能：获取指定用户加入的群聊会话列表，优先从Redis缓存获取，若缓存不存在则从数据库查询
// 参数：ownerId - 用户ID，表示要获取哪个用户的群聊会话列表
// 返回值:
//   - string: 操作结果消息，成功或失败的具体描述
//   - []respond.GroupSessionListRespond: 群聊会话列表响应对象数组
//   - int: 状态码，0表示成功，-1表示系统错误
func (s *sessionService) GetGroupSessionList(ownerId string) (string, []respond.GroupSessionListRespond, int) {
	// 尝试从Redis缓存中获取用户的群聊会话列表
	rspString, err := myredis.GetKeyNilIsErr("group_session_list_" + ownerId)
	if err != nil {
		if errors.Is(err, redis.Nil) {
			// 缓存中不存在，从数据库查询用户的所有会话记录，按创建时间倒序排列
			var sessionList []model.Session
			if res := dao.GormDB.Order("created_at DESC").Where("send_id = ?", ownerId).Find(&sessionList); res.Error != nil {
				if errors.Is(res.Error, gorm.ErrRecordNotFound) {
					// 没有找到任何会话记录
					zlog.Info("未创建群聊会话")
					return "未创建群聊会话", nil, 0
				} else {
					// 数据库查询错误
					zlog.Error(res.Error.Error())
					return constants.SYSTEM_ERROR, nil, -1
				}
			}

			// 创建群聊会话列表响应对象数组
			var sessionListRsp []respond.GroupSessionListRespond
			// 遍历会话列表，筛选出与群聊之间的会话（接收者ID以'G'开头表示群聊）
			for _, session := range sessionList {
				if session.ReceiveId[0] == 'G' {
					// 创建群聊会话响应对象，包含会话ID、头像、群聊ID和群聊名称
					sessionListRsp = append(sessionListRsp, respond.GroupSessionListRespond{
						SessionId: session.Uuid,        // 会话UUID
						Avatar:    session.Avatar,      // 接收方头像
						GroupId:   session.ReceiveId,   // 接收方群聊ID
						GroupName: session.ReceiveName, // 接收方群聊名称
					})
				}
			}

			// 将群聊会话列表响应对象数组序列化为JSON字符串
			rspString, err := json.Marshal(sessionListRsp)
			if err != nil {
				// 序列化失败，记录错误日志
				zlog.Error(err.Error())
			}

			// 将序列化的会话列表存入Redis缓存，设置过期时间
			if err := myredis.SetKeyEx("group_session_list_"+ownerId, string(rspString), time.Minute*constants.REDIS_TIMEOUT); err != nil {
				// 缓存存储失败，记录错误日志
				zlog.Error(err.Error())
			}

			return "获取成功", sessionListRsp, 0
		} else {
			// Redis连接错误
			zlog.Error(err.Error())
			return constants.SYSTEM_ERROR, nil, -1
		}
	}

	// 从Redis缓存获取数据成功，将JSON字符串反序列化为响应对象数组
	var rsp []respond.GroupSessionListRespond
	if err := json.Unmarshal([]byte(rspString), &rsp); err != nil {
		// 反序列化失败，记录错误日志
		zlog.Error(err.Error())
	}

	return "获取成功", rsp, 0
}

// DeleteSession 删除会话
// 功能：软删除指定的会话记录，并清除相关的缓存数据
// 参数：ownerId - 用户ID，sessionId - 要删除的会话ID
// 返回值:
//   - string: 操作结果消息，成功或失败的具体描述
//   - int: 状态码，0表示成功，-1表示系统错误
func (s *sessionService) DeleteSession(ownerId, sessionId string) (string, int) {

	// 查询要删除的会话记录
	var session model.Session
	if res := dao.GormDB.Where("uuid = ?", sessionId).Find(&session); res.Error != nil {
		zlog.Error(res.Error.Error())
		return constants.SYSTEM_ERROR, -1
	}

	// 设置软删除标志，标记为已删除状态
	session.DeletedAt.Valid = true
	session.DeletedAt.Time = time.Now()
	// 保存更新后的会话记录到数据库
	if res := dao.GormDB.Save(&session); res.Error != nil {
		zlog.Error(res.Error.Error())
		return constants.SYSTEM_ERROR, -1
	}

	// TODO: 注释掉的部分可能用于按会话ID清除特定缓存键
	if err := myredis.DelKeysWithSuffix(sessionId); err != nil {
		zlog.Error(err.Error())
	}

	// 清除用户的群聊会话列表缓存，确保下次获取时能反映最新状态
	if err := myredis.DelKeysWithPattern("group_session_list_" + ownerId); err != nil {
		zlog.Error(err.Error())
	}
	// 清除用户的会话列表缓存，确保下次获取时能反映最新状态
	if err := myredis.DelKeysWithPattern("session_list_" + ownerId); err != nil {
		zlog.Error(err.Error())
	}

	return "删除成功", 0
}

// CheckOpenSessionAllowed 检查是否允许发起会话
// 功能：检查两个用户之间或用户与群聊之间是否允许建立会话，验证双方状态和关系
// 参数：sendId - 发送者ID，receiveId - 接收者ID
// 返回值:
//   - string: 操作结果消息，成功或失败的具体描述
//   - bool: 是否允许发起会话，true表示允许，false表示不允许
//   - int: 状态码，0表示成功，-1表示系统错误，-2表示业务验证失败
func (s *sessionService) CheckOpenSessionAllowed(sendId, receiveId string) (string, bool, int) {
	// 查询发送者与接收者之间的联系人关系
	var contact model.UserContact
	if res := dao.GormDB.Where("user_id = ? and contact_id = ?", sendId, receiveId).First(&contact); res.Error != nil {
		zlog.Error(res.Error.Error())
		return constants.SYSTEM_ERROR, false, -1
	}

	// 检查联系人状态，判断是否允许发起会话
	switch contact.Status {
	case contact_status_enum.BE_BLACK:
		// 如果发送者被接收者拉黑，则不允许发起会话
		return "已被对方拉黑，无法发起会话", false, -2
	case contact_status_enum.BLACK:
		// 如果发送者拉黑了接收者，则不允许发起会话
		return "已拉黑对方，先解除拉黑状态才能发起会话", false, -2
	}

	// 根据接收者ID的第一个字符判断接收者类型（用户或群聊）
	if receiveId[0] == 'U' {
		// 接收者为用户，检查用户状态
		var user model.UserInfo
		if res := dao.GormDB.Where("uuid = ?", receiveId).First(&user); res.Error != nil {
			zlog.Error(res.Error.Error())
			return constants.SYSTEM_ERROR, false, -1
		}

		// 检查接收用户是否被禁用
		if user.Status == user_status_enum.DISABLE {
			zlog.Info("对方已被禁用，无法发起会话")
			return "对方已被禁用，无法发起会话", false, -2
		}
	} else {
		// 接收者为群聊，检查群聊状态
		var group model.GroupInfo
		if res := dao.GormDB.Where("uuid = ?", receiveId).First(&group); res.Error != nil {
			zlog.Error(res.Error.Error())
			return constants.SYSTEM_ERROR, false, -1
		}

		// 检查接收群聊是否被禁用
		if group.Status == group_status_enum.DISABLE {
			zlog.Info("对方已被禁用，无法发起会话")
			return "对方已被禁用，无法发起会话", false, -2
		}
	}

	// 所有条件都满足，允许发起会话
	return "可以发起会话", true, 0
}
