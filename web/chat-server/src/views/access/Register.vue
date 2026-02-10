<template>
    <!-- 注册页面主容器 -->
    <div class="register-wrap">
        <!-- 注册窗口 -->
        <div
            class="register-window"
            :style="{
                boxShadow: `var(${'--el-box-shadow-dark'})`,
            }"
        >
            <!-- 注册标题 -->
            <h2 class="register-item">注册</h2>
            <!-- 注册表单 -->
            <el-form
                ref="formRef"
                :model="registerData"
                label-width="70px"
                class="demo-dynamic"
            >
                <!-- 昵称输入项 -->
                <el-form-item
                    prop="nickname"
                    label="昵称"
                    :rules="[
                        {
                            required: true,
                            message: '此项为必填项',
                            trigger: 'blur',
                        },
                        {
                            min: 3,
                            max: 10,
                            message: '昵称长度在 3 到 10 个字符',
                            trigger: 'blur',
                        },
                    ]"
                >
                    <el-input v-model="registerData.nickname" />
                </el-form-item>
                <!-- 手机号码输入项 -->
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
                    <el-input v-model="registerData.telephone" />
                </el-form-item>
                <!-- 密码输入项 -->
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
                    <el-input type="password" v-model="registerData.password" />
                </el-form-item>
                <!-- 验证码输入项 -->
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
                    <el-input v-model="registerData.sms_code" style="max-width: 200px">
                        <template #append>
                            <el-button
                                @click="sendSmsCode"
                                style="background-color: rgb(229, 132, 132); color: #ffffff"
                                >点击发送</el-button
                            >
                        </template>
                    </el-input>
                </el-form-item>
            </el-form>
            <!-- 注册按钮容器 -->
            <div class="register-button-container">
                <el-button type="primary" class="register-btn" @click="handleRegister"
                    >注册</el-button
                >
            </div>
            <!-- 登录按钮容器 -->
            <div class="go-login-button-container">
                <button class="go-sms-login-btn" @click="handleSmsLogin">
                    验证码登录
                </button>
                <button class="go-password-login-btn" @click="handleLogin">
                    密码登录
                </button>
            </div>
        </div>
    </div>
</template>

<script>
// 导入Vue相关API
import { reactive, toRefs } from "vue";
// 导入网络请求库
import axios from "axios";
// 导入路由
import { useRouter } from "vue-router";
// 导入Element Plus消息组件
import { ElMessage } from "element-plus";
// 导入Vuex存储
import { useStore } from "vuex";

export default {
    name: "Register",
    setup() {
        // 响应式数据
        const data = reactive({
            registerData: {
                telephone: "", // 手机号码
                password: "", // 密码
                nickname: "", // 昵称
                sms_code: "", // 验证码
            },
        });
        
        // 路由实例
        const router = useRouter();
        // 存储实例
        const store = useStore();
        
        // 处理注册
        const handleRegister = async () => {
            try {
                // 验证表单数据完整性
                if (
                    !data.registerData.nickname ||
                    !data.registerData.telephone ||
                    !data.registerData.password ||
                    !data.registerData.sms_code
                ) {
                    ElMessage.error("请填写完整注册信息。");
                    return;
                }
                
                // 验证昵称长度
                if (
                    data.registerData.nickname.length < 3 ||
                    data.registerData.nickname.length > 10
                ) {
                    ElMessage.error("昵称长度在 3 到 10 个字符。");
                    return;
                }
                
                // 验证手机号码格式
                if (!checkTelephoneValid()) {
                    ElMessage.error("请输入有效的手机号码。");
                    return;
                }
                
                // 发送注册请求
                const response = await axios.post(
                    store.state.backendUrl + "/register",
                    data.registerData
                );
                
                // 处理注册响应
                if (response.data.code == 200) {
                    ElMessage.success(response.data.message);
                    console.log(response.data.message);
                    
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
                } else {
                    ElMessage.error(response.data.message);
                    console.log(response.data.message);
                }
            } catch (error) {
                ElMessage.error(error);
                console.log(error);
            }
        };
        
        // 验证手机号码格式
        const checkTelephoneValid = () => {
            // 中国大陆手机号码正则表达式
            const regex = /^1([38][0-9]|4[579]|5[^4]|6[6]|7[1-35-8]|9[189])\d{8}$/;
            return regex.test(data.registerData.telephone);
        };

        // 跳转到密码登录页面
        const handleLogin = () => {
            router.push("/login");
        };

        // 跳转到验证码登录页面
        const handleSmsLogin = () => {
            router.push("/smsLogin");
        };

        // 发送验证码
        const sendSmsCode = async () => {
            // 验证表单数据完整性
            if (
                !data.registerData.telephone ||
                !data.registerData.nickname ||
                !data.registerData.password
            ) {
                ElMessage.error("请填写完整注册信息。");
                return;
            }
            
            // 验证手机号码格式
            if (!checkTelephoneValid()) {
                ElMessage.error("请输入有效的手机号码。");
                return;
            }
            
            // 构建请求数据
            const req = {
                telephone: data.registerData.telephone,
            };
            
            // 发送验证码请求
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
        };

        // 返回暴露的变量和方法
        return {
            ...toRefs(data),
            router,
            handleRegister,
            handleLogin,
            handleSmsLogin,
            sendSmsCode,
        };
    },
};
</script>

<style>
/* 注册页面容器样式 */
.register-wrap {
    height: 100vh;
    background-image: url("@/assets/img/chat_server_background.jpg");
    background-size: cover;
    background-position: center;
    background-repeat: no-repeat;
}

/* 注册窗口样式 */
.register-window {
    background-color: rgb(255, 255, 255, 0.7);
    position: fixed;
    top: 50%;
    left: 50%;
    transform: translate(-50%, -50%);
    padding: 30px 50px;
    border-radius: 20px;
}

/* 注册标题样式 */
.register-item {
    text-align: center;
    margin-bottom: 20px;
    color: #494949;
}

/* 注册按钮容器样式 */
.register-button-container {
    display: flex;
    justify-content: center; /* 水平居中 */
    margin-top: 20px; /* 可选，根据需要调整按钮与输入框之间的间距 */
    width: 100%;
}

/* 注册按钮样式 */
.register-btn,
.register-btn:hover {
    background-color: rgb(229, 132, 132);
    border: none;
    color: #ffffff;
    font-weight: bold;
}

/* 提示框样式 */
.el-alert {
    margin-top: 20px;
}

/* 登录按钮容器样式 */
.go-login-button-container {
    display: flex;
    flex-direction: row-reverse;
    margin-top: 10px;
}

/* 登录按钮样式 */
.go-sms-login-btn,
.go-password-login-btn {
    background-color: rgba(255, 255, 255, 0);
    border: none;
    cursor: pointer;
    color: #d65b54;
    font-weight: bold;
    text-decoration: underline;
    text-underline-offset: 0.2em;
    margin-left: 10px;
}
</style>