// package dao 数据访问层包，负责数据库连接和操作
package dao

import (
	"fmt"
	"gochat/internal/config" // 配置包，用于获取数据库连接配置
	"gochat/internal/model"  // 模型包，定义数据库表结构
	"gochat/pkg/zlog"        // 日志包，用于记录错误信息

	"gorm.io/driver/mysql" // MySQL 驱动
	"gorm.io/gorm"         // GORM 框架
)

// GormDB 全局数据库实例，供其他模块使用
var GormDB *gorm.DB

// init 函数在包被导入时自动执行，用于初始化数据库连接
func init() {
	// 获取配置信息
	conf := config.GetConfig()
	user := conf.MysqlConfig.User // 数据库用户名
	// password := conf.MysqlConfig.Password  // 数据库密码（当前使用 socket 连接，无需密码）
	// host := conf.MysqlConfig.Host          // 数据库主机（当前使用 socket 连接，无需主机）
	// port := conf.MysqlConfig.Port          // 数据库端口（当前使用 socket 连接，无需端口）
	appName := conf.MainConfig.AppName // 数据库名称

	// 构建 DSN (Data Source Name)，使用 Unix socket 连接方式
	// 格式：用户名@unix(socket路径)/数据库名?参数
	dsn := fmt.Sprintf("%s@unix(/var/run/mysqld/mysqld.sock)/%s?charset=utf8mb4&parseTime=True&loc=Local", user, appName)

	// 注释掉的 TCP 连接方式
	// dsn := fmt.Sprintf("%s:%s@tcp(%s:%d)/%s?charset=utf8mb4&parseTime=True&loc=Local", user, password, host, port, appName)

	var err error
	// 连接数据库，使用 MySQL 驱动和默认配置
	GormDB, err = gorm.Open(mysql.Open(dsn), &gorm.Config{})
	if err != nil {
		// 连接失败，记录致命错误并退出程序
		zlog.Fatal(err.Error())
	}

	// 自动迁移数据库表结构
	// 当数据库中不存在对应表时，会自动创建
	// 当表结构发生变化时，会自动更新（注意：可能会丢失数据）
	err = GormDB.AutoMigrate(
		&model.UserInfo{},     // 用户信息表
		&model.GroupInfo{},    // 群组信息表
		&model.UserContact{},  // 用户联系人表
		&model.Session{},      // 会话表
		&model.ContactApply{}, // 联系人申请表
		&model.Message{},      // 消息表
	)
	if err != nil {
		// 迁移失败，记录致命错误并退出程序
		zlog.Fatal(err.Error())
	}
}
