package chat

import (
	"context"
	"encoding/json"
	"gochat/internal/config"
	"gochat/internal/dao"
	"gochat/internal/dto/request"
	"gochat/internal/model"
	myKafka "gochat/internal/service/kafka"
	"gochat/pkg/constants"
	"gochat/pkg/enum/message/message_status_enum"
	"gochat/pkg/zlog"
	"log"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	"github.com/segmentio/kafka-go"
)

// MessageBack 定义服务器向客户端回传的消息结构
type MessageBack struct {
	Message []byte // 序列化后的消息内容
	Uuid    string // 消息唯一标识
}

// Client 定义WebSocket客户端连接结构
type Client struct {
	Conn     *websocket.Conn   // WebSocket连接对象
	Uuid     string            // 客户端唯一标识
	SendTo   chan []byte       // 发送消息到服务器的通道
	SendBack chan *MessageBack // 服务器回传消息到客户端的通道
}

// upgrader 用于将HTTP连接升级为WebSocket连接
var upgrader = websocket.Upgrader{
	ReadBufferSize:  2048, // 读取缓冲区大小
	WriteBufferSize: 2048, // 写入缓冲区大小
	// 检查连接的Origin头，此处返回true表示允许所有跨域请求
	CheckOrigin: func(r *http.Request) bool {
		return true
	},
}

// ctx 上下文对象，用于Kafka操作
var ctx = context.Background()

// messageMode 消息传输模式，从配置中获取，支持"channel"和"kafka"
var messageMode = config.GetConfig().KafkaConfig.MessageMode

// Read 从WebSocket连接读取消息并发送到服务器
// 每个客户端连接会启动一个goroutine执行此方法
func (c *Client) Read() {
	zlog.Info("ws read goroutine start")
	for {
		// 阻塞读取WebSocket消息
		_, jsonMessage, err := c.Conn.ReadMessage() // 阻塞状态
		if err != nil {
			zlog.Error(err.Error())
			return // 发生错误，断开WebSocket连接
		}

		// 解析消息为ChatMessageRequest结构
		var message = request.ChatMessageRequest{}
		if err := json.Unmarshal(jsonMessage, &message); err != nil {
			zlog.Error(err.Error())
		}
		log.Println("接受到消息为: ", jsonMessage)

		// 根据配置的消息模式处理消息
		if messageMode == "channel" {
			// 通道模式：使用Go的channel进行消息传递
			// 如果服务器的转发通道没满，先处理客户端缓存的消息
			for len(ChatServer.Transmit) < constants.CHANNEL_SIZE && len(c.SendTo) > 0 {
				sendToMessage := <-c.SendTo
				ChatServer.SendMessageToTransmit(sendToMessage)
			}

			// 如果服务器通道没满且客户端缓存为空，直接发送到服务器通道
			if len(ChatServer.Transmit) < constants.CHANNEL_SIZE {
				ChatServer.SendMessageToTransmit(jsonMessage)
			} else if len(c.SendTo) < constants.CHANNEL_SIZE {
				// 如果服务器通道满了，将消息缓存到客户端通道
				c.SendTo <- jsonMessage
			} else {
				// 通道都满了，返回错误提示
				if err := c.Conn.WriteMessage(websocket.TextMessage, []byte("由于目前同一时间过多用户发送消息，消息发送失败，请稍后重试")); err != nil {
					zlog.Error(err.Error())
				}
			}
		} else {
			// Kafka模式：使用Kafka进行消息传递
			if err := myKafka.KafkaService.ChatWriter.WriteMessages(ctx, kafka.Message{
				Key:   []byte(strconv.Itoa(config.GetConfig().KafkaConfig.Partition)),
				Value: jsonMessage,
			}); err != nil {
				zlog.Error(err.Error())
			}
			zlog.Info("已发送消息：" + string(jsonMessage))
		}
	}
}

// Write 从服务器读取消息并发送到WebSocket连接
// 每个客户端连接会启动一个goroutine执行此方法
func (c *Client) Write() {
	zlog.Info("ws write goroutine start")
	// 阻塞从SendBack通道读取消息
	for messageBack := range c.SendBack { // 阻塞状态
		// 通过WebSocket发送消息给客户端
		err := c.Conn.WriteMessage(websocket.TextMessage, messageBack.Message)
		if err != nil {
			zlog.Error(err.Error())
			return // 发生错误，断开WebSocket连接
		}
		// log.Println("已发送消息：", messageBack.Message)

		// 消息发送成功，更新消息状态为已发送
		if res := dao.GormDB.Model(&model.Message{}).Where("uuid = ?", messageBack.Uuid).Update("status", message_status_enum.Sent); res.Error != nil {
			zlog.Error(res.Error.Error())
		}
	}
}

// NewClientInit 初始化新的客户端连接
// 当接收到前端的登录消息时，会调用该函数
func NewClientInit(c *gin.Context, clientId string) {
	kafkaConfig := config.GetConfig().KafkaConfig

	// 将HTTP连接升级为WebSocket连接
	conn, err := upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		zlog.Error(err.Error())
	}

	// 创建新的Client对象
	client := &Client{
		Conn:     conn,                                            // WebSocket连接
		Uuid:     clientId,                                        // 客户端唯一标识
		SendTo:   make(chan []byte, constants.CHANNEL_SIZE),       // 发送消息到服务器的通道
		SendBack: make(chan *MessageBack, constants.CHANNEL_SIZE), // 服务器回传消息的通道
	}

	// 根据消息模式将客户端添加到对应的服务器
	if kafkaConfig.MessageMode == "channel" {
		ChatServer.SendClientToLogin(client)
	} else {
		KafkaChatServer.SendClientToLogin(client)
	}

	// 启动客户端的读写goroutine
	go client.Read()  // 读取客户端消息
	go client.Write() // 向客户端写入消息

	zlog.Info("ws连接成功")
}

// ClientLogout 处理客户端登出
// 当接收到前端的登出消息时，会调用该函数
func ClientLogout(clientId string) (string, int) {
	kafkaConfig := config.GetConfig().KafkaConfig

	// 获取客户端对象
	client := ChatServer.Clients[clientId]
	if client != nil {
		// 根据消息模式从对应的服务器中移除客户端
		if kafkaConfig.MessageMode == "channel" {
			ChatServer.SendClientToLogout(client)
		} else {
			KafkaChatServer.SendClientToLogout(client)
		}

		// 关闭WebSocket连接
		if err := client.Conn.Close(); err != nil {
			zlog.Error(err.Error())
			return constants.SYSTEM_ERROR, -1
		}

		// 关闭客户端的通道
		close(client.SendTo)
		close(client.SendBack)
	}

	return "退出成功", 0
}
