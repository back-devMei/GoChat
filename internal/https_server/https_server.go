// Package https_server 实现HTTPS服务器的初始化和路由配置
package https_server

import (
	v1 "gochat/api/v1"       // 导入API v1版本的控制器
	"gochat/internal/config" // 导入配置管理包
	"gochat/pkg/ssl"         // 导入SSL/TLS处理包

	"github.com/gin-contrib/cors" // Gin框架的CORS中间件
	"github.com/gin-gonic/gin"    // Gin Web框架
)

// GE 全局Gin引擎实例，用于处理HTTP请求
var GE *gin.Engine

// init 初始化函数，在包被导入时自动执行
// 配置Gin引擎、CORS中间件、SSL处理、静态文件服务以及API路由
func init() {
	// 初始化Gin引擎，默认启用Logger和Recovery中间件
	GE = gin.Default()

	// 配置CORS（跨源资源共享）中间件
	corsConfig := cors.DefaultConfig()
	corsConfig.AllowOrigins = []string{"*"}                                                         // 允许所有来源访问
	corsConfig.AllowMethods = []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"}                   // 允许的HTTP方法
	corsConfig.AllowHeaders = []string{"Origin", "Content-Length", "Content-Type", "Authorization"} // 允许的请求头

	// 应用CORS中间件
	GE.Use(cors.New(corsConfig))

	// 应用SSL重定向中间件，强制HTTP请求重定向到HTTPS
	// 注意：这里使用主机和端口配置进行SSL重定向
	GE.Use(ssl.TlsHandler(config.GetConfig().MainConfig.Host, config.GetConfig().MainConfig.Port))

	// 配置静态文件服务
	// 提供头像文件的静态访问服务
	GE.Static("/static/avatars", config.GetConfig().StaticSrcConfig.StaticAvatarPath)
	// 提供其他文件的静态访问服务
	GE.Static("/static/files", config.GetConfig().StaticSrcConfig.StaticFilePath)

	// 用户认证相关API路由
	GE.POST("/login", v1.Login)       // 用户登录
	GE.POST("/register", v1.Register) // 用户注册

	// 用户信息管理相关API路由
	GE.POST("/user/updateUserInfo", v1.UpdateUserInfo)   // 更新用户信息
	GE.POST("/user/getUserInfoList", v1.GetUserInfoList) // 获取用户信息列表
	GE.POST("/user/ableUsers", v1.AbleUsers)             // 启用用户
	GE.POST("/user/getUserInfo", v1.GetUserInfo)         // 获取用户信息
	GE.POST("/user/disableUsers", v1.DisableUsers)       // 禁用用户
	GE.POST("/user/deleteUsers", v1.DeleteUsers)         // 删除用户
	GE.POST("/user/setAdmin", v1.SetAdmin)               // 设置管理员
	GE.POST("/user/sendSmsCode", v1.SendSmsCode)         // 发送短信验证码
	GE.POST("/user/smsLogin", v1.SmsLogin)               // 短信登录
	GE.POST("/user/wsLogout", v1.WsLogout)               // WebSocket登出

	// 群组管理相关API路由
	GE.POST("/group/createGroup", v1.CreateGroup)               // 创建群组
	GE.POST("/group/loadMyGroup", v1.LoadMyGroup)               // 加载我的群组
	GE.POST("/group/checkGroupAddMode", v1.CheckGroupAddMode)   // 检查群组加入方式
	GE.POST("/group/enterGroupDirectly", v1.EnterGroupDirectly) // 直接加入群组
	GE.POST("/group/leaveGroup", v1.LeaveGroup)                 // 退出群组
	GE.POST("/group/dismissGroup", v1.DismissGroup)             // 解散群组
	GE.POST("/group/getGroupInfo", v1.GetGroupInfo)             // 获取群组信息
	GE.POST("/group/getGroupInfoList", v1.GetGroupInfoList)     // 获取群组信息列表
	GE.POST("/group/deleteGroups", v1.DeleteGroups)             // 删除群组
	GE.POST("/group/setGroupsStatus", v1.SetGroupsStatus)       // 设置群组状态
	GE.POST("/group/updateGroupInfo", v1.UpdateGroupInfo)       // 更新群组信息
	GE.POST("/group/getGroupMemberList", v1.GetGroupMemberList) // 获取群组成员列表
	GE.POST("/group/removeGroupMembers", v1.RemoveGroupMembers) // 移除群组成员

	// 会话管理相关API路由
	GE.POST("/session/openSession", v1.OpenSession)                         // 开启会话
	GE.POST("/session/getUserSessionList", v1.GetUserSessionList)           // 获取用户会话列表
	GE.POST("/session/getGroupSessionList", v1.GetGroupSessionList)         // 获取群组会话列表
	GE.POST("/session/deleteSession", v1.DeleteSession)                     // 删除会话
	GE.POST("/session/checkOpenSessionAllowed", v1.CheckOpenSessionAllowed) // 检查开启会话权限

	// 联系人管理相关API路由
	GE.POST("/contact/getUserList", v1.GetUserList)               // 获取用户列表
	GE.POST("/contact/loadMyJoinedGroup", v1.LoadMyJoinedGroup)   // 加载我加入的群组
	GE.POST("/contact/getContactInfo", v1.GetContactInfo)         // 获取联系人信息
	GE.POST("/contact/deleteContact", v1.DeleteContact)           // 删除联系人
	GE.POST("/contact/applyContact", v1.ApplyContact)             // 申请联系人
	GE.POST("/contact/getNewContactList", v1.GetNewContactList)   // 获取新联系人列表
	GE.POST("/contact/passContactApply", v1.PassContactApply)     // 通过联系人申请
	GE.POST("/contact/blackContact", v1.BlackContact)             // 拉黑联系人
	GE.POST("/contact/cancelBlackContact", v1.CancelBlackContact) // 取消拉黑联系人
	GE.POST("/contact/getAddGroupList", v1.GetAddGroupList)       // 获取加入群组列表
	GE.POST("/contact/refuseContactApply", v1.RefuseContactApply) // 拒绝联系人申请
	GE.POST("/contact/blackApply", v1.BlackApply)                 // 拉黑申请

	// 消息管理相关API路由
	GE.POST("/message/getMessageList", v1.GetMessageList)           // 获取消息列表
	GE.POST("/message/getGroupMessageList", v1.GetGroupMessageList) // 获取群组消息列表
	GE.POST("/message/uploadAvatar", v1.UploadAvatar)               // 上传头像
	GE.POST("/message/uploadFile", v1.UploadFile)                   // 上传文件

	// 聊天室相关API路由
	GE.POST("/chatroom/getCurContactListInChatRoom", v1.GetCurContactListInChatRoom) // 获取聊天室中的联系人列表

	// WebSocket相关API路由
	GE.GET("/wss", v1.WsLogin) // WebSocket连接入口
}
