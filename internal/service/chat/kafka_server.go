package chat

import (
	"encoding/json"
	"errors"
	"fmt"
	"gochat/internal/dao"
	"gochat/internal/dto/request"
	"gochat/internal/dto/respond"
	"gochat/internal/model"
	"gochat/internal/service/kafka"
	myredis "gochat/internal/service/redis"
	"gochat/pkg/constants"
	"gochat/pkg/enum/message/message_status_enum"
	"gochat/pkg/enum/message/message_type_enum"
	"gochat/pkg/util/random"
	"gochat/pkg/zlog"
	"log"
	"os"
	"sync"
	"time"

	"github.com/go-redis/redis/v8"
	"github.com/gorilla/websocket"
)

// KafkaServer 定义基于Kafka的聊天服务器结构
// 作用：处理从Kafka接收到的消息，管理WebSocket客户端连接，
// 并将消息路由到相应的客户端

type KafkaServer struct {
	Clients map[string]*Client // 客户端连接映射，key为客户端UUID
	mutex   *sync.Mutex        // 互斥锁，保护Clients映射的并发访问
	Login   chan *Client       // 登录通道，用于处理客户端登录
	Logout  chan *Client       // 退出登录通道，用于处理客户端登出
}

// KafkaChatServer 全局Kafka服务器实例
var KafkaChatServer *KafkaServer

// kafkaQuit 用于接收系统信号，控制服务器退出
var kafkaQuit = make(chan os.Signal, 1)

// init 初始化KafkaChatServer实例
func init() {
	if KafkaChatServer == nil {
		KafkaChatServer = &KafkaServer{
			Clients: make(map[string]*Client), // 初始化客户端映射
			mutex:   &sync.Mutex{},            // 初始化互斥锁
			Login:   make(chan *Client),       // 初始化登录通道
			Logout:  make(chan *Client),       // 初始化登出通道
		}
	}
	//signal.Notify(kafkaQuit, syscall.SIGINT, syscall.SIGTERM)
}

// Start 启动Kafka服务器，开始处理消息
// 功能：
// 1. 启动goroutine持续从Kafka读取消息并处理
// 2. 处理客户端登录和登出请求
// 3. 管理客户端连接状态
func (k *KafkaServer) Start() {
	// 延迟函数，处理panic并关闭通道
	defer func() {
		if r := recover(); r != nil {
			zlog.Error(fmt.Sprintf("kafka server panic: %v", r))
		}
		close(k.Login)  // 关闭登录通道
		close(k.Logout) // 关闭登出通道
	}()

	// 启动goroutine读取Kafka消息
	go func() {
		defer func() {
			if r := recover(); r != nil {
				zlog.Error(fmt.Sprintf("kafka server panic: %v", r))
			}
		}()

		// 无限循环，持续读取Kafka消息
		for {
			// 从Kafka读取消息
			kafkaMessage, err := kafka.KafkaService.ChatReader.ReadMessage(ctx)
			if err != nil {
				zlog.Error(err.Error())
				continue // 出错时跳过当前消息，继续处理下一条
			}

			// 记录消息详情
			log.Printf("topic=%s, partition=%d, offset=%d, key=%s, value=%s", kafkaMessage.Topic, kafkaMessage.Partition, kafkaMessage.Offset, kafkaMessage.Key, kafkaMessage.Value)
			zlog.Info(fmt.Sprintf("topic=%s, partition=%d, offset=%d, key=%s, value=%s", kafkaMessage.Topic, kafkaMessage.Partition, kafkaMessage.Offset, kafkaMessage.Key, kafkaMessage.Value))

			// 解析消息
			data := kafkaMessage.Value
			var chatMessageReq request.ChatMessageRequest
			if err := json.Unmarshal(data, &chatMessageReq); err != nil {
				zlog.Error(err.Error())
				continue // 解析失败时跳过当前消息
			}
			// log.Println("原消息为：", data, "反序列化后为：", chatMessageReq)

			// 根据消息类型处理
			switch chatMessageReq.Type {
			case message_type_enum.Text:
				// 处理文本消息
				// 1. 创建消息模型
				message := model.Message{
					Uuid:       fmt.Sprintf("M%s", random.GetNowAndLenRandomString(11)), // 生成唯一消息ID
					SessionId:  chatMessageReq.SessionId,                                // 会话ID
					Type:       chatMessageReq.Type,                                     // 消息类型：文本
					Content:    chatMessageReq.Content,                                  // 消息内容
					Url:        "",                                                      // 文本消息无URL
					SendId:     chatMessageReq.SendId,                                   // 发送者ID
					SendName:   chatMessageReq.SendName,                                 // 发送者姓名
					SendAvatar: chatMessageReq.SendAvatar,                               // 发送者头像
					ReceiveId:  chatMessageReq.ReceiveId,                                // 接收者ID
					FileSize:   "0B",                                                    // 文本消息无文件大小
					FileType:   "",                                                      // 文本消息无文件类型
					FileName:   "",                                                      // 文本消息无文件名
					Status:     message_status_enum.Unsent,                              // 消息状态：未发送
					CreatedAt:  time.Now(),                                              // 消息创建时间
					AVdata:     "",                                                      // 文本消息无音视频数据
				}

				// 对SendAvatar去除前面/static之前的所有内容，防止ip前缀引入
				message.SendAvatar = normalizePath(message.SendAvatar)
				// 2. 保存消息到数据库
				if res := dao.GormDB.Create(&message); res.Error != nil {
					zlog.Error(res.Error.Error())
					continue // 保存失败时跳过当前消息
				}

				// 3. 根据接收者ID首字母判断消息类型：'U'为用户私聊，'G'为群聊
				switch message.ReceiveId[0] {
				case 'U': // 发送给User（用户私聊）
					// 4. 构建消息响应
					messageRsp := respond.GetMessageListRespond{
						SendId:     message.SendId,
						SendName:   message.SendName,
						SendAvatar: chatMessageReq.SendAvatar,
						ReceiveId:  message.ReceiveId,
						Type:       message.Type,
						Content:    message.Content,
						Url:        message.Url,
						FileSize:   message.FileSize,
						FileName:   message.FileName,
						FileType:   message.FileType,
						CreatedAt:  message.CreatedAt.Format("2006-01-02 15:04:05"),
					}

					// 5. 序列化消息响应
					jsonMessage, err := json.Marshal(messageRsp)
					if err != nil {
						zlog.Error(err.Error())
						continue
					}
					// log.Println("返回的消息为：", messageRsp, "序列化后为：", jsonMessage)

					// 6. 构建消息回显结构
					var messageBack = &MessageBack{
						Message: jsonMessage,
						Uuid:    message.Uuid,
					}

					// 7. 发送消息给接收者和发送者
					k.mutex.Lock()
					if receiveClient, ok := k.Clients[message.ReceiveId]; ok {
						receiveClient.SendBack <- messageBack // 向接收者发送消息
					}
					// 因为send_id肯定在线，所以这里在后端进行在线回显message，其实优化的话前端可以直接回显
					// 问题在于前后端的req和rsp结构不同，前端存储message的messageList不能存req，只能存rsp
					// 所以这里后端进行回显，前端不回显
					if sendClient, ok := k.Clients[message.SendId]; ok {
						sendClient.SendBack <- messageBack // 发送者也收到消息回显
					}
					k.mutex.Unlock()

					// 8. 更新Redis缓存中的私聊消息列表
					var rspString string
					rspString, err = myredis.GetKeyNilIsErr("message_list_" + message.SendId + "_" + message.ReceiveId)
					if err == nil {
						var rsp []respond.GetMessageListRespond
						if err := json.Unmarshal([]byte(rspString), &rsp); err != nil {
							zlog.Error(err.Error())
						} else {
							rsp = append(rsp, messageRsp) // 添加新消息到私聊消息列表
							rspByte, err := json.Marshal(rsp)
							if err != nil {
								zlog.Error(err.Error())
							} else {
								if err := myredis.SetKeyEx("message_list_"+message.SendId+"_"+message.ReceiveId, string(rspByte), time.Minute*constants.REDIS_TIMEOUT); err != nil {
									zlog.Error(err.Error())
								}
							}
						}
					} else {
						if !errors.Is(err, redis.Nil) { // 如果不是因为键不存在导致的错误
							zlog.Error(err.Error())
						}
					}

				case 'G': // 发送给Group（群聊）
					// 4. 构建群组消息响应
					messageRsp := respond.GetGroupMessageListRespond{
						SendId:     message.SendId,
						SendName:   message.SendName,
						SendAvatar: chatMessageReq.SendAvatar,
						ReceiveId:  message.ReceiveId,
						Type:       message.Type,
						Content:    message.Content,
						Url:        message.Url,
						FileSize:   message.FileSize,
						FileName:   message.FileName,
						FileType:   message.FileType,
						CreatedAt:  message.CreatedAt.Format("2006-01-02 15:04:05"),
					}

					// 5. 序列化消息响应
					jsonMessage, err := json.Marshal(messageRsp)
					if err != nil {
						zlog.Error(err.Error())
						continue
					}
					// log.Println("返回的消息为：", messageRsp, "序列化后为：", jsonMessage)

					// 6. 构建消息回显结构
					var messageBack = &MessageBack{
						Message: jsonMessage,
						Uuid:    message.Uuid,
					}

					// 7. 查询群组信息，获取群组成员列表
					var group model.GroupInfo
					if res := dao.GormDB.Where("uuid = ?", message.ReceiveId).First(&group); res.Error != nil {
						zlog.Error(res.Error.Error())
						continue
					}

					// 8. 解析群组成员列表
					var members []string
					if err := json.Unmarshal(group.Members, &members); err != nil {
						zlog.Error(err.Error())
						continue
					}

					// 9. 向群组所有成员发送消息
					k.mutex.Lock()
					for _, member := range members {
						if member != message.SendId {
							if receiveClient, ok := k.Clients[member]; ok {
								receiveClient.SendBack <- messageBack // 向群组其他成员发送消息
							}
						} else {
							if sendClient, ok := k.Clients[message.SendId]; ok {
								sendClient.SendBack <- messageBack // 发送者也收到消息回显
							}
						}
					}
					k.mutex.Unlock()

					// 10. 更新Redis缓存中的群组消息列表
					var rspString string
					rspString, err = myredis.GetKeyNilIsErr("group_messagelist_" + message.ReceiveId)
					if err == nil {
						var rsp []respond.GetGroupMessageListRespond
						if err := json.Unmarshal([]byte(rspString), &rsp); err != nil {
							zlog.Error(err.Error())
						} else {
							rsp = append(rsp, messageRsp) // 添加新消息到群组消息列表
							rspByte, err := json.Marshal(rsp)
							if err != nil {
								zlog.Error(err.Error())
							} else {
								if err := myredis.SetKeyEx("group_messagelist_"+message.ReceiveId, string(rspByte), time.Minute*constants.REDIS_TIMEOUT); err != nil {
									zlog.Error(err.Error())
								}
							}
						}
					} else {
						if !errors.Is(err, redis.Nil) { // 如果不是因为键不存在导致的错误
							zlog.Error(err.Error())
						}
					}
				}

			case message_type_enum.File:
				// 处理文件消息
				// 1. 创建消息模型
				message := model.Message{
					Uuid:       fmt.Sprintf("M%s", random.GetNowAndLenRandomString(11)), // 生成唯一消息ID
					SessionId:  chatMessageReq.SessionId,                                // 会话ID
					Type:       chatMessageReq.Type,                                     // 消息类型：文件
					Content:    "",                                                      // 文件消息无内容字段
					Url:        chatMessageReq.Url,                                      // 文件URL
					SendId:     chatMessageReq.SendId,                                   // 发送者ID
					SendName:   chatMessageReq.SendName,                                 // 发送者姓名
					SendAvatar: chatMessageReq.SendAvatar,                               // 发送者头像
					ReceiveId:  chatMessageReq.ReceiveId,                                // 接收者ID
					FileSize:   chatMessageReq.FileSize,                                 // 文件大小
					FileType:   chatMessageReq.FileType,                                 // 文件类型
					FileName:   chatMessageReq.FileName,                                 // 文件名
					Status:     message_status_enum.Unsent,                              // 消息状态：未发送
					CreatedAt:  time.Now(),                                              // 消息创建时间
					AVdata:     "",                                                      // 文件消息无音视频数据
				}
				// 对SendAvatar去除前面/static之前的所有内容，防止ip前缀引入
				message.SendAvatar = normalizePath(message.SendAvatar)

				// 2. 保存消息到数据库
				if res := dao.GormDB.Create(&message); res.Error != nil {
					zlog.Error(res.Error.Error())
					continue
				}

				if message.ReceiveId[0] == 'U' { // 发送给User（用户私聊）
					// 3. 构建消息响应
					messageRsp := respond.GetMessageListRespond{
						SendId:     message.SendId,
						SendName:   message.SendName,
						SendAvatar: chatMessageReq.SendAvatar,
						ReceiveId:  message.ReceiveId,
						Type:       message.Type,
						Content:    message.Content,
						Url:        message.Url,
						FileSize:   message.FileSize,
						FileName:   message.FileName,
						FileType:   message.FileType,
						CreatedAt:  message.CreatedAt.Format("2006-01-02 15:04:05"),
					}

					// 4. 序列化消息响应
					jsonMessage, err := json.Marshal(messageRsp)
					if err != nil {
						zlog.Error(err.Error())
						continue
					}
					// log.Println("返回的消息为：", messageRsp, "序列化后为：", jsonMessage)

					// 5. 构建消息回显结构
					var messageBack = &MessageBack{
						Message: jsonMessage,
						Uuid:    message.Uuid,
					}

					// 6. 发送消息给接收者和发送者
					k.mutex.Lock()
					if receiveClient, ok := k.Clients[message.ReceiveId]; ok {
						receiveClient.SendBack <- messageBack // 向接收者发送消息
					}
					// 因为send_id肯定在线，所以这里在后端进行在线回显message，其实优化的话前端可以直接回显
					// 问题在于前后端的req和rsp结构不同，前端存储message的messageList不能存req，只能存rsp
					// 所以这里后端进行回显，前端不回显
					if sendClient, ok := k.Clients[message.SendId]; ok {
						sendClient.SendBack <- messageBack // 发送者也收到消息回显
					}
					k.mutex.Unlock()

					// 7. 更新Redis缓存中的私聊消息列表
					var rspString string
					rspString, err = myredis.GetKeyNilIsErr("message_list_" + message.SendId + "_" + message.ReceiveId)
					if err == nil {
						var rsp []respond.GetMessageListRespond
						if err := json.Unmarshal([]byte(rspString), &rsp); err != nil {
							zlog.Error(err.Error())
						} else {
							rsp = append(rsp, messageRsp) // 添加新消息到私聊消息列表
							rspByte, err := json.Marshal(rsp)
							if err != nil {
								zlog.Error(err.Error())
							} else {
								if err := myredis.SetKeyEx("message_list_"+message.SendId+"_"+message.ReceiveId, string(rspByte), time.Minute*constants.REDIS_TIMEOUT); err != nil {
									zlog.Error(err.Error())
								}
							}
						}
					} else {
						if !errors.Is(err, redis.Nil) { // 如果不是因为键不存在导致的错误
							zlog.Error(err.Error())
						}
					}
				} else { // 发送给Group（群聊）
					// 3. 构建群组消息响应
					messageRsp := respond.GetGroupMessageListRespond{
						SendId:     message.SendId,
						SendName:   message.SendName,
						SendAvatar: chatMessageReq.SendAvatar,
						ReceiveId:  message.ReceiveId,
						Type:       message.Type,
						Content:    message.Content,
						Url:        message.Url,
						FileSize:   message.FileSize,
						FileName:   message.FileName,
						FileType:   message.FileType,
						CreatedAt:  message.CreatedAt.Format("2006-01-02 15:04:05"),
					}

					// 4. 序列化消息响应
					jsonMessage, err := json.Marshal(messageRsp)
					if err != nil {
						zlog.Error(err.Error())
						continue
					}
					// log.Println("返回的消息为：", messageRsp, "序列化后为：", jsonMessage)

					// 5. 构建消息回显结构
					var messageBack = &MessageBack{
						Message: jsonMessage,
						Uuid:    message.Uuid,
					}

					// 6. 查询群组信息，获取群组成员列表
					var group model.GroupInfo
					if res := dao.GormDB.Where("uuid = ?", message.ReceiveId).First(&group); res.Error != nil {
						zlog.Error(res.Error.Error())
						continue
					}

					// 7. 解析群组成员列表
					var members []string
					if err := json.Unmarshal(group.Members, &members); err != nil {
						zlog.Error(err.Error())
						continue
					}

					// 8. 向群组所有成员发送消息
					k.mutex.Lock()
					for _, member := range members {
						if member != message.SendId {
							if receiveClient, ok := k.Clients[member]; ok {
								receiveClient.SendBack <- messageBack // 向群组其他成员发送消息
							}
						} else {
							if sendClient, ok := k.Clients[message.SendId]; ok {
								sendClient.SendBack <- messageBack // 发送者也收到消息回显
							}
						}
					}
					k.mutex.Unlock()

					// 9. 更新Redis缓存中的群组消息列表
					var rspString string
					rspString, err = myredis.GetKeyNilIsErr("group_messagelist_" + message.ReceiveId)
					if err == nil {
						var rsp []respond.GetGroupMessageListRespond
						if err := json.Unmarshal([]byte(rspString), &rsp); err != nil {
							zlog.Error(err.Error())
						} else {
							rsp = append(rsp, messageRsp) // 添加新消息到群组消息列表
							rspByte, err := json.Marshal(rsp)
							if err != nil {
								zlog.Error(err.Error())
							} else {
								if err := myredis.SetKeyEx("group_messagelist_"+message.ReceiveId, string(rspByte), time.Minute*constants.REDIS_TIMEOUT); err != nil {
									zlog.Error(err.Error())
								}
							}
						}
					} else {
						if !errors.Is(err, redis.Nil) { // 如果不是因为键不存在导致的错误
							zlog.Error(err.Error())
						}
					}
				}

			case message_type_enum.AudioOrVideo:
				// 处理音视频消息
				// 1. 解析音视频数据
				var avData request.AVData
				if err := json.Unmarshal([]byte(chatMessageReq.AVdata), &avData); err != nil {
					zlog.Error(err.Error())
					continue
				}
				//log.Println(avData)

				// 2. 创建消息模型
				message := model.Message{
					Uuid:       fmt.Sprintf("M%s", random.GetNowAndLenRandomString(11)), // 生成唯一消息ID
					SessionId:  chatMessageReq.SessionId,                                // 会话ID
					Type:       chatMessageReq.Type,                                     // 消息类型：音视频
					Content:    "",                                                      // 音视频消息内容为空
					Url:        "",                                                      // 音视频消息URL为空
					SendId:     chatMessageReq.SendId,                                   // 发送者ID
					SendName:   chatMessageReq.SendName,                                 // 发送者姓名
					SendAvatar: chatMessageReq.SendAvatar,                               // 发送者头像
					ReceiveId:  chatMessageReq.ReceiveId,                                // 接收者ID
					FileSize:   "",                                                      // 音视频消息文件大小为空
					FileType:   "",                                                      // 音视频消息文件类型为空
					FileName:   "",                                                      // 音视频消息文件名为空
					Status:     message_status_enum.Unsent,                              // 消息初始状态为未发送
					CreatedAt:  time.Now(),                                              // 消息创建时间
					AVdata:     chatMessageReq.AVdata,                                   // 音视频数据
				}

				// 3. 只有当音视频消息是通话相关操作时才保存到数据库
				if avData.MessageId == "PROXY" && (avData.Type == "start_call" || avData.Type == "receive_call" || avData.Type == "reject_call") {
					// 对SendAvatar去除前面/static之前的所有内容，防止ip前缀引入
					message.SendAvatar = normalizePath(message.SendAvatar)
					if res := dao.GormDB.Create(&message); res.Error != nil {
						zlog.Error(res.Error.Error())
					}
				}

				if chatMessageReq.ReceiveId[0] == 'U' { // 发送给User（用户私聊）
					// 4. 构建音视频消息响应
					messageRsp := respond.AVMessageRespond{
						SendId:     message.SendId,
						SendName:   message.SendName,
						SendAvatar: message.SendAvatar,
						ReceiveId:  message.ReceiveId,
						Type:       message.Type,
						Content:    message.Content,
						Url:        message.Url,
						FileSize:   message.FileSize,
						FileName:   message.FileName,
						FileType:   message.FileType,
						CreatedAt:  message.CreatedAt.Format("2006-01-02 15:04:05"),
						AVdata:     message.AVdata,
					}

					// 5. 序列化消息响应
					jsonMessage, err := json.Marshal(messageRsp)
					if err != nil {
						zlog.Error(err.Error())
						continue
					}
					// log.Println("返回的消息为：", messageRsp, "序列化后为：", jsonMessage)
					// log.Println("返回的消息为：", messageRsp)

					// 6. 构建消息回显结构
					var messageBack = &MessageBack{
						Message: jsonMessage,
						Uuid:    message.Uuid,
					}

					// 7. 发送消息给接收者
					k.mutex.Lock()
					if receiveClient, ok := k.Clients[message.ReceiveId]; ok {
						receiveClient.SendBack <- messageBack // 向接收者发送消息
					}
					// 通话这不能回显，发回去的话就会出现两个start_call。
					//sendClient := s.Clients[message.SendId]
					//sendClient.SendBack <- messageBack
					k.mutex.Unlock()
				}
			}
		}
	}()

	// 处理客户端登录和登出消息
	for {
		select {
		case client := <-k.Login:
			{
				// 客户端登录处理
				k.mutex.Lock()
				k.Clients[client.Uuid] = client // 将客户端添加到映射中
				k.mutex.Unlock()
				zlog.Debug(fmt.Sprintf("欢迎来到gochat聊天服务器，亲爱的用户%s\n", client.Uuid))
				// 向客户端发送欢迎消息
				err := client.Conn.WriteMessage(websocket.TextMessage, []byte("欢迎来到gochat聊天服务器"))
				if err != nil {
					zlog.Error(err.Error())
				}
			}

		case client := <-k.Logout:
			{
				// 客户端登出处理
				k.mutex.Lock()
				delete(k.Clients, client.Uuid) // 从映射中移除客户端
				k.mutex.Unlock()
				zlog.Info(fmt.Sprintf("用户%s退出登录\n", client.Uuid))
				// 向客户端发送登出确认消息
				if err := client.Conn.WriteMessage(websocket.TextMessage, []byte("已退出登录")); err != nil {
					zlog.Error(err.Error())
				}
			}
		}
	}
}

// Close 关闭Kafka服务器，关闭登录和登出通道
func (k *KafkaServer) Close() {
	close(k.Login)
	close(k.Logout)
}

// SendClientToLogin 将客户端发送到登录通道
// 作用：处理客户端登录请求，将客户端添加到服务器的客户端映射中
func (k *KafkaServer) SendClientToLogin(client *Client) {
	k.mutex.Lock()
	k.Login <- client
	k.mutex.Unlock()
}

// SendClientToLogout 将客户端发送到登出通道
// 作用：处理客户端登出请求，将客户端从服务器的客户端映射中移除
func (k *KafkaServer) SendClientToLogout(client *Client) {
	k.mutex.Lock()
	k.Logout <- client
	k.mutex.Unlock()
}

// RemoveClient 从客户端映射中移除指定UUID的客户端
// 作用：强制移除客户端连接，通常在客户端异常断开时使用
func (k *KafkaServer) RemoveClient(uuid string) {
	k.mutex.Lock()
	delete(k.Clients, uuid)
	k.mutex.Unlock()
}
