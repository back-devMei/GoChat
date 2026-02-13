<template>
    <!-- 登录页面容器 -->
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
                ref="formRef"
                :model="loginData"
                label-width="70px"
                class="demo-dynamic"
            >
                <!-- 手机号输入框 -->
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
                <!-- 验证码输入框 -->
                <el-form-item
                    prop="sms_code"
                    label="验证码"
                    :rules="[
                        {
                            required: true,
                            message: '此项为必填项',
                            trigger: 'blur',
                        },
                    ]"
                >
                    <el-input v-model="loginData.sms_code" style="max-width: 200px">
                        <template #append>
                            <!-- 发送验证码按钮 -->
                            <el-button
                                @click="sendSmsCode"
                                style="background-color: rgb(229, 132, 132); color: #ffffff"
                            >
                                点击发送
                            </el-button>
                        </template>
                    </el-input>
                </el-form-item>
            </el-form>
            <!-- 登录按钮容器 -->
            <div class="login-button-container">
                <el-button type="primary" class="login-btn" @click="handleSmsLogin">
                    登录
                </el-button>
            </div>

            <!-- 切换按钮容器 -->
            <div class="go-register-button-container">
                <button class="go-register-btn" @click="handleRegister">注册</button>
                <button class="go-sms-btn" @click="handleLogin">密码登录</button>
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
// 导入Element Plus消息组件
import { ElMessage } from "element-plus";
// 导入Vuex存储
import { useStore } from "vuex";

export default {
    name: "smsLogin",
    setup() {
        // 响应式数据
        const data = reactive({
            loginData: {
                telephone: "", // 手机号码
                sms_code: "", // 验证码
            },
        });
        
        // 路由实例
        const router = useRouter();
        // 存储实例
        const store = useStore();
        
        // 处理短信登录
        const handleSmsLogin = async () => {
            try {
                // 验证输入
                if (!data.loginData.telephone || !data.loginData.sms_code) {
                    ElMessage.error("请填写完整登录信息。");
                    return;
                }
                // 验证手机号格式
                if (!checkTelephoneValid()) {
                    ElMessage.error("请输入有效的手机号码。");
                    return;
                }
                
                // 发送登录请求
                const response = await axios.post(
                    store.state.backendUrl + "/user/smsLogin",
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
                        // 登录成功
                        ElMessage.success(response.data.message);
                        
                        // 处理头像URL
                        if (!response.data.data.avatar.startsWith("http")) {
                            response.data.data.avatar =
                                store.state.backendUrl + response.data.data.avatar;
                        }
                        
                        // 存储用户信息
                        store.commit("setUserInfo", response.data.data);
                        
                        // 创建WebSocket连接
                        const wsUrl =
                            store.state.wsUrl + "/wss?client_id=" + response.data.data.uuid;
                        console.log(wsUrl);
                        
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
                        
                        // 跳转到聊天页面
                        router.push("/chat/sessionlist");
                    } catch (error) {
                        console.log(error);
                    }
                } else {
                    // 登录失败
                    ElMessage.error(response.data.message);
                }
            } catch (error) {
                // 网络错误
                ElMessage.error(error);
            }
        };
        
        // 验证手机号格式
        const checkTelephoneValid = () => {
            // 中国大陆手机号正则表达式
            const regex = /^1([38][0-9]|4[579]|5[^4]|6[6]|7[1-35-8]|9[189])\d{8}$/;
            return regex.test(data.loginData.telephone);
        };
        
        // 跳转到注册页面
        const handleRegister = () => {
            router.push("/register");
        };
        
        // 跳转到密码登录页面
        const handleLogin = () => {
            router.push("/login");
        };
        
        // 发送验证码
        const sendSmsCode = async () => {
            // 验证手机号
            if (!data.loginData.telephone) {
                ElMessage.error("请输入手机号码。");
                return;
            }
            if (!checkTelephoneValid()) {
                ElMessage.error("请输入有效的手机号码。");
                return;
            }
            
            try {
                // 构建请求数据
                const req = {
                    telephone: data.loginData.telephone,
                };
                
                // 发送请求
                const rsp = await axios.post(
                    store.state.backendUrl + "/user/sendSmsCode",
                    req
                );
                
                console.log(rsp);
                
                // 处理响应
                if (rsp.data.code == 200) {
                    ElMessage.success(rsp.data.message);
                } else if (rsp.data.code == 400) {
                    ElMessage.warning(rsp.data.message);
                } else {
                    ElMessage.error(rsp.data.message);
                }
            } catch (error) {
                console.error(error);
            }
        };

        // 返回暴露的变量和方法
        return {
            ...toRefs(data),
            router,
            handleSmsLogin,
            handleLogin,
            handleRegister,
            sendSmsCode,
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

/* 切换按钮容器样式 */
.go-register-button-container {
    display: flex;
    flex-direction: row-reverse;
    margin-top: 10px;
}

/* 切换按钮样式 */
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

/* 提示框样式 */
.el-alert {
    margin-top: 20px;
}
</style>