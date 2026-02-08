<template>
    <!-- 登录页面主容器 -->
    <div class="login-wrap">
        <!-- 登录窗口 -->
        <div
            class="login-window"
            :style="{
                boxShadow: `var(${'--el-box-shadow-dark'})`,
            }"
        >
            <!-- 登录标题 -->
            <h2 class="login-item">登录</h2>
            
            <!-- 登录表单 -->
            <el-form
                :model="loginData"
                label-width="70px"
                class="demo-dynamic"
            >
                <!-- 账号输入框 -->
                <el-form-item
                    prop="telephone"
                    label="账号"
                    :rules="[
                        {
                            required: true,
                            message: '此项为必填项',
                            trigger: 'blur',
                        },
                    ]"
                >
                    <el-input v-model="loginData.telephone" />
                </el-form-item>
                
                <!-- 密码输入框 -->
                <el-form-item
                    prop="password"
                    label="密码"
                    :rules="[
                        {
                            required: true,
                            message: '此项为必填项',
                            trigger: 'blur',
                        },
                    ]"
                >
                    <el-input type="password" v-model="loginData.password" />
                </el-form-item>
            </el-form>
            
            <!-- 登录按钮容器 -->
            <div class="login-button-container">
                <el-button type="primary" class="login-btn" @click="handleLogin">
                    登录
                </el-button>
            </div>

            <!-- 注册和验证码登录按钮容器 -->
            <div class="go-register-button-container">
                <button class="go-register-btn" @click="handleRegister">注册</button>
                <button class="go-sms-btn" @click="handleSmsLogin">验证码登录</button>
            </div>
        </div>
    </div>
</template>

<script>
// 导入Vue响应式API
import { reactive, toRefs } from "vue";
// 导入axios用于网络请求
import axios from "axios";
// 导入路由
import { useRouter } from "vue-router";
// 导入Element Plus消息提示
import { ElMessage } from "element-plus";
// 导入Vuex存储
import { useStore } from "vuex";

// 导出登录组件
export default {
    name: "Login",
    
    // 组件设置
    setup() {
        // 响应式数据
        const data = reactive({
            // 登录表单数据
            loginData: {
                telephone: "", // 账号（手机号码）
                password: "", // 密码
            },
        });
        
        // 路由实例
        const router = useRouter();
        // 存储实例
        const store = useStore();
        
        // 登录处理函数
        const handleLogin = async () => {
            try {
                // 验证登录信息是否完整
                if (!data.loginData.telephone || !data.loginData.password) {
                    ElMessage.error("请填写完整登录信息。");
                    return;
                }
                
                // 验证手机号码格式
                if (!checkTelephoneValid()) {
                    ElMessage.error("请输入有效的手机号码。");
                    return;
                }
                
                // 打印后端URL和WebSocket URL
                console.log(store.state.backendUrl, store.state.wsUrl);
                
                // 发送登录请求
                const response = await axios.post(
                    store.state.backendUrl + "/login",
                    data.loginData
                );
                
                console.log(response);
                
                // 处理登录响应
                if (response.data.code == 200) {
                    // 检查账号状态
                    if (response.data.data.status == 1) {
                        ElMessage.error("该账号已被封禁，请联系管理员。");
                        return;
                    }
                    
                    try {
                        // 显示登录成功消息
                        ElMessage.success(response.data.message);
                        
                        // 处理头像URL
                        if (!response.data.data.avatar.startsWith("http")) {
                            response.data.data.avatar = 
                                store.state.backendUrl + response.data.data.avatar;
                        }
                        
                        // 存储用户信息到Vuex
                        store.commit("setUserInfo", response.data.data);
                        
                        // 准备创建WebSocket连接
                        const wsUrl = 
                            store.state.wsUrl + "/wss?client_id=" + response.data.data.uuid;
                        console.log(wsUrl);
                        
                        // 创建WebSocket连接
                        store.state.socket = new WebSocket(wsUrl);
                        
                        // WebSocket事件处理
                        store.state.socket.onopen = () => {
                            console.log("WebSocket连接已打开");
                        };
                        store.state.socket.onmessage = (message) => {
                            console.log("收到消息：", message.data);
                        };
                        store.state.socket.onclose = () => {
                            console.log("WebSocket连接已关闭");
                        };
                        store.state.socket.onerror = () => {
                            console.log("WebSocket连接发生错误");
                        };
                        
                        // 跳转到聊天会话列表页面
                        router.push("/chat/sessionlist");
                    } catch (error) {
                        console.log(error);
                    }
                } else {
                    // 显示登录失败消息
                    ElMessage.error(response.data.message);
                }
            } catch (error) {
                // 显示网络错误消息
                ElMessage.error(error);
            }
        };
        
        // 验证手机号码格式
        // ^ - 表示字符串的开始
        // 1 - 手机号码的第一个数字必须是1（中国大陆手机号的特征）
        // [3456789] - 第二个数字必须是3、4、5、6、7、8、9中的任意一个
        // \d{9} - 后面必须跟着9个数字
        // $ - 表示字符串的结束
        const checkTelephoneValid = () => {
            const regex = /^1[3456789]\d{9}$/;
            return regex.test(data.loginData.telephone);
        };
        
        // 跳转到注册页面
        const handleRegister = () => {
            router.push("/register");
        };
        
        // 跳转到验证码登录页面
        const handleSmsLogin = () => {
            router.push("/smsLogin");
        };

        // 返回组件需要的响应式数据和方法
        return {
            ...toRefs(data),
            router,
            handleLogin,
            handleRegister,
            handleSmsLogin,
        };
    },
};
</script>

<style>
/* 登录页面容器样式 */
.login-wrap {
    height: 100vh;
    background-image: url("@/assets/img/chat_server_background.jpg");
    background-size: cover;
    background-position: center;
    background-repeat: no-repeat;
}

/* 登录窗口样式 */
.login-window {
    background-color: rgb(255, 255, 255, 0.7);
    position: fixed;
    top: 50%;
    left: 50%;
    transform: translate(-50%, -50%);
    padding: 30px 50px;
    border-radius: 20px;
    /*opacity: 0.7;*/
}

/* 登录标题样式 */
.login-item {
    text-align: center;
    margin-bottom: 20px;
    color: #494949;
}

/* 登录按钮容器样式 */
.login-button-container {
    display: flex;
    justify-content: center; /* 水平居中 */
    margin-top: 20px; /* 可选，根据需要调整按钮与输入框之间的间距 */
    width: 100%;
}

/* 登录按钮样式 */
.login-btn,
.login-btn:hover {
    background-color: rgb(229, 132, 132);
    border: none;
    color: #ffffff;
    font-weight: bold;
}

/* 注册和验证码登录按钮容器样式 */
.go-register-button-container {
    display: flex;
    flex-direction: row-reverse;
    margin-top: 10px;
}

/* 注册和验证码登录按钮样式 */
.go-register-btn,
.go-sms-btn {
    background-color: rgba(255, 255, 255, 0);
    border: none;
    cursor: pointer;
    color: #d65b54;
    font-weight: bold;
    text-decoration: underline;
    text-underline-offset: 0.2em;
    margin-left: 10px;
}

/* 警告框样式 */
.el-alert {
    margin-top: 20px;
}
</style>