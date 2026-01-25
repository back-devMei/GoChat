// kafka 包提供了与 Apache Kafka 消息队列系统的集成服务
// 主要负责聊天消息的发送和接收，同时也预留了登录登出消息的处理接口
package kafka

import (
	"gochat/internal/config"
	"gochat/pkg/zlog"
	"time"

	"github.com/segmentio/kafka-go"
)

// kafkaService 结构体定义了 Kafka 服务的相关组件
// 包含用于聊天消息发送的写入器、用于接收消息的读取器和用于创建主题的连接
type kafkaService struct {
	ChatWriter *kafka.Writer // 聊天消息写入器，用于发送聊天消息到 Kafka
	ChatReader *kafka.Reader // 聊天消息读取器，用于从 Kafka 接收聊天消息
}

// KafkaService 全局唯一的 Kafka 服务实例
var KafkaService = new(kafkaService)

// KafkaInit 初始化 Kafka 服务
// 根据配置创建聊天消息的读写器，建立与 Kafka 服务器的连接
func (k *kafkaService) KafkaInit() {
	kafkaConfig := config.GetConfig().KafkaConfig
	// 使用结构体字面量创建 Writer 实例，直接初始化配置
	k.ChatWriter = &kafka.Writer{
		Addr:                   kafka.TCP(kafkaConfig.HostPort),
		Topic:                  kafkaConfig.ChatTopic,
		Balancer:               &kafka.Hash{},
		WriteTimeout:           kafkaConfig.Timeout * time.Second,
		RequiredAcks:           kafka.RequireNone,
		AllowAutoTopicCreation: false,
	}

	// 使用 NewReader 函数创建 Reader 实例，这种方式更适合复杂的读取配置
	k.ChatReader = kafka.NewReader(kafka.ReaderConfig{
		Brokers:        []string{kafkaConfig.HostPort},
		Topic:          kafkaConfig.ChatTopic,
		CommitInterval: kafkaConfig.Timeout * time.Second,
		GroupID:        "chat",
		StartOffset:    kafka.LastOffset,
	})
}

// KafkaClose 关闭 Kafka 服务
// 释放聊天消息读写器占用的资源，关闭与 Kafka 的连接
func (k *kafkaService) KafkaClose() {
	if k.ChatWriter != nil {
		if err := k.ChatWriter.Close(); err != nil {
			zlog.Error(err.Error())
		}
	}
	if k.ChatReader != nil {
		if err := k.ChatReader.Close(); err != nil {
			zlog.Error(err.Error())
		}
	}
}

// CreateTopic 创建 Kafka 主题
// 根据配置创建消息主题，支持聊天、登录和登出主题
func (k *kafkaService) CreateTopic() {
	// 如果已经有topic了，就不创建了
	kafkaConfig := config.GetConfig().KafkaConfig

	// 连接至任意kafka节点
	conn, err := kafka.Dial("tcp", kafkaConfig.HostPort)
	if err != nil {
		zlog.Error("Failed to connect to Kafka: " + err.Error())
		return
	}
	defer conn.Close() // 确保连接被关闭

	// 定义主题配置数组
	// 创建聊天、登录和登出主题
	topicConfigs := []kafka.TopicConfig{
		{
			Topic:             kafkaConfig.ChatTopic,
			NumPartitions:     kafkaConfig.Partition,
			ReplicationFactor: 1,
		},
		// {
		// 	Topic:             kafkaConfig.LoginTopic,
		// 	NumPartitions:     kafkaConfig.Partition,
		// 	ReplicationFactor: 1,
		// },
		// {
		// 	Topic:             kafkaConfig.LogoutTopic,
		// 	NumPartitions:     kafkaConfig.Partition,
		// 	ReplicationFactor: 1,
		// },
	}

	// 创建主题
	if err = conn.CreateTopics(topicConfigs...); err != nil {
		zlog.Error("Failed to create topics: " + err.Error())
	}
}
