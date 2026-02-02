package gorm

import (
	"encoding/json"
	"errors"
	"fmt"
	"gochat/internal/config"
	"gochat/internal/dao"
	"gochat/internal/dto/respond"
	"gochat/internal/model"
	myredis "gochat/internal/service/redis"
	"gochat/pkg/constants"
	"gochat/pkg/zlog"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/go-redis/redis/v8"
)

type messageService struct {
}

var MessageService = new(messageService)

// GetMessageList 获取聊天记录
// 功能：获取两个用户之间的聊天记录，优先从Redis缓存获取，若缓存不存在则从数据库查询
// 参数：userOneId - 第一个用户ID，userTwoId - 第二个用户ID
// 返回值:
//   - string: 操作结果消息，成功或失败的具体描述
//   - []respond.GetMessageListRespond: 聊天记录响应对象数组
//   - int: 状态码，0表示成功，-1表示系统错误
func (m *messageService) GetMessageList(userOneId, userTwoId string) (string, []respond.GetMessageListRespond, int) {
	// 尝试从Redis缓存中获取两个用户之间的聊天记录
	rspString, err := myredis.GetKeyNilIsErr("message_list_" + userOneId + "_" + userTwoId)
	if err != nil {
		if errors.Is(err, redis.Nil) {
			// 缓存中不存在，从数据库查询两个用户之间的所有消息记录，按创建时间升序排列
			zlog.Info(err.Error())
			zlog.Info(fmt.Sprintf("%s %s", userOneId, userTwoId))

			var messageList []model.Message
			// 查询条件：(userOneId发送给userTwoId的消息) OR (userTwoId发送给userOneId的消息)
			if res := dao.GormDB.Where("(send_id = ? AND receive_id = ?) OR (send_id = ? AND receive_id = ?)", userOneId, userTwoId, userTwoId, userOneId).Order("created_at ASC").Find(&messageList); res.Error != nil {
				// 数据库查询错误
				zlog.Error(res.Error.Error())
				return constants.SYSTEM_ERROR, nil, -1
			}

			// 构建聊天记录响应对象数组
			var rspList []respond.GetMessageListRespond
			// 遍历消息列表，将每条消息转换为响应对象
			for _, message := range messageList {
				// 创建消息响应对象，包含发送者ID、发送者姓名、发送者头像、接收者ID、内容、URL、类型、文件类型、文件名、文件大小和创建时间
				rspList = append(rspList, respond.GetMessageListRespond{
					SendId:     message.SendId,                                  // 发送者ID
					SendName:   message.SendName,                                // 发送者姓名
					SendAvatar: message.SendAvatar,                              // 发送者头像
					ReceiveId:  message.ReceiveId,                               // 接收者ID
					Content:    message.Content,                                 // 消息内容
					Url:        message.Url,                                     // 消息URL
					Type:       message.Type,                                    // 消息类型
					FileType:   message.FileType,                                // 文件类型
					FileName:   message.FileName,                                // 文件名
					FileSize:   message.FileSize,                                // 文件大小
					CreatedAt:  message.CreatedAt.Format("2006-01-02 15:04:05"), // 创建时间，格式化为"年-月-日 时:分:秒"
				})
			}

			// TODO: 以下代码被注释，可能是为了将来将查询到的聊天记录存入Redis缓存
			rspString, err := json.Marshal(rspList)
			if err != nil {
				zlog.Error(err.Error())
			}

			if err := myredis.SetKeyEx("message_list_"+userOneId+"_"+userTwoId, string(rspString), time.Minute*constants.REDIS_TIMEOUT); err != nil {
				zlog.Error(err.Error())
			}
			return "获取聊天记录成功", rspList, 0
		} else {
			// Redis连接错误
			zlog.Error(err.Error())
			return constants.SYSTEM_ERROR, nil, -1
		}
	}

	// 从Redis缓存获取数据成功，将JSON字符串反序列化为响应对象数组
	var rsp []respond.GetMessageListRespond
	if err := json.Unmarshal([]byte(rspString), &rsp); err != nil {
		// 反序列化失败，记录错误日志
		zlog.Error(err.Error())
	}

	return "获取群聊记录成功", rsp, 0
}

// GetGroupMessageList 获取群聊消息记录
// 功能：获取指定群聊的消息记录，优先从Redis缓存获取，若缓存不存在则从数据库查询
// 参数：groupId - 群聊ID
// 返回值:
//   - string: 操作结果消息，成功或失败的具体描述
//   - []respond.GetGroupMessageListRespond: 群聊消息记录响应对象数组
//   - int: 状态码，0表示成功，-1表示系统错误
func (m *messageService) GetGroupMessageList(groupId string) (string, []respond.GetGroupMessageListRespond, int) {
	// 尝试从Redis缓存中获取群聊消息记录
	rspString, err := myredis.GetKeyNilIsErr("group_messagelist_" + groupId)
	if err != nil {
		if errors.Is(err, redis.Nil) {
			// 缓存中不存在，从数据库查询指定群聊的所有消息记录，按创建时间升序排列
			var messageList []model.Message
			if res := dao.GormDB.Where("receive_id = ?", groupId).Order("created_at ASC").Find(&messageList); res.Error != nil {
				// 数据库查询错误
				zlog.Error(res.Error.Error())
				return constants.SYSTEM_ERROR, nil, -1
			}

			// 构建群聊消息记录响应对象数组
			var rspList []respond.GetGroupMessageListRespond
			// 遍历消息列表，将每条消息转换为响应对象
			for _, message := range messageList {
				// 创建群聊消息响应对象，包含发送者ID、发送者姓名、发送者头像、接收者ID、内容、URL、类型、文件类型、文件名、文件大小和创建时间
				rsp := respond.GetGroupMessageListRespond{
					SendId:     message.SendId,                                  // 发送者ID
					SendName:   message.SendName,                                // 发送者姓名
					SendAvatar: message.SendAvatar,                              // 发送者头像
					ReceiveId:  message.ReceiveId,                               // 接收者ID（群聊ID）
					Content:    message.Content,                                 // 消息内容
					Url:        message.Url,                                     // 消息URL
					Type:       message.Type,                                    // 消息类型
					FileType:   message.FileType,                                // 文件类型
					FileName:   message.FileName,                                // 文件名
					FileSize:   message.FileSize,                                // 文件大小
					CreatedAt:  message.CreatedAt.Format("2006-01-02 15:04:05"), // 创建时间，格式化为"年-月-日 时:分:秒"
				}
				// 将单条消息响应对象添加到响应对象数组中
				rspList = append(rspList, rsp)
			}

			// TODO: 以下代码被注释，可能是为了将来将查询到的群聊消息记录存入Redis缓存
			rspString, err := json.Marshal(rspList)
			if err != nil {
				zlog.Error(err.Error())
			}

			if err := myredis.SetKeyEx("group_messagelist_"+groupId, string(rspString), time.Minute*constants.REDIS_TIMEOUT); err != nil {
				zlog.Error(err.Error())
			}
			return "获取聊天记录成功", rspList, 0
		} else {
			// Redis连接错误
			zlog.Error(err.Error())
			return constants.SYSTEM_ERROR, nil, -1
		}
	}

	// 从Redis缓存获取数据成功，将JSON字符串反序列化为响应对象数组
	var rsp []respond.GetGroupMessageListRespond
	if err := json.Unmarshal([]byte(rspString), &rsp); err != nil {
		// 反序列化失败，记录错误日志
		zlog.Error(err.Error())
	}

	return "获取聊天记录成功", rsp, 0
}

// UploadAvatar 上传头像
// 功能：处理用户上传头像的请求，将上传的文件保存到本地指定目录
// 参数：c - Gin框架的上下文对象，包含HTTP请求的相关信息
// 返回值:
//   - string: 操作结果消息，成功或失败的具体描述
//   - int: 状态码，0表示成功，-1表示系统错误
func (m *messageService) UploadAvatar(c *gin.Context) (string, int) {
	// 解析multipart form数据，设置最大文件大小限制
	if err := c.Request.ParseMultipartForm(constants.FILE_MAX_SIZE); err != nil {
		zlog.Error(err.Error())
		return constants.SYSTEM_ERROR, -1
	}

	// 获取multipart form数据
	mForm := c.Request.MultipartForm
	// 遍历上传的文件
	for key := range mForm.File {
		// 根据键名获取上传的文件
		file, fileHeader, err := c.Request.FormFile(key)
		if err != nil {
			zlog.Error(err.Error())
			return constants.SYSTEM_ERROR, -1
		}

		// 确保文件在函数结束时关闭
		defer file.Close()

		// 记录上传的文件名和文件大小
		zlog.Info(fmt.Sprintf("文件名：%s，文件大小：%d", fileHeader.Filename, fileHeader.Size))
		// 获取文件扩展名
		// 原来Filename应该是213451545.xxx，将Filename修改为avatar_ownerId.xxx
		ext := filepath.Ext(fileHeader.Filename)
		zlog.Info(ext)

		// 构建本地文件路径，将文件保存到配置的头像静态路径下
		localFileName := config.GetConfig().StaticAvatarPath + "/" + fileHeader.Filename
		// 创建本地文件用于保存上传的头像
		out, err := os.Create(localFileName)
		if err != nil {
			zlog.Error(err.Error())
			return constants.SYSTEM_ERROR, -1
		}

		// 确保输出文件在函数结束时关闭
		defer out.Close()
		// 将上传的文件内容复制到本地文件
		if _, err := io.Copy(out, file); err != nil {
			zlog.Error(err.Error())
			return constants.SYSTEM_ERROR, -1
		}

		// 记录文件上传完成日志
		zlog.Info("完成文件上传")
	}

	return "上传成功", 0
}

// UploadFile 上传文件
// 功能：处理用户上传文件的请求，将上传的文件保存到本地指定目录
// 参数：c - Gin框架的上下文对象，包含HTTP请求的相关信息
// 返回值:
//   - string: 操作结果消息，成功或失败的具体描述
//   - int: 状态码，0表示成功，-1表示系统错误
func (m *messageService) UploadFile(c *gin.Context) (string, int) {
	// 解析multipart form数据，设置最大文件大小限制
	if err := c.Request.ParseMultipartForm(constants.FILE_MAX_SIZE); err != nil {
		zlog.Error(err.Error())
		return constants.SYSTEM_ERROR, -1
	}

	// 获取multipart form数据
	mForm := c.Request.MultipartForm
	// 遍历上传的文件
	for key := range mForm.File {
		// 根据键名获取上传的文件
		file, fileHeader, err := c.Request.FormFile(key)
		if err != nil {
			zlog.Error(err.Error())
			return constants.SYSTEM_ERROR, -1
		}

		// 确保文件在函数结束时关闭
		defer file.Close()

		// 记录上传的文件名和文件大小
		zlog.Info(fmt.Sprintf("文件名：%s，文件大小：%d", fileHeader.Filename, fileHeader.Size))
		// 获取文件扩展名
		// 原来Filename应该是213451545.xxx，将Filename修改为avatar_ownerId.xxx
		ext := filepath.Ext(fileHeader.Filename)
		zlog.Info(ext)

		// 构建本地文件路径，将文件保存到配置的文件静态路径下
		localFileName := config.GetConfig().StaticFilePath + "/" + fileHeader.Filename
		// 创建本地文件用于保存上传的文件
		out, err := os.Create(localFileName)
		if err != nil {
			zlog.Error(err.Error())
			return constants.SYSTEM_ERROR, -1
		}

		// 确保输出文件在函数结束时关闭
		defer out.Close()

		// 将上传的文件内容复制到本地文件
		if _, err := io.Copy(out, file); err != nil {
			zlog.Error(err.Error())
			return constants.SYSTEM_ERROR, -1
		}

		// 记录文件上传完成日志
		zlog.Info("完成文件上传")
	}
	return "上传成功", 0
}
