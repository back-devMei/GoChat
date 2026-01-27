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
	"gochat/internal/service/sms"
	"gochat/pkg/constants"
	"gochat/pkg/enum/user_info/user_status_enum"
	"gochat/pkg/util/random"
	"gochat/pkg/zlog"
	"regexp"
	"time"

	redis "github.com/go-redis/redis/v8"
	"gorm.io/gorm"
)

type userInfoService struct {
}

var UserInfoService = new(userInfoService)

// dao层加不了校验，在service层加
// checkTelephoneValid 检验电话是否有效
func (u *userInfoService) checkTelephoneValid(telephone string) bool {
	// 中国手机号正则：
	// 以1开头，第二位为3/8/4/5/6/7/9，第三位根据第二位有特定规则，后8位任意数字
	// ^字符串开头，$字符串结束
	// ' '：不处理任何转义字符，内容完全按字面意思保存
	pattern := `^1([38][0-9]|4[579]|5[^4]|6[6]|7[1-35-8]|9[189])\d{8}$`
	match, err := regexp.MatchString(pattern, telephone)
	if err != nil {
		zlog.Error(err.Error())
	}
	return match
}

// checkEmailValid 校验邮箱是否有效
func (u *userInfoService) checkEmailValid(email string) bool {
	// 邮箱正则：用户名@域名.顶级域名，不含空格
	pattern := `^[^\s@]+@[^\s@]+\.[^\s@]+$`
	match, err := regexp.MatchString(pattern, email)
	if err != nil {
		zlog.Error(err.Error())
	}
	return match
}

// checkUserIsAdminOrNot 检验用户是否为管理员
func (u *userInfoService) checkUserIsAdminOrNot(user model.UserInfo) int8 {
	return user.IsAdmin // 0表示不是管理员，1表示是管理员
}

// Login 用户密码登录功能
// 根据用户提交的登录请求信息验证用户身份，执行登录验证逻辑
// 参数: loginReq - 包含用户登录信息的请求对象(手机号和密码)
// 返回值:
//   - string: 操作结果消息(成功或失败的具体描述)
//   - *respond.LoginRespond: 登录成功的用户信息响应对象，登录失败时为nil
//   - int: 操作状态码(0表示成功，负数表示不同类型的错误)
func (u *userInfoService) Login(loginReq request.LoginRequest) (string, *respond.LoginRespond, int) {
	// 提取用户提交的密码
	password := loginReq.Password

	// 定义用户信息结构体实例用于查询结果存储
	var user model.UserInfo

	// 在数据库中根据手机号查询用户记录
	// 使用 First 方法查找第一个匹配的用户记录
	res := dao.GormDB.First(&user, "telephone = ?", loginReq.Telephone)

	// 检查数据库查询是否出现错误
	if res.Error != nil {
		// 判断是否为"记录未找到"错误
		if errors.Is(res.Error, gorm.ErrRecordNotFound) {
			// 用户不存在的情况下，返回注册提示信息
			message := "用户不存在，请注册"
			zlog.Error(message)
			return message, nil, -2 // -2 表示用户不存在
		}
		// 其他数据库错误，返回系统错误
		zlog.Error(res.Error.Error())
		return constants.SYSTEM_ERROR, nil, -1 // -1 表示系统错误
	}

	// 验证用户密码是否正确
	if user.Password != password {
		// 密码错误，返回错误提示
		message := "密码不正确，请重试"
		zlog.Error(message)
		return message, nil, -2 // -2 表示密码错误
	}

	// 登录验证成功，构建响应对象
	loginRsp := &respond.LoginRespond{
		Uuid:      user.Uuid,      // 用户UUID
		Telephone: user.Telephone, // 用户手机号
		Nickname:  user.Nickname,  // 用户昵称
		Email:     user.Email,     // 用户邮箱
		Avatar:    user.Avatar,    // 用户头像
		Gender:    user.Gender,    // 用户性别
		Birthday:  user.Birthday,  // 用户生日
		Signature: user.Signature, // 用户个性签名
		IsAdmin:   user.IsAdmin,   // 是否为管理员
		Status:    user.Status,    // 用户状态
	}

	// 格式化用户创建日期，提取年月日信息
	year, month, day := user.CreatedAt.Date()
	loginRsp.CreatedAt = fmt.Sprintf("%d.%d.%d", year, month, day)

	// 返回登录成功的信息
	return "登陆成功", loginRsp, 0 // 0 表示登录成功
}

// SmsLogin 验证码登录
// 通过手机号和接收到的短信验证码进行用户登录验证
// 参数: req - 包含手机号和验证码的请求对象
// 返回值:
//   - string: 操作结果消息，如登录成功或失败的具体描述
//   - *respond.LoginRespond: 登录成功时返回用户信息，失败时为nil
//   - int: 状态码，0表示成功，负数表示不同类型的错误
func (u *userInfoService) SmsLogin(req request.SmsLoginRequest) (string, *respond.LoginRespond, int) {
	// 查询数据库，根据手机号获取用户信息
	var user model.UserInfo
	res := dao.GormDB.First(&user, "telephone = ?", req.Telephone)
	// 检查数据库查询是否出错
	if res.Error != nil {
		// 检查是否为"记录未找到"错误
		if errors.Is(res.Error, gorm.ErrRecordNotFound) {
			message := "用户不存在，请注册"
			zlog.Error(message)
			return message, nil, -2 // -2 表示用户不存在
		}
		// 其他数据库错误
		zlog.Error(res.Error.Error())
		return constants.SYSTEM_ERROR, nil, -1 // -1 表示系统错误
	}

	// 构造Redis中存储验证码的键名
	key := "auth_code_" + req.Telephone
	// 从Redis中获取该手机号对应的验证码
	code, err := myredis.GetKey(key)
	if err != nil {
		zlog.Error(err.Error())
		return constants.SYSTEM_ERROR, nil, -1
	}
	// 验证用户输入的验证码是否正确
	if code != req.SmsCode {
		message := "验证码不正确，请重试"
		zlog.Info(message)
		return message, nil, -2 // -2 表示验证码错误
	} else {
		// 验证码正确，删除Redis中的验证码(一次性使用)
		if err := myredis.DelKeyIfExists(key); err != nil {
			zlog.Error(err.Error())
			return constants.SYSTEM_ERROR, nil, -1
		}
	}

	// 构造登录响应对象，包含用户的基本信息
	loginRsp := &respond.LoginRespond{
		Uuid:      user.Uuid,      // 用户唯一标识符
		Telephone: user.Telephone, // 手机号
		Nickname:  user.Nickname,  // 昵称
		Email:     user.Email,     // 邮箱
		Avatar:    user.Avatar,    // 头像
		Gender:    user.Gender,    // 性别
		Birthday:  user.Birthday,  // 生日
		Signature: user.Signature, // 个人签名
		IsAdmin:   user.IsAdmin,   // 管理员权限
		Status:    user.Status,    // 用户状态
	}
	// 格式化用户创建日期，提取年月日信息
	year, month, day := user.CreatedAt.Date()
	loginRsp.CreatedAt = fmt.Sprintf("%d.%d.%d", year, month, day)

	// 返回登录成功的信息和用户数据
	return "登陆成功", loginRsp, 0
}

// SendSmsCode 发送短信验证码 - 验证码登录
func (u *userInfoService) SendSmsCode(telephone string) (string, int) {
	return sms.VerificationCode(telephone)
}

// checkTelephoneExist 检查手机号是否存在
func (u *userInfoService) checkTelephoneExist(telephone string) (string, int) {
	var user model.UserInfo
	// gorm默认排除软删除，所以翻译过来的select语句是
	/*
		SELECT
			*
		FROM
			user_info
		WHERE
			telephone = 18089596095
			AND
			user_info.deleted_at IS NULL
		ORDER BY
			user_info.id
		LIMIT 1
	*/
	if res := dao.GormDB.Where("telephone = ?", telephone).First(&user); res.Error != nil {
		if errors.Is(res.Error, gorm.ErrRecordNotFound) {
			zlog.Info("该电话不存在，可以注册")
			return "", 0
		}
		zlog.Error(res.Error.Error())
		return constants.SYSTEM_ERROR, -1
	}
	zlog.Info("该电话已经存在，注册失败")
	return "该电话已经存在，注册失败", -2
}

// Register 用户注册功能
// 处理用户注册请求，验证验证码，创建新用户账户
// 参数: registerReq - 包含用户注册信息的请求对象(手机号、验证码、密码、昵称等)
// 返回值:
//   - string: 操作结果消息(成功或失败的具体描述)
//   - *respond.RegisterRespond: 注册成功的用户信息响应对象，注册失败时为nil
//   - int: 操作状态码(0表示成功，负数表示不同类型的错误)
func (u *userInfoService) Register(registerReq request.RegisterRequest) (string, *respond.RegisterRespond, int) {
	// 构建Redis中存储验证码的键名
	key := "auth_code_" + registerReq.Telephone

	// 从Redis中获取该手机号对应的验证码
	code, err := myredis.GetKey(key)
	if err != nil {
		zlog.Error(err.Error())
		return constants.SYSTEM_ERROR, nil, -1
	}

	// 验证用户输入的验证码是否正确
	if code != registerReq.SmsCode {
		message := "验证码不正确，请重试"
		zlog.Info(message)
		return message, nil, -2
	} else {
		// 验证码正确，删除Redis中的验证码(一次性使用)
		if err := myredis.DelKeyIfExists(key); err != nil {
			zlog.Error(err.Error())
			return constants.SYSTEM_ERROR, nil, -1
		}
	}

	// 不用校验手机号，前端校验
	// 检查手机号是否已被注册
	message, ret := u.checkTelephoneExist(registerReq.Telephone)
	if ret != 0 {
		return message, nil, ret
	}

	// 创建新用户对象
	var newUser model.UserInfo

	// 生成用户UUID (格式为 U + 11位随机字符串)
	newUser.Uuid = "U" + random.GetNowAndLenRandomString(11)

	// 设置用户基本信息
	newUser.Telephone = registerReq.Telephone // 手机号
	newUser.Password = registerReq.Password   // 密码
	newUser.Nickname = registerReq.Nickname   // 昵称

	// 设置默认头像（使用项目内置的默认头像文件）
	newUser.Avatar = "/static/avatars/default-user-avatar.png"

	// 设置创建时间
	newUser.CreatedAt = time.Now()

	// 新注册用户默认不是管理员
	newUser.IsAdmin = 0

	// 设置用户状态为正常
	newUser.Status = user_status_enum.NORMAL

	// 将新用户信息保存到数据库
	res := dao.GormDB.Create(&newUser)
	if res.Error != nil {
		zlog.Error(res.Error.Error())
		return constants.SYSTEM_ERROR, nil, -1
	}

	// 构建注册响应对象
	registerRsp := &respond.RegisterRespond{
		Uuid:      newUser.Uuid,      // 用户UUID
		Telephone: newUser.Telephone, // 手机号
		Nickname:  newUser.Nickname,  // 昵称
		Email:     newUser.Email,     // 邮箱
		Avatar:    newUser.Avatar,    // 头像
		Gender:    newUser.Gender,    // 性别
		Birthday:  newUser.Birthday,  // 生日
		Signature: newUser.Signature, // 个性签名
		IsAdmin:   newUser.IsAdmin,   // 管理员权限
		Status:    newUser.Status,    // 用户状态
	}

	// 格式化用户创建日期，提取年月日信息
	year, month, day := newUser.CreatedAt.Date()
	registerRsp.CreatedAt = fmt.Sprintf("%d.%d.%d", year, month, day)

	// 返回注册成功的信息
	return "注册成功", registerRsp, 0
}

// UpdateUserInfo 修改用户信息
// 更新用户的个人信息（邮箱、昵称、生日、签名、头像等），支持部分字段更新
// 参数: updateReq - 包含需要更新的用户信息的请求对象
// 返回值:
//   - string: 操作结果消息，成功或失败的具体描述
//   - int: 状态码，0表示成功，负数表示错误
//
// 说明：
//   - 当用户修改信息时，可能影响联系人列表显示，但不需要立即清除Redis中的联系人列表缓存
//   - 因为联系人列表会在超时后自动更新
//   - 但需要更新Redis中的用户信息缓存，以确保用户搜索等功能能获取最新数据
func (u *userInfoService) UpdateUserInfo(updateReq request.UpdateUserInfoRequest) (string, int) {
	// 查询数据库，根据UUID获取当前用户信息
	var user model.UserInfo
	if res := dao.GormDB.First(&user, "uuid = ?", updateReq.Uuid); res.Error != nil {
		// 如果查询失败，记录错误日志并返回系统错误
		zlog.Error(res.Error.Error())
		return constants.SYSTEM_ERROR, -1
	}

	// 检查并更新用户邮箱（仅当请求中提供了新邮箱时）
	if updateReq.Email != "" {
		user.Email = updateReq.Email
	}
	// 检查并更新用户昵称（仅当请求中提供了新昵称时）
	if updateReq.Nickname != "" {
		user.Nickname = updateReq.Nickname
	}
	// 检查并更新用户生日（仅当请求中提供了新生日时）
	if updateReq.Birthday != "" {
		user.Birthday = updateReq.Birthday
	}
	// 检查并更新用户签名（仅当请求中提供了新签名时）
	if updateReq.Signature != "" {
		user.Signature = updateReq.Signature
	}
	// 检查并更新用户头像（仅当请求中提供了新头像时）
	if updateReq.Avatar != "" {
		user.Avatar = updateReq.Avatar
	}

	// 将更新后的用户信息保存到数据库
	if res := dao.GormDB.Save(&user); res.Error != nil {
		// 如果保存失败，记录错误日志并返回系统错误
		zlog.Error(res.Error.Error())
		return constants.SYSTEM_ERROR, -1
	}

	// TODO: 在实际部署中，应该取消下面的注释以清除Redis中的用户信息缓存
	// 使Redis中对应用户的缓存失效，以便下次查询时获取最新数据
	if err := myredis.DelKeysWithPattern("user_info_" + updateReq.Uuid); err != nil {
		zlog.Error(err.Error())
		return constants.SYSTEM_ERROR, -1
	}

	// 返回成功消息
	return "修改用户信息成功", 0
}

// GetUserInfo 获取用户信息
// 根据UUID获取指定用户的信息，优先从Redis缓存获取，若缓存不存在则从数据库查询
// 参数: uuid - 需要查询的用户UUID
// 返回值:
//   - string: 操作结果消息，成功或失败的具体描述
//   - *respond.GetUserInfoRespond: 用户信息响应对象
//   - int: 状态码，0表示成功，负数表示错误
func (u *userInfoService) GetUserInfo(uuid string) (string, *respond.GetUserInfoRespond, int) {
	// 记录要查询的用户UUID
	zlog.Info(uuid)
	// 尝试从Redis缓存中获取用户信息
	rspString, err := myredis.GetKeyNilIsErr("user_info_" + uuid)
	// 检查从Redis获取数据是否出错
	if err != nil {
		// 如果错误是"键不存在"（redis.Nil），说明缓存中没有该用户信息
		if errors.Is(err, redis.Nil) {
			zlog.Info(err.Error())
			// 从数据库中查询用户信息
			var user model.UserInfo
			if res := dao.GormDB.Where("uuid = ?", uuid).Find(&user); res.Error != nil {
				// 数据库查询失败，记录错误并返回系统错误
				zlog.Error(res.Error.Error())
				return constants.SYSTEM_ERROR, nil, -1
			}

			// 构造用户信息响应对象
			rsp := respond.GetUserInfoRespond{
				Uuid:      user.Uuid,                                    // 用户UUID
				Telephone: user.Telephone,                               // 用户手机号
				Nickname:  user.Nickname,                                // 用户昵称
				Avatar:    user.Avatar,                                  // 用户头像
				Birthday:  user.Birthday,                                // 用户生日
				Email:     user.Email,                                   // 用户邮箱
				Gender:    user.Gender,                                  // 用户性别
				Signature: user.Signature,                               // 用户个性签名
				CreatedAt: user.CreatedAt.Format("2006-01-02 15:04:05"), // 用户创建时间
				IsAdmin:   user.IsAdmin,                                 // 是否为管理员
				Status:    user.Status,                                  // 用户状态
			}

			// TODO: 在实际部署中，可以取消下面的注释来将查询结果存入Redis缓存
			// 将响应对象序列化为JSON字符串
			rspString, err := json.Marshal(rsp)
			if err != nil {
				zlog.Error(err.Error())
			}

			// 将用户信息存入Redis缓存，设置过期时间
			if err := myredis.SetKeyEx("user_info_"+uuid, string(rspString), constants.REDIS_TIMEOUT*time.Minute); err != nil {
				zlog.Error(err.Error())
			}

			// 返回数据库查询结果
			return "获取用户信息成功", &rsp, 0
		} else {
			// 如果是其他Redis错误（不仅仅是键不存在），记录错误并返回系统错误
			zlog.Error(err.Error())
			return constants.SYSTEM_ERROR, nil, -1
		}
	}

	// 如果Redis中存在缓存数据，则将JSON字符串反序列化为响应对象
	var rsp respond.GetUserInfoRespond
	if err := json.Unmarshal([]byte(rspString), &rsp); err != nil {
		// JSON反序列化失败，记录错误
		zlog.Error(err.Error())
	}

	// 返回缓存中的用户信息
	return "获取用户信息成功", &rsp, 0
}

// GetUserInfoList 获取用户列表（排除指定用户）
// 获取系统中除指定用户外的所有用户信息，主要用于管理员功能
// 参数: ownerId - 需要排除的用户UUID（通常是当前管理员自己的UUID）
// 返回值:
//   - string: 操作结果消息，成功或失败的具体描述
//   - []respond.GetUserListRespond: 用户信息列表响应对象
//   - int: 状态码，0表示成功，负数表示错误
//
// 说明：
//   - 由于管理员数量较少，且用户信息变动频繁，使用Redis缓存会增加复杂性
//   - 因此管理员相关的用户列表暂不使用Redis缓存，直接查询数据库
func (u *userInfoService) GetUserInfoList(ownerId string) (string, []respond.GetUserListRespond, int) {
	// 从数据库中查询所有用户（包含软删除的记录）
	var users []model.UserInfo
	// 使用Unscoped()查询包括已软删除的用户在内的所有用户
	// 排除指定的ownerId用户，避免在列表中显示自己
	if res := dao.GormDB.Unscoped().Where("uuid != ?", ownerId).Find(&users); res.Error != nil {
		// 如果查询失败，记录错误日志并返回系统错误
		zlog.Error(res.Error.Error())
		return constants.SYSTEM_ERROR, nil, -1
	}

	// 初始化响应对象数组
	var rsp []respond.GetUserListRespond
	// 遍历查询到的用户列表，构造响应对象
	for _, user := range users {
		// 创建单个用户的响应对象，包含基本信息
		rp := respond.GetUserListRespond{
			Uuid:      user.Uuid,      // 用户UUID
			Telephone: user.Telephone, // 用户手机号
			Nickname:  user.Nickname,  // 用户昵称
			Status:    user.Status,    // 用户状态
			IsAdmin:   user.IsAdmin,   // 是否为管理员
		}

		// 判断用户是否已被软删除，并设置相应标志
		if user.DeletedAt.Valid {
			// 用户已被软删除
			rp.IsDeleted = true
		} else {
			// 用户未被软删除
			rp.IsDeleted = false
		}

		// 将单个用户响应对象添加到响应数组中
		rsp = append(rsp, rp)
	}

	// 返回成功消息和用户列表
	return "获取用户列表成功", rsp, 0
}

// AbleUsers 启用用户
// 将指定UUID列表中的用户状态设置为启用状态（NORMAL）
// 参数: uuidList - 需要启用的用户UUID列表
// 返回值:
//   - string: 操作结果消息，成功或失败的具体描述
//   - int: 状态码，0表示成功，负数表示错误
//
// 说明：
//   - 用户启用/禁用操作需要实时更新联系人列表状态，因此需要清除Redis中的联系人列表缓存
//   - 这样可以确保联系人列表能够反映出最新的用户状态
func (u *userInfoService) AbleUsers(uuidList []string) (string, int) {
	// 查询需要启用的用户信息
	var users []model.UserInfo
	if res := dao.GormDB.Where("uuid in (?)", uuidList).Find(&users); res.Error != nil {
		// 如果查询失败，记录错误日志并返回系统错误
		zlog.Error(res.Error.Error())
		return constants.SYSTEM_ERROR, -1
	}

	// 遍历用户列表，将每个用户的状态设置为正常（启用）
	for _, user := range users {
		// 设置用户状态为正常状态（启用）
		user.Status = user_status_enum.NORMAL

		// 保存用户状态更改到数据库
		if res := dao.GormDB.Save(&user); res.Error != nil {
			// 如果保存失败，记录错误日志并返回系统错误
			zlog.Error(res.Error.Error())
			return constants.SYSTEM_ERROR, -1
		}
	}

	// TODO: 在实际部署中，应取消下面的注释以清除Redis中的联系人列表缓存
	// 清除联系人列表缓存，确保联系人列表反映最新的用户状态
	// 删除所有"contact_user_list"开头的key
	if err := myredis.DelKeysWithPrefix("contact_user_list"); err != nil {
		zlog.Error(err.Error())
	}

	// 返回启用用户成功的消息
	return "启用用户成功", 0
}

// DisableUsers 禁用用户
// 将指定UUID列表中的用户状态设置为禁用状态（DISABLE），并处理相关数据
// 参数: uuidList - 需要禁用的用户UUID列表
// 返回值:
//   - string: 操作结果消息，成功或失败的具体描述
//   - int: 状态码，0表示成功，负数表示错误
//
// 说明：
//   - 用户禁用操作需要实时更新联系人列表状态，因此需要清除Redis中的联系人列表缓存
//   - 同时还会软删除该用户相关的会话记录
func (u *userInfoService) DisableUsers(uuidList []string) (string, int) {
	// 查询需要禁用的用户信息
	var users []model.UserInfo
	if res := dao.GormDB.Where("uuid in (?)", uuidList).Find(&users); res.Error != nil {
		// 如果查询失败，记录错误日志并返回系统错误
		zlog.Error(res.Error.Error())
		return constants.SYSTEM_ERROR, -1
	}

	// 遍历用户列表，处理每个用户的禁用操作
	for _, user := range users {
		// 设置用户状态为禁用状态
		user.Status = user_status_enum.DISABLE
		if res := dao.GormDB.Save(&user); res.Error != nil {
			// 如果保存用户状态失败，记录错误日志并返回系统错误
			zlog.Error(res.Error.Error())
			return constants.SYSTEM_ERROR, -1
		}

		// 查询该用户相关的所有会话记录（作为发送方或接收方）
		var sessionList []model.Session
		if res := dao.GormDB.Where("send_id = ? or receive_id = ?", user.Uuid, user.Uuid).Find(&sessionList); res.Error != nil {
			// 如果查询会话失败，记录错误日志并返回系统错误
			zlog.Error(res.Error.Error())
			return constants.SYSTEM_ERROR, -1
		}

		// 遍历会话列表，软删除每个相关的会话记录
		for _, session := range sessionList {
			// 创建软删除时间戳
			var deletedAt gorm.DeletedAt
			deletedAt.Time = time.Now()
			deletedAt.Valid = true
			// 设置会话记录的删除时间
			session.DeletedAt = deletedAt
			if res := dao.GormDB.Save(&session); res.Error != nil {
				// 如果保存会话删除状态失败，记录错误日志并返回系统错误
				zlog.Error(res.Error.Error())
				return constants.SYSTEM_ERROR, -1
			}
		}
	}

	// TODO: 在实际部署中，应取消下面的注释以清除Redis中的联系人列表缓存
	// 清除联系人列表缓存，确保联系人列表反映最新的用户状态
	// 删除所有"contact_user_list"开头的key
	if err := myredis.DelKeysWithPrefix("contact_user_list"); err != nil {
		zlog.Error(err.Error())
	}

	// 返回禁用用户成功的消息
	return "禁用用户成功", 0
}

// DeleteUsers 删除用户
// 对指定UUID列表中的用户执行软删除操作，并删除其相关的所有数据
// 参数: uuidList - 需要删除的用户UUID列表
// 返回值:
//   - string: 操作结果消息，成功或失败的具体描述
//   - int: 状态码，0表示成功，负数表示错误
//
// 说明：
//   - 用户删除操作是软删除，仅标记删除时间而不实际从数据库移除记录
//   - 同时会软删除该用户相关的会话、联系人和申请记录
//   - 需要清除联系人列表缓存以确保数据一致性
func (u *userInfoService) DeleteUsers(uuidList []string) (string, int) {
	// 查询需要删除的用户信息
	var users []model.UserInfo
	if res := dao.GormDB.Where("uuid in (?)", uuidList).Find(&users); res.Error != nil {
		// 如果查询失败，记录错误日志并返回系统错误
		zlog.Error(res.Error.Error())
		return constants.SYSTEM_ERROR, -1
	}

	// 遍历用户列表，处理每个用户的删除操作
	for _, user := range users {
		// 设置软删除时间戳，标记用户为已删除状态
		user.DeletedAt.Valid = true
		user.DeletedAt.Time = time.Now()
		if res := dao.GormDB.Save(&user); res.Error != nil {
			// 如果保存用户删除状态失败，记录错误日志并返回系统错误
			zlog.Error(res.Error.Error())
			return constants.SYSTEM_ERROR, -1
		}

		// 软删除该用户相关的所有会话记录（作为发送方或接收方）
		var sessionList []model.Session
		if res := dao.GormDB.Where("send_id = ? or receive_id = ?", user.Uuid, user.Uuid).Find(&sessionList); res.Error != nil {
			// 如果没有找到相关会话（正常情况），记录信息日志
			if errors.Is(res.Error, gorm.ErrRecordNotFound) {
				zlog.Info(res.Error.Error())
			} else {
				// 如果查询会话出现其他错误，记录错误日志并返回系统错误
				zlog.Error(res.Error.Error())
				return constants.SYSTEM_ERROR, -1
			}
		}

		// 遍历会话列表，软删除每个相关的会话记录
		for _, session := range sessionList {
			// 创建软删除时间戳
			var deletedAt gorm.DeletedAt
			deletedAt.Time = time.Now()
			deletedAt.Valid = true
			// 设置会话记录的删除时间
			session.DeletedAt = deletedAt
			if res := dao.GormDB.Save(&session); res.Error != nil {
				// 如果保存会话删除状态失败，记录错误日志并返回系统错误
				zlog.Error(res.Error.Error())
				return constants.SYSTEM_ERROR, -1
			}
		}

		// 软删除该用户相关的所有联系人记录
		var contactList []model.UserContact
		if res := dao.GormDB.Where("user_id = ? or contact_id = ?", user.Uuid, user.Uuid).Find(&contactList); res.Error != nil {
			// 如果没有找到相关联系人（正常情况），记录信息日志
			if errors.Is(res.Error, gorm.ErrRecordNotFound) {
				zlog.Info(res.Error.Error())
			} else {
				// 如果查询联系人出现其他错误，记录错误日志并返回系统错误
				zlog.Error(res.Error.Error())
				return constants.SYSTEM_ERROR, -1
			}
		}

		// 遍历联系人列表，软删除每个相关的联系人记录
		for _, contact := range contactList {
			// 创建软删除时间戳
			var deletedAt gorm.DeletedAt
			deletedAt.Time = time.Now()
			deletedAt.Valid = true
			// 设置联系人记录的删除时间
			contact.DeletedAt = deletedAt
			if res := dao.GormDB.Save(&contact); res.Error != nil {
				// 如果保存联系人删除状态失败，记录错误日志并返回系统错误
				zlog.Error(res.Error.Error())
				return constants.SYSTEM_ERROR, -1
			}
		}

		// 软删除该用户相关的所有联系人申请记录
		var applyList []model.ContactApply
		if res := dao.GormDB.Where("user_id = ? or contact_id = ?", user.Uuid, user.Uuid).Find(&applyList); res.Error != nil {
			// 如果没有找到相关申请记录（正常情况），记录信息日志
			if errors.Is(res.Error, gorm.ErrRecordNotFound) {
				zlog.Info(res.Error.Error())
			} else {
				// 如果查询申请记录出现其他错误，记录错误日志并返回系统错误
				zlog.Error(res.Error.Error())
				return constants.SYSTEM_ERROR, -1
			}
		}

		// 遍历申请列表，软删除每个相关的申请记录
		for _, apply := range applyList {
			// 创建软删除时间戳
			var deletedAt gorm.DeletedAt
			deletedAt.Time = time.Now()
			deletedAt.Valid = true
			// 设置申请记录的删除时间
			apply.DeletedAt = deletedAt
			if res := dao.GormDB.Save(&apply); res.Error != nil {
				// 如果保存申请删除状态失败，记录错误日志并返回系统错误
				zlog.Error(res.Error.Error())
				return constants.SYSTEM_ERROR, -1
			}
		}

	}

	// TODO: 在实际部署中，应取消下面的注释以清除Redis中的联系人列表缓存
	// 清除联系人列表缓存，确保联系人列表反映最新的用户状态
	// 删除所有"contact_user_list"开头的key
	if err := myredis.DelKeysWithPrefix("contact_user_list"); err != nil {
		zlog.Error(err.Error())
	}

	// 返回删除用户成功的消息
	return "删除用户成功", 0
}

// SetAdmin 设置管理员
func (u *userInfoService) SetAdmin(uuidList []string, isAdmin int8) (string, int) {
	var users []model.UserInfo
	if res := dao.GormDB.Where("uuid = (?)", uuidList).Find(&users); res.Error != nil {
		zlog.Error(res.Error.Error())
		return constants.SYSTEM_ERROR, -1
	}

	for _, user := range users {
		user.IsAdmin = isAdmin
		if res := dao.GormDB.Save(&user); res.Error != nil {
			zlog.Error(res.Error.Error())
			return constants.SYSTEM_ERROR, -1
		}
	}

	return "设置管理员成功", 0
}
