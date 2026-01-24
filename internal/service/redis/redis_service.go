// Package redis 提供Redis缓存服务的封装，用于存储和管理应用的临时数据
package redis

import (
	"context"
	"errors"
	"fmt"
	"gochat/internal/config"
	"gochat/pkg/zlog"
	"strconv"
	"time"

	"github.com/go-redis/redis/v8"
)

// redisClient Redis客户端实例，用于执行Redis命令
var redisClient *redis.Client

// ctx 上下文对象，用于Redis命令执行
var ctx = context.Background()

/*
 * init 初始化函数，在包被导入时自动执行
 * 连接到Redis服务器并初始化客户端连接，获取配置信息并进行初始化设置
 */
func init() {
	// 获取配置信息
	conf := config.GetConfig()
	host := conf.RedisConfig.Host         // Redis服务器主机地址
	port := conf.RedisConfig.Port         // Redis服务器端口
	password := conf.RedisConfig.Password // Redis服务器密码
	db := conf.RedisConfig.Db             // Redis数据库编号

	// 构建Redis服务器地址
	addr := host + ":" + strconv.Itoa(port)

	// 创建Redis客户端连接
	redisClient = redis.NewClient(&redis.Options{
		Addr:     addr,     // 服务器地址
		Password: password, // 密码（如果没有设置密码则为空字符串）
		DB:       db,       // 选择的数据库编号
	})
}

/*
 * SetKeyEx 设置带过期时间的键值对
 * 参数:
 *   - key: 键名
 *   - value: 键值
 *   - timeout: 过期时间
 *
 * 返回值:
 *   - error: 错误信息，成功时为nil
 */
func SetKeyEx(key string, value string, timeout time.Duration) error {
	err := redisClient.Set(ctx, key, value, timeout).Err()
	if err != nil {
		return err
	}
	return nil
}

/*
 * GetKey 获取指定键的值
 * 参数:
 *   - key: 键名
 *
 * 返回值:
 *   - string: 键对应的值
 *   - error: 错误信息，成功时为nil，键不存在时返回空字符串和nil
 */
func GetKey(key string) (string, error) {
	value, err := redisClient.Get(ctx, key).Result()
	if err != nil {
		// 键不存在，返回空字符串和nil
		if errors.Is(err, redis.Nil) {
			zlog.Info("该key不存在")
			return "", nil
		}
		return "", err
	}
	return value, nil
}

/*
 * GetKeyNilIsErr 获取指定键的值，键不存在时返回错误
 * 参数:
 *   - key: 键名
 *
 * 返回值:
 *   - string: 键对应的值
 *   - error: 错误信息，成功时为nil，键不存在时也会返回错误
 */
func GetKeyNilIsErr(key string) (string, error) {
	value, err := redisClient.Get(ctx, key).Result()
	if err != nil {
		return "", err
	}
	return value, nil
}

/*
 * GetKeyWithPrefixNilIsErr 根据前缀获取键名，如果找不到或找到多个则返回错误
 * 参数:
 *   - prefix: 键名前缀
 *
 * 返回值:
 *   - string: 找到的键名
 *   - error: 错误信息，成功时为nil
 */
func GetKeyWithPrefixNilIsErr(prefix string) (string, error) {
	var keys []string
	var err error

	for {
		// 使用 Keys 命令迭代匹配的键
		keys, err = redisClient.Keys(ctx, prefix+"*").Result()

		// 获取键列表失败
		if err != nil {
			return "", err
		}

		if len(keys) == 0 {
			zlog.Info("没有找到相关前缀key")
			return "", redis.Nil
		}

		if len(keys) == 1 {
			zlog.Info(fmt.Sprintf("成功找到了相关前缀key %v", keys))
			return keys[0], nil
		} else {
			zlog.Error("找到了数量大于1的key，查找异常")
			return "", errors.New("找到了数量大于1的key，查找异常")
		}
	}

}

/*
 * GetKeyWithSuffixNilIsErr 根据后缀获取键名，如果找不到或找到多个则返回错误
 * 参数:
 *   - suffix: 键名后缀
 *
 * 返回值:
 *   - string: 找到的键名
 *   - error: 错误信息，成功时为nil
 */
func GetKeyWithSuffixNilIsErr(suffix string) (string, error) {
	var keys []string
	var err error

	for {
		// 使用 Keys 命令迭代匹配的键
		keys, err = redisClient.Keys(ctx, "*"+suffix).Result()

		// 获取键列表失败
		if err != nil {
			return "", err
		}

		if len(keys) == 0 {
			zlog.Info("没有找到相关后缀key")
			return "", redis.Nil
		}

		if len(keys) == 1 {
			zlog.Info(fmt.Sprintf("成功找到了相关后缀key %v", keys))
			return keys[0], nil
		} else {
			zlog.Error("找到了数量大于1的key，查找异常")
			return "", errors.New("找到了数量大于1的key，查找异常")
		}
	}

}

/*
 * DelKeyIfExists 如果键存在则删除
 * 参数:
 *   - key: 要删除的键名
 *
 * 返回值:
 *   - error: 错误信息，成功时为nil
 */
func DelKeyIfExists(key string) error {
	exists, err := redisClient.Exists(ctx, key).Result()

	// 检查键是否存在操作出现问题
	if err != nil {
		return err
	}

	// 键存在
	if exists == 1 {
		delErr := redisClient.Del(ctx, key).Err()
		if delErr != nil {
			return delErr
		}
	}
	// 无论键是否存在，都不返回错误
	return nil
}

/*
 * DelKeysWithPattern 根据模式删除多个键
 * 参数:
 *   - pattern: 键名匹配模式
 *
 * 返回值:
 *   - error: 错误信息，成功时为nil
 */
func DelKeysWithPattern(pattern string) error {
	var keys []string
	var err error

	for {
		// 使用 Keys 命令迭代匹配的键
		keys, err = redisClient.Keys(ctx, pattern).Result()
		if err != nil {
			return err
		}

		// 如果没有更多的键，则跳出循环
		if len(keys) == 0 {
			zlog.Info("没有找到对应key")
			break
		}

		// 删除找到的键
		if len(keys) > 0 {
			_, err = redisClient.Del(ctx, keys...).Result()
			if err != nil {
				return err
			}
			zlog.Info(fmt.Sprintf("成功删除相关对应key %v", keys))
		}
	}

	return nil
}

/*
 * DelKeysWithPrefix 根据前缀删除多个键
 * 参数:
 *   - prefix: 键名前缀
 *
 * 返回值:
 *   - error: 错误信息，成功时为nil
 */
func DelKeysWithPrefix(prefix string) error {
	var keys []string
	var err error

	for {
		// 使用 Keys 命令迭代匹配的键
		keys, err = redisClient.Keys(ctx, prefix+"*").Result()
		if err != nil {
			return err
		}

		// 如果没有更多的键，则跳出循环
		if len(keys) == 0 {
			zlog.Info("没有找到相关前缀key")
			break
		}

		// 删除找到的键
		if len(keys) > 0 {
			_, err = redisClient.Del(ctx, keys...).Result()
			if err != nil {
				return err
			}
			zlog.Info(fmt.Sprintf("成功删除相关前缀key %v", keys))
		}
	}

	return nil
}

/*
 * DelKeysWithSuffix 根据后缀删除多个键
 * 参数:
 *   - suffix: 键名后缀
 *
 * 返回值:
 *   - error: 错误信息，成功时为nil
 */
func DelKeysWithSuffix(suffix string) error {
	var keys []string
	var err error

	for {
		// 使用 Keys 命令迭代匹配的键
		keys, err = redisClient.Keys(ctx, "*"+suffix).Result()
		if err != nil {
			return err
		}

		// 如果没有更多的键，则跳出循环
		if len(keys) == 0 {
			zlog.Info("没有找到相关后缀key")
			break
		}

		// 删除找到的键
		if len(keys) > 0 {
			_, err = redisClient.Del(ctx, keys...).Result()
			if err != nil {
				return err
			}
			zlog.Info(fmt.Sprintf("成功删除相关后缀key %v", keys))
		}
	}

	return nil
}

/*
 * DeleteAllRedisKeys 清空Redis数据库中的所有键
 *
 * 返回值:
 *   - error: 错误信息，成功时为nil
 */
func DeleteAllRedisKeys() error {
	var cursor uint64 = 0
	for {
		keys, nextCursor, err := redisClient.Scan(ctx, cursor, "*", 0).Result()

		// 扫描操作出现问题
		if err != nil {
			return err
		}
		// 更新游标
		cursor = nextCursor

		if len(keys) > 0 {
			_, err := redisClient.Del(ctx, keys...).Result()
			if err != nil {
				return err
			}
		}

		if cursor == 0 {
			break
		}
	}
	return nil
}
