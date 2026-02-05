// Package chat 实现WebSocket聊天服务器的核心功能
// 包含客户端连接管理、消息传输、群组消息处理等功能
package chat

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
	"gochat/pkg/enum/message/message_status_enum"
	"gochat/pkg/enum/message/message_type_enum"
	"gochat/pkg/util/random"
	"gochat/pkg/zlog"
	"log"
	"strings"
	"sync"
	"time"

	"github.com/go-redis/redis/v8"
	"github.com/gorilla/websocket"
)

// Server 定义聊天服务器结构体
// 管理所有客户端连接、消息传输以及登录登出事件
type Server struct {
	Clients  map[string]*Client // 存储所有已连接的客户端，以UUID为键
	mutex    *sync.Mutex        // 保护Clients映射表的并发访问
	Transmit chan []byte        // 消息转发通道，用于接收待转发的消息
	Login    chan *Client       // 登录通道，接收新登录的客户端
	Logout   chan *Client       // 退出登录通道，接收要登出的客户端
}

var ChatServer *Server

// init 初始化全局聊天服务器实例
// 在程序启动时自动创建一个单例的聊天服务器
func init() {
	if ChatServer == nil {
		ChatServer = &Server{
			Clients:  make(map[string]*Client),                   // 初始化客户端映射表
			mutex:    &sync.Mutex{},                              // 初始化互斥锁
			Transmit: make(chan []byte, constants.CHANNEL_SIZE),  // 初始化消息转发通道
			Login:    make(chan *Client, constants.CHANNEL_SIZE), // 初始化登录通道
			Logout:   make(chan *Client, constants.CHANNEL_SIZE), // 初始化登出通道
		}
	}
}

// normalizePath 规范化路径格式
// 将包含完整URL的路径转换为相对路径，例如将 https://127.0.0.1:8000/static/xxx 转为 /static/xxx
func normalizePath(path string) string {
	// 查找 "/static/" 的位置
	staticIndex := strings.Index(path, "/static/")
	if staticIndex < 0 {
		log.Println(path)
		zlog.Error("路径不合法")
	}

	// 返回从 "/static/" 开始的部分
	return path[staticIndex:]
}

// Start 启动聊天服务器主循环
// 服务器的核心运行函数，使用select监听多个通道事件
// 处理客户端登录、登出和消息传输事件
// 1. 监听Login通道：处理新客户端连接
// 2. 监听Logout通道：处理客户端断开连接
// 3. 监听Transmit通道：处理消息传输，支持文本、文件、音视频消息类型
func (s *Server) Start() {
	// 程序结束时关闭所有通道以释放资源
	defer func() {
		close(s.Transmit)
		close(s.Logout)
		close(s.Login)
	}()

	for {
		select {
		case client := <-s.Login:
			{
				s.mutex.Lock()
				s.Clients[client.Uuid] = client // 加锁保护map结构
				s.mutex.Unlock()

				zlog.Debug(fmt.Sprintf("欢迎来到gochat聊天服务器，亲爱的用户%s\n", client.Uuid))
				err := client.Conn.WriteMessage(websocket.TextMessage, []byte("欢迎来到gochat聊天服务器"))
				if err != nil {
					zlog.Error(err.Error())
				}
			}

		case client := <-s.Logout:
			{
				s.mutex.Lock()
				delete(s.Clients, client.Uuid)
				s.mutex.Unlock()

				zlog.Info(fmt.Sprintf("用户%s退出登录\n", client.Uuid))
				if err := client.Conn.WriteMessage(websocket.TextMessage, []byte("已退出登录")); err != nil {
					zlog.Error(err.Error())
				}
			}

		case data := <-s.Transmit:
			{
				var chatMessageReq request.ChatMessageRequest
				if err := json.Unmarshal(data, &chatMessageReq); err != nil {
					zlog.Error(err.Error())
				}
				// log.Println("原消息为：", data, "反序列化后为：", chatMessageReq)

				if chatMessageReq.Type == message_type_enum.Text {
					// 创建文本消息实体并保存到数据库
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
						FileSize:   "0B",                                                    // 文本消息文件大小
						FileType:   "",                                                      // 文本消息无文件类型
						FileName:   "",                                                      // 文本消息无文件名
						Status:     message_status_enum.Unsent,                              // 消息状态：未发送
						CreatedAt:  time.Now(),                                              // 消息创建时间
						AVdata:     "",                                                      // 文本消息无音视频数据
					}

					// 标准化发送者头像路径，去除IP前缀，仅保留 /static/ 后的部分
					message.SendAvatar = normalizePath(message.SendAvatar)
					// 将消息保存到数据库
					if res := dao.GormDB.Create(&message); res.Error != nil {
						zlog.Error(res.Error.Error())
					}

					// 根据接收者ID首字母判断消息类型：'U'为用户私聊，'G'为群聊
					switch message.ReceiveId[0] {
					case 'U':
						// 发送给User
						// 如果能找到ReceiveId，说明在线，可以发送，否则存表后跳过
						// 因为在线的时候是通过websocket更新消息记录的，离线后通过存表，登录时只调用一次数据库操作
						// 切换chat对象后，前端的messageList也会改变，获取messageList从第二次就是从redis中获取
						messageRsp := respond.GetMessageListRespond{
							SendId:     message.SendId,                                  // 发送者ID
							SendName:   message.SendName,                                // 发送者姓名
							SendAvatar: chatMessageReq.SendAvatar,                       // 发送者头像
							ReceiveId:  message.ReceiveId,                               // 接收者ID
							Type:       message.Type,                                    // 消息类型
							Content:    message.Content,                                 // 消息内容
							Url:        message.Url,                                     // 消息URL
							FileSize:   message.FileSize,                                // 文件大小
							FileName:   message.FileName,                                // 文件名
							FileType:   message.FileType,                                // 文件类型
							CreatedAt:  message.CreatedAt.Format("2006-01-02 15:04:05"), // 消息创建时间
						}

						jsonMessage, err := json.Marshal(messageRsp)
						if err != nil {
							zlog.Error(err.Error())
						}
						// log.Println("返回的消息为：", messageRsp, "序列化后为：", jsonMessage)

						var messageBack = &MessageBack{
							Message: jsonMessage,
							Uuid:    message.Uuid,
						}

						s.mutex.Lock()
						if receiveClient, ok := s.Clients[message.ReceiveId]; ok {
							receiveClient.SendBack <- messageBack // 向client.Send发送
						}
						// 因为send_id肯定在线，所以这里在后端进行在线回显message，其实优化的话前端可以直接回显
						// 问题在于前后端的req和rsp结构不同，前端存储message的messageList不能存req，只能存rsp
						// 所以这里后端进行回显，前端不回显
						sendClient := s.Clients[message.SendId]
						sendClient.SendBack <- messageBack
						s.mutex.Unlock()

						// 更新Redis缓存中的用户间消息列表 - 用于快速获取历史消息
						var rspString string
						// 从Redis获取现有的消息列表，key格式为 "message_list_{发送者ID}_{接收者ID}"
						rspString, err = myredis.GetKeyNilIsErr("message_list_" + message.SendId + "_" + message.ReceiveId)
						if err == nil {
							// Redis中存在该消息列表，解析现有消息数组
							var rsp []respond.GetMessageListRespond
							if err := json.Unmarshal([]byte(rspString), &rsp); err != nil {
								// 反序列化错误，记录日志
								zlog.Error(err.Error())
							}
							// 将新消息追加到现有消息列表末尾
							rsp = append(rsp, messageRsp)

							// 将更新后的消息列表重新序列化为JSON字符串
							rspByte, err := json.Marshal(rsp)
							if err != nil {
								// 序列化错误，记录日志
								zlog.Error(err.Error())
							}

							// 将更新后的消息列表写入Redis，设置过期时间
							if err := myredis.SetKeyEx("message_list_"+message.SendId+"_"+message.ReceiveId, string(rspByte), time.Minute*constants.REDIS_TIMEOUT); err != nil {
								// 写入Redis失败，记录错误日志
								zlog.Error(err.Error())
							}
						} else {
							// 获取Redis键值失败，如果不是因为键不存在（redis.Nil）则记录错误
							if !errors.Is(err, redis.Nil) {
								// 实际错误（非键不存在），记录日志
								zlog.Error(err.Error())
							}
							// 如果是因为键不存在（redis.Nil），则表示这是首次聊天，无需处理
						}

					case 'G':
						// 发送给群组
						messageRsp := respond.GetGroupMessageListRespond{
							SendId:     message.SendId,                                  // 发送者ID
							SendName:   message.SendName,                                // 发送者姓名
							SendAvatar: chatMessageReq.SendAvatar,                       // 发送者头像
							ReceiveId:  message.ReceiveId,                               // 接收者ID
							Type:       message.Type,                                    // 消息类型
							Content:    message.Content,                                 // 消息内容
							Url:        message.Url,                                     // 消息URL
							FileSize:   message.FileSize,                                // 文件大小
							FileName:   message.FileName,                                // 文件名
							FileType:   message.FileType,                                // 文件类型
							CreatedAt:  message.CreatedAt.Format("2006-01-02 15:04:05"), // 消息创建时间
						}

						// 1. 将消息响应结构体序列化为JSON格式
						jsonMessage, err := json.Marshal(messageRsp)
						if err != nil {
							zlog.Error(err.Error())
						}
						// log.Println("返回的消息为：", messageRsp, "序列化后为：", jsonMessage)

						// 2. 构建消息回显结构，包含序列化后的消息和消息UUID
						var messageBack = &MessageBack{
							Message: jsonMessage,  // 序列化后的消息内容
							Uuid:    message.Uuid, // 消息唯一标识
						}

						// 3. 查询群组信息，根据消息的接收ID（群组UUID）
						var group model.GroupInfo
						if res := dao.GormDB.Where("uuid = ?", message.ReceiveId).First(&group); res.Error != nil {
							zlog.Error(res.Error.Error())
						}

						// 4. 解析群组成员列表，从群组信息的Members字段（JSON格式）
						var members []string
						if err := json.Unmarshal(group.Members, &members); err != nil {
							zlog.Error(err.Error())
						}

						// 5. 向群组成员广播消息
						s.mutex.Lock() // 加锁保护并发访问
						for _, member := range members {
							if member != message.SendId {
								// 向群组其他成员发送消息
								if receiveClient, ok := s.Clients[member]; ok {
									receiveClient.SendBack <- messageBack
								}
							} else {
								// 发送者也收到消息回显，确保发送者能看到自己发送的消息
								sendClient := s.Clients[message.SendId]
								sendClient.SendBack <- messageBack
							}
						}
						s.mutex.Unlock() // 解锁

						// 6. 更新Redis缓存中的群组消息列表
						var rspString string
						rspString, err = myredis.GetKeyNilIsErr("group_messagelist_" + message.ReceiveId)
						if err == nil {
							// 如果缓存存在，解析并更新消息列表
							var rsp []respond.GetGroupMessageListRespond
							if err := json.Unmarshal([]byte(rspString), &rsp); err != nil {
								zlog.Error(err.Error())
							}
							rsp = append(rsp, messageRsp) // 添加新消息到群组消息列表

							rspByte, err := json.Marshal(rsp)
							if err != nil {
								zlog.Error(err.Error())
							}

							// 将更新后的消息列表写回Redis，并设置过期时间
							if err := myredis.SetKeyEx("group_messagelist_"+message.ReceiveId, string(rspByte), time.Minute*constants.REDIS_TIMEOUT); err != nil {
								zlog.Error(err.Error())
							}
						} else {
							// 如果缓存不存在或其他错误，只记录非键不存在的错误
							if !errors.Is(err, redis.Nil) {
								zlog.Error(err.Error())
							}
						}
					}
				} else if chatMessageReq.Type == message_type_enum.File {
					// 1. 创建文件消息实体并保存到数据库
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
					// 标准化发送者头像路径，去除IP前缀，仅保留 /static/ 后的部分
					message.SendAvatar = normalizePath(message.SendAvatar)

					// 将文件消息保存到数据库
					if res := dao.GormDB.Create(&message); res.Error != nil {
						zlog.Error(res.Error.Error())
					}

					// 根据接收者ID首字母判断消息类型：'U'为用户私聊，'G'为群聊
					switch message.ReceiveId[0] {
					case 'U': // 发送给User（用户私聊）
						// 如果能找到ReceiveId，说明在线，可以发送，否则存表后跳过
						// 因为在线的时候是通过websocket更新消息记录的，离线后通过存表，登录时只调用一次数据库操作
						// 切换chat对象后，前端的messageList也会改变，获取messageList从第二次就是从redis中获取
						messageRsp := respond.GetMessageListRespond{
							SendId:     message.SendId,                                  // 发送者ID
							SendName:   message.SendName,                                // 发送者姓名
							SendAvatar: message.SendAvatar,                              // 发送者头像
							ReceiveId:  message.ReceiveId,                               // 接收者ID
							Type:       message.Type,                                    // 消息类型：文件
							Content:    message.Content,                                 // 文件消息无内容字段
							Url:        message.Url,                                     // 文件URL
							FileSize:   message.FileSize,                                // 文件大小
							FileName:   message.FileName,                                // 文件名
							FileType:   message.FileType,                                // 文件类型
							CreatedAt:  message.CreatedAt.Format("2006-01-02 15:04:05"), // 消息创建时间
						}

						// 序列化消息响应为JSON
						jsonMessage, err := json.Marshal(messageRsp)
						if err != nil {
							zlog.Error(err.Error())
						}
						// log.Println("返回的消息为：", messageRsp, "序列化后为：", jsonMessage)

						// 构建消息回显结构
						var messageBack = &MessageBack{
							Message: jsonMessage,
							Uuid:    message.Uuid,
						}

						// 向接收者和发送者发送消息
						s.mutex.Lock()
						if receiveClient, ok := s.Clients[message.ReceiveId]; ok {
							receiveClient.SendBack <- messageBack // 向接收者发送消息
						}
						// 因为send_id肯定在线，所以这里在后端进行在线回显message，其实优化的话前端可以直接回显
						// 问题在于前后端的req和rsp结构不同，前端存储message的messageList不能存req，只能存rsp
						// 所以这里后端进行回显，前端不回显
						sendClient := s.Clients[message.SendId]
						sendClient.SendBack <- messageBack // 发送者也收到消息回显
						s.mutex.Unlock()

						// 更新Redis缓存中的私聊消息列表
						var rspString string
						rspString, err = myredis.GetKeyNilIsErr("message_list_" + message.SendId + "_" + message.ReceiveId)
						if err == nil {
							var rsp []respond.GetMessageListRespond
							if err := json.Unmarshal([]byte(rspString), &rsp); err != nil {
								zlog.Error(err.Error())
							}
							rsp = append(rsp, messageRsp) // 添加新消息到私聊消息列表

							rspByte, err := json.Marshal(rsp)
							if err != nil {
								zlog.Error(err.Error())
							}

							if err := myredis.SetKeyEx("message_list_"+message.SendId+"_"+message.ReceiveId, string(rspByte), time.Minute*constants.REDIS_TIMEOUT); err != nil {
								zlog.Error(err.Error())
							}
						} else {
							if !errors.Is(err, redis.Nil) { // 如果不是因为键不存在导致的错误
								zlog.Error(err.Error())
							}
						}
					case 'G': // 发送给群组的文件消息
						// 构建群组消息响应
						messageRsp := respond.GetGroupMessageListRespond{
							SendId:     message.SendId,                                  // 发送者ID
							SendName:   message.SendName,                                // 发送者姓名
							SendAvatar: chatMessageReq.SendAvatar,                       // 发送者头像
							ReceiveId:  message.ReceiveId,                               // 接收者ID
							Type:       message.Type,                                    // 消息类型：文件
							Content:    message.Content,                                 // 文件消息无内容字段
							Url:        message.Url,                                     // 文件URL
							FileSize:   message.FileSize,                                // 文件大小
							FileName:   message.FileName,                                // 文件名
							FileType:   message.FileType,                                // 文件类型
							CreatedAt:  message.CreatedAt.Format("2006-01-02 15:04:05"), // 消息创建时间
						}

						// 序列化消息响应为JSON
						jsonMessage, err := json.Marshal(messageRsp)
						if err != nil {
							zlog.Error(err.Error())
						}
						// log.Println("返回的消息为：", messageRsp, "序列化后为：", jsonMessage)

						// 构建消息回显结构
						var messageBack = &MessageBack{
							Message: jsonMessage,
							Uuid:    message.Uuid,
						}

						// 查询群组信息，获取群组成员列表
						var group model.GroupInfo
						if res := dao.GormDB.Where("uuid = ?", message.ReceiveId).First(&group); res.Error != nil {
							zlog.Error(res.Error.Error())
						}

						// 解析群组成员列表
						var members []string
						if err := json.Unmarshal(group.Members, &members); err != nil {
							zlog.Error(err.Error())
						}

						// 向群组所有成员发送消息
						s.mutex.Lock()
						for _, member := range members {
							if member != message.SendId {
								if receiveClient, ok := s.Clients[member]; ok {
									receiveClient.SendBack <- messageBack // 向群组其他成员发送消息
								}
							} else {
								sendClient := s.Clients[message.SendId]
								sendClient.SendBack <- messageBack // 发送者也收到消息回显
							}
						}
						s.mutex.Unlock()

						// 更新Redis缓存中的群组消息列表
						var rspString string
						rspString, err = myredis.GetKeyNilIsErr("group_messagelist_" + message.ReceiveId)
						if err == nil {
							var rsp []respond.GetGroupMessageListRespond
							if err := json.Unmarshal([]byte(rspString), &rsp); err != nil {
								zlog.Error(err.Error())
							}
							rsp = append(rsp, messageRsp) // 添加新消息到群组消息列表

							rspByte, err := json.Marshal(rsp)
							if err != nil {
								zlog.Error(err.Error())
							}

							if err := myredis.SetKeyEx("group_messagelist_"+message.ReceiveId, string(rspByte), time.Minute*constants.REDIS_TIMEOUT); err != nil {
								zlog.Error(err.Error())
							}
						} else {
							if !errors.Is(err, redis.Nil) { // 如果不是因为键不存在导致的错误
								zlog.Error(err.Error())
							}
						}
					}
				} else if chatMessageReq.Type == message_type_enum.AudioOrVideo { // 处理音视频消息
					var avData request.AVData
					if err := json.Unmarshal([]byte(chatMessageReq.AVdata), &avData); err != nil {
						zlog.Error(err.Error())
					}
					//log.Println(avData)
					message := model.Message{
						Uuid:       fmt.Sprintf("M%s", random.GetNowAndLenRandomString(11)), // 生成唯一消息ID
						SessionId:  chatMessageReq.SessionId,                                // 会话ID
						Type:       chatMessageReq.Type,                                     // 消息类型为音视频
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

					// 只有当音视频消息是通话相关操作时才保存到数据库
					if avData.MessageId == "PROXY" && (avData.Type == "start_call" || avData.Type == "receive_call" || avData.Type == "reject_call") {
						// 规范化发送者头像路径，防止IP前缀被引入
						message.SendAvatar = normalizePath(message.SendAvatar)
						if res := dao.GormDB.Create(&message); res.Error != nil {
							zlog.Error(res.Error.Error())
						}
					}

					if chatMessageReq.ReceiveId[0] == 'U' { // 发送给User
						// 如果能找到ReceiveId，说明在线，可以发送，否则存表后跳过
						// 因为在线的时候是通过websocket更新消息记录的，离线后通过存表，登录时只调用一次数据库操作
						// 切换chat对象后，前端的messageList也会改变，获取messageList从第二次就是从redis中获取
						messageRsp := respond.AVMessageRespond{
							SendId:     message.SendId,                                  // 发送者ID
							SendName:   message.SendName,                                // 发送者姓名
							SendAvatar: message.SendAvatar,                              // 发送者头像
							ReceiveId:  message.ReceiveId,                               // 接收者ID
							Type:       message.Type,                                    // 消息类型
							Content:    message.Content,                                 // 音视频消息内容为空
							Url:        message.Url,                                     // 音视频消息URL为空
							FileSize:   message.FileSize,                                // 音视频消息文件大小为空
							FileName:   message.FileName,                                // 音视频消息文件名为空
							FileType:   message.FileType,                                // 音视频消息文件类型为空
							CreatedAt:  message.CreatedAt.Format("2006-01-02 15:04:05"), // 消息创建时间
							AVdata:     message.AVdata,                                  // 音视频数据
						}

						jsonMessage, err := json.Marshal(messageRsp)
						if err != nil {
							zlog.Error(err.Error())
						}
						// log.Println("返回的消息为：", messageRsp, "序列化后为：", jsonMessage)
						// log.Println("返回的消息为：", messageRsp)

						var messageBack = &MessageBack{
							Message: jsonMessage,
							Uuid:    message.Uuid,
						}

						s.mutex.Lock()
						if receiveClient, ok := s.Clients[message.ReceiveId]; ok {
							//messageBack.Message = jsonMessage
							//messageBack.Uuid = message.Uuid
							receiveClient.SendBack <- messageBack // 向client.Send发送
						}
						// 通话消息不能回显给发送者，否则会出现重复的通话请求
						// 例如发送开始通话请求后，如果回显给发送者，会导致两个start_call
						//sendClient := s.Clients[message.SendId]
						//sendClient.SendBack <- messageBack
						s.mutex.Unlock()
					}
				}

			}
		}
	}
}

// Close 关闭服务器，清理资源
// 关闭所有通道以释放资源
func (s *Server) Close() {
	close(s.Login)
	close(s.Logout)
	close(s.Transmit)
}

// SendClientToLogin 将客户端添加到登录队列
// 通过登录通道通知服务器有新的客户端连接
func (s *Server) SendClientToLogin(client *Client) {
	s.mutex.Lock()
	s.Login <- client // 将客户端发送到登录通道
	s.mutex.Unlock()
}

// SendClientToLogout 将客户端添加到登出队列
// 通过登出通道通知服务器有客户端断开连接
func (s *Server) SendClientToLogout(client *Client) {
	s.mutex.Lock()
	s.Logout <- client // 将客户端发送到登出通道
	s.mutex.Unlock()
}

// SendMessageToTransmit 将消息添加到传输队列
// 通过传输通道将消息发送给所有相关客户端
func (s *Server) SendMessageToTransmit(message []byte) {
	s.mutex.Lock()
	s.Transmit <- message // 将消息发送到传输通道
	s.mutex.Unlock()
}

// RemoveClient 从客户端列表中移除指定UUID的客户端
// 用于手动清理客户端连接记录
func (s *Server) RemoveClient(uuid string) {
	s.mutex.Lock()
	delete(s.Clients, uuid) // 从客户端映射表中删除指定UUID的客户端
	s.mutex.Unlock()
}
