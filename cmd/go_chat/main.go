// Package main 包含GoChat服务器的主入口函数
package main

import (
	"fmt"
	"gochat/internal/config"                // 配置管理
	"gochat/internal/https_server"          // HTTPS服务器
	"gochat/internal/service/chat"          // 聊天服务
	"gochat/internal/service/kafka"         // Kafka服务
	myredis "gochat/internal/service/redis" // Redis服务
	"gochat/pkg/zlog"                       // 日志工具
	"os"
	"os/signal"
	"syscall"
)

// main 函数是GoChat服务器的入口点
// 功能：
//  1. 加载配置
//  2. 初始化消息服务（Kafka或Channel模式）
//  3. 启动HTTPS服务器
//  4. 设置信号监听，处理优雅关闭
//  5. 清理资源（关闭Kafka、清理Redis）
func main() {
	// 加载配置
	conf := config.GetConfig()
	host := conf.MainConfig.Host
	port := conf.MainConfig.Port
	kafkaConfig := conf.KafkaConfig

	// 初始化Kafka服务（如果使用Kafka消息模式）
	if kafkaConfig.MessageMode == "kafka" {
		kafka.KafkaService.KafkaInit()
	}

	// 根据消息模式选择启动相应的聊天服务
	if kafkaConfig.MessageMode == "channel" {
		// 使用Channel模式启动聊天服务器
		go chat.ChatServer.Start()
	} else {
		// 使用Kafka模式启动聊天服务器
		go chat.KafkaChatServer.Start()
	}

	// 启动HTTPS服务器（异步）
	go func() {
		// Ubuntu22.04云服务器部署
		if err := https_server.GE.RunTLS(fmt.Sprintf("%s:%d", host, port), "/etc/ssl/certs/server.crt", "/etc/ssl/private/server.key"); err != nil {
			zlog.Fatal("server running fault")
			return
		}
	}()

	// 设置信号监听，用于优雅关闭服务器
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

	// 等待中断信号
	<-quit

	// 关闭Kafka服务（如果使用Kafka消息模式）
	if kafkaConfig.MessageMode == "kafka" {
		kafka.KafkaService.KafkaClose()
	}

	// 关闭聊天服务器
	chat.ChatServer.Close()
	zlog.Info("关闭服务器...")

	// 删除所有Redis键，清理缓存
	if err := myredis.DeleteAllRedisKeys(); err != nil {
		zlog.Error(err.Error())
	} else {
		zlog.Info("所有Redis键已删除")
	}

	zlog.Info("服务器已关闭")
}
