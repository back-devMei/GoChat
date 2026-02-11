<!-- 联系人列表模态框组件 -->
<!-- 功能：管理联系人、群聊、好友申请等 -->
<template>
    <div class="contactlist-container">
        <!-- 头部搜索和操作区域 -->
        <div class="contactlist-header">
            <!-- 搜索框 -->
            <el-input
                v-model="contactSearch"
                class="contact-search-input"
                placeholder="搜索联系人/群聊"
                size="small"
                suffix-icon="Search"
            />
            
            <!-- 右侧操作按钮 -->
            <div class="contactlist-header-right">
                <!-- 创建/添加操作下拉菜单 -->
                <el-dropdown placement="bottom" trigger="click">
                    <button class="create-group-btn">
                        <svg
                            t="1733664667695"
                            class="create-group-icon"
                            viewBox="0 0 1024 1024"
                            version="1.1"
                            xmlns="http://www.w3.org/2000/svg"
                            p-id="2875"
                            width="128"
                            height="128"
                        >
                            <path
                                d="M488.021333 96a248.021333 248.021333 0 1 1-17.92 495.36l-1.749333 0.341333-4.352 0.298667A304 304 0 0 0 160 896a32 32 0 1 1-64 0 368.170667 368.170667 0 0 1 250.026667-348.672A247.978667 247.978667 0 0 1 488.021333 96z m288 528a32 32 0 0 1 32 32l-0.042666 87.978667H896a32 32 0 0 1 31.701333 27.690666l0.298667 4.352a32 32 0 0 1-32 32l-88.021333-0.042666V896a32 32 0 0 1-27.648 31.701333l-4.352 0.298667a32 32 0 0 1-32-32v-88.021333h-87.978667a32 32 0 0 1-31.701333-27.648l-0.298667-4.352a32 32 0 0 1 32-32h87.978667v-87.978667a32 32 0 0 1 27.690666-31.701333zM488.021333 160a184.021333 184.021333 0 1 0 0 368 184.021333 184.021333 0 0 0 0-368z"
                                fill="#2c2c2c"
                                p-id="2876"
                            ></path>
                        </svg>
                    </button>
                    <template #dropdown>
                        <el-dropdown-menu>
                            <el-dropdown-item @click="showCreateGroupModal">
                                创建群聊
                            </el-dropdown-item>
                            <el-dropdown-item @click="showApplyContactModal">
                                添加用户/群聊
                            </el-dropdown-item>
                            <el-dropdown-item @click="showNewContactModal">
                                新的好友
                            </el-dropdown-item>
                        </el-dropdown-menu>
                    </template>
                </el-dropdown>
                
                <!-- 新的好友申请模态框 -->
                <SmallModal :isVisible="isNewContactModalVisible">
                    <template v-slot:header>
                        <div class="modal-header">
                            <div class="modal-quit-btn-container">
                                <button class="modal-quit-btn" @click="quitNewContactModal">
                                    <el-icon><Close /></el-icon>
                                </button>
                            </div>
                            <div class="modal-header-title">
                                <h3>新的朋友</h3>
                            </div>
                        </div>
                    </template>
                    <template v-slot:body>
                        <div class="newcontact-modal-body">
                            <el-scrollbar max-height="400px">
                                <ul class="newcontact-list" style="list-style-type: none">
                                    <li
                                        v-for="newContact in newContactList"
                                        :key="newContact.contact_id"
                                        class="newcontact-item"
                                    >
                                        <div
                                            style="
                                                display: flex;
                                                align-items: center;
                                                justify-content: center;
                                            "
                                        >
                                            <img
                                                :src="newContact.contact_avatar"
                                                style="width: 30px; height: 30px; margin-right: 10px"
                                            />

                                            <el-tooltip
                                                effect="customized"
                                                :content="newContact.message"
                                                placement="top"
                                                hide-after="0"
                                                enterable="false"
                                            >
                                                <div>
                                                    {{ newContact.contact_name }}
                                                </div>
                                            </el-tooltip>
                                        </div>
                                        <el-dropdown placement="right" trigger="click">
                                            <el-button class="action-btn"> 去处理 </el-button>
                                            <template #dropdown>
                                                <el-dropdown-menu>
                                                    <el-dropdown-item
                                                        @click="handleAgree(newContact.contact_id)"
                                                        >同意</el-dropdown-item
                                                    >
                                                    <el-dropdown-item
                                                        @click="handleReject(newContact.contact_id)"
                                                    >
                                                        拒绝
                                                    </el-dropdown-item>
                                                    <el-dropdown-item
                                                        @click="handleBlack(newContact.contact_id)"
                                                    >
                                                        拉黑
                                                    </el-dropdown-item>
                                                </el-dropdown-menu>
                                            </template>
                                        </el-dropdown>
                                    </li>
                                </ul>
                            </el-scrollbar>
                        </div>
                    </template>
                    <template v-slot:footer>
                        <div class="newcontact-modal-footer"></div>
                    </template>
                </SmallModal>
                
                <!-- 添加用户/群聊模态框 -->
                <SmallModal :isVisible="isApplyContactModalVisible">
                    <template v-slot:header>
                        <div class="modal-header">
                            <div class="modal-quit-btn-container">
                                <button class="modal-quit-btn" @click="quitApplyContactModal">
                                    <el-icon><Close /></el-icon>
                                </button>
                            </div>
                            <div class="modal-header-title">
                                <h3>添加用户/群聊</h3>
                            </div>
                        </div>
                    </template>
                    <template v-slot:body>
                        <div class="modal-body">
                            <el-form
                                ref="formRef"
                                :model="applyContactReq"
                                label-width="100px"
                                class="apply-contact-form"
                            >
                                <el-form-item
                                    prop="name"
                                    label="用户/群聊id"
                                    :rules="[
                                        {
                                            required: true,
                                            message: '此项为必填项',
                                            trigger: 'blur',
                                        },
                                    ]"
                                >
                                    <el-input
                                        v-model="applyContactReq.contact_id"
                                        placeholder="请填写申请的用户/群聊id"
                                    />
                                </el-form-item>
                                <el-form-item prop="message" label="申请消息">
                                    <el-input
                                        v-model="applyContactReq.message"
                                        placeholder="选填，填写更容易通过"
                                        type="textarea"
                                        show-word-limit
                                        maxlength="100"
                                        :autosize="{ minRows: 3, maxRows: 3 }"
                                    />
                                </el-form-item>
                            </el-form>
                        </div>
                    </template>
                    <template v-slot:footer>
                        <div class="modal-footer">
                            <el-button
                                class="modal-close-btn"
                                @click="closeApplyContactModal"
                            >
                                完成
                            </el-button>
                        </div>
                    </template>
                </SmallModal>
                
                <!-- 创建群聊模态框 -->
                <Modal :isVisible="isCreateGroupModalVisible">
                    <template v-slot:header>
                        <div class="modal-header">
                            <div class="modal-quit-btn-container">
                                <button class="modal-quit-btn" @click="quitCreateGroupModal">
                                    <el-icon><Close /></el-icon>
                                </button>
                            </div>
                            <div class="modal-header-title">
                                <h3>创建群聊</h3>
                            </div>
                        </div>
                    </template>
                    <template v-slot:body>
                        <el-scrollbar
                            max-height="300px"
                            style="
                                width: 400px;
                                display: flex;
                                align-items: center;
                                justify-content: center;
                                margin-top: 20px;
                            "
                        >
                            <div class="creatgroup-modal-body">
                                <el-form
                                    ref="formRef"
                                    :model="createGroupReq"
                                    label-width="80px"
                                    class="demo-dynamic"
                                >
                                    <el-form-item
                                        prop="name"
                                        label="群名称"
                                        :rules="[
                                            {
                                                required: true,
                                                message: '此项为必填项',
                                                trigger: 'blur',
                                            },
                                        ]"
                                    >
                                        <el-input
                                            v-model="createGroupReq.name"
                                            placeholder="必填"
                                        />
                                    </el-form-item>
                                    <el-form-item prop="notice" label="群公告">
                                        <el-input
                                            v-model="createGroupReq.notice"
                                            type="textarea"
                                            show-word-limit
                                            maxlength="500"
                                            :autosize="{ minRows: 3, maxRows: 3 }"
                                            placeholder="选填"
                                        />
                                    </el-form-item>
                                    <el-form-item
                                        prop="add_mode"
                                        label="加群方式"
                                        :rules="[
                                            {
                                                required: true,
                                                message: 'Please select activity resource',
                                                trigger: 'change',
                                            },
                                        ]"
                                    >
                                        <el-radio-group v-model="createGroupReq.add_mode">
                                            <el-radio :value="0">直接加入</el-radio>
                                            <el-radio :value="1">群主审核</el-radio>
                                        </el-radio-group>
                                    </el-form-item>
                                    <el-form-item prop="avatar" label="群头像">
                                        <el-upload
                                            v-model:file-list="fileList"
                                            ref="uploadRef"
                                            :auto-upload="false"
                                            :action="uploadPath"
                                            :on-success="handleUploadSuccess"
                                            :before-upload="beforeFileUpload"
                                        >
                                            <template #trigger>
                                                <el-button
                                                    style="background-color: rgb(252, 210.9, 210.9)"
                                                    >上传图片</el-button
                                                >
                                            </template>
                                        </el-upload>
                                    </el-form-item>
                                </el-form>
                            </div>
                        </el-scrollbar>
                    </template>
                    <template v-slot:footer>
                        <div class="creategroup-modal-footer">
                            <el-button class="modal-close-btn" @click="closeCreateGroupModal">
                                完成
                            </el-button>
                        </div>
                    </template>
                </Modal>
            </div>
        </div>
        
        <!-- 联系人列表主体 -->
        <div class="contactlist-body">
            <div class="contactlist-user">
                <!-- 联系人列表 -->
                <el-menu
                    router
                    unique-opened
                    @open="handleShowUserList"
                    @close="handleHideUserList"
                >
                    <el-sub-menu index="1">
                        <template #title>
                            <span class="contactlist-user-title">联系人</span>
                        </template>
                    </el-sub-menu>
                    <el-menu-item
                        v-for="user in contactUserList"
                        :key="user.user_id"
                        @click="handleToChatUser(user)"
                        class="contactlist-user-menu-item"
                    >
                        <el-dropdown
                            trigger="contextmenu"
                            class="contactlist-dropdown"
                            placement="right"
                        >
                            <div class="contactlist-user-item">
                                <img :src="user.avatar" class="contactlist-user-avatar" />
                                {{ user.user_name }}
                            </div>
                            <template #dropdown>
                                <el-dropdown-menu>
                                    <el-dropdown-item @click="handleCancelBlack(user)"
                                        >解除拉黑</el-dropdown-item
                                    >
                                    <!-- 其他菜单项 -->
                                </el-dropdown-menu>
                            </template>
                        </el-dropdown>
                    </el-menu-item>
                </el-menu>

                <!-- 我创建的群聊列表 -->
                <el-menu
                    router
                    unique-opened
                    @open="handleShowMyGroupList"
                    @close="handleHideMyGroupList"
                >
                    <el-sub-menu index="1">
                        <template #title>
                            <span class="contactlist-user-title">我创建的群聊</span>
                        </template>
                    </el-sub-menu>
                    <el-menu-item
                        v-for="group in myGroupList"
                        :key="group.group_id"
                        @click="handleToChatGroup(group)"
                    >
                        <img :src="group.avatar" class="contactlist-avatar" />
                        {{ group.group_name }}
                    </el-menu-item>
                </el-menu>
                
                <!-- 我加入的群聊列表 -->
                <el-menu
                    router
                    unique-opened
                    @open="handleShowMyJoinedGroupList"
                    @close="handleHideMyJoinedGroupList"
                >
                    <el-sub-menu index="1">
                        <template #title>
                            <span class="contactlist-user-title">我加入的群聊</span>
                        </template>
                    </el-sub-menu>
                    <el-menu-item
                        v-for="group in myJoinedGroupList"
                        :key="group.group_id"
                        @click="handleToChatGroup(group)"
                    >
                        <img :src="group.avatar" class="contactlist-avatar" />
                        {{ group.group_name }}
                    </el-menu-item>
                </el-menu>
            </div>
        </div>
    </div>
</template>

<script>
// 导入必要的依赖
import { reactive, toRefs, onMounted } from "vue";
import { useRouter } from "vue-router";
import { useStore } from "vuex";
import axios from "axios";
import { ElMessage } from "element-plus";
import Modal from "./Modal.vue";
import SmallModal from "./SmallModal.vue";

export default {
    name: "ContactListModal",
    props: {
        isVisible: true,
    },
    components: {
        Modal,
        SmallModal,
    },
    setup() {
        const router = useRouter();
        const store = useStore();
        
        // 响应式数据
        const data = reactive({
            chatMessage: "",
            chatName: "",
            userInfo: store.state.userInfo,
            contactSearch: "",
            // 创建群聊请求参数
            createGroupReq: {
                owner_id: "",
                name: "",
                notice: "",
                add_mode: null,
                avatar: "",
            },
            // 模态框显示状态
            isCreateGroupModalVisible: false,
            isApplyContactModalVisible: false,
            isNewContactModalVisible: false,
            // 联系人列表
            contactUserList: [],
            // 群聊列表
            myGroupList: [],
            myJoinedGroupList: [],
            // 添加联系人请求参数
            applyContactReq: {
                owner_id: "",
                contact_id: "",
                message: "",
            },
            // 通用请求参数
            ownListReq: {
                owner_id: "",
            },
            // 新的好友申请列表
            newContactList: [],
            applyContent: "",
            uploadRef: null,
            uploadPath: store.state.backendUrl + "/message/uploadAvatar",
            fileList: [],
            cnt: 0,
        });
        
        /**
         * 头像上传成功处理
         */
        const handleUploadSuccess = () => {
            ElMessage.success("头像上传成功");
            data.fileList = [];
        };
        
        /**
         * 上传前文件检查
         * @param {Object} file - 上传的文件对象
         * @returns {boolean} - 是否允许上传
         */
        const beforeFileUpload = (file) => {
            console.log("上传前file====>", file);
            console.log(data.fileList);
            console.log(file);
            if (data.fileList.length > 1) {
                ElMessage.error("只能上传一张头像");
                return false;
            }
            const isLt50M = file.size / 1024 / 1024 < 50;
            if (!isLt50M) {
                ElMessage.error("上传头像图片大小不能超过 50MB!");
                return false;
            }
        };
        
        /**
         * 创建群聊
         */
        const handleCreateGroup = async () => {
            try {
                data.createGroupReq.owner_id = data.userInfo.uuid;
                if (data.fileList.length > 0) {
                    data.createGroupReq.avatar =
                        "/static/avatars/" + data.fileList[0].name;
                    console.log(data.createGroupReq.avatar);
                    data.uploadRef.submit();
                }
                const response = await axios.post(
                    store.state.backendUrl + "/group/createGroup",
                    data.createGroupReq
                );
            } catch (error) {
                console.error(error);
            }
        };
        
        /**
         * 显示创建群聊模态框
         */
        const showCreateGroupModal = () => {
            data.isCreateGroupModalVisible = true;
        };
        
        /**
         * 关闭创建群聊模态框
         */
        const quitCreateGroupModal = () => {
            data.isCreateGroupModalVisible = false;
        };
        
        /**
         * 完成创建群聊
         */
        const closeCreateGroupModal = () => {
            if (data.createGroupReq.name == "") {
                ElMessage("请输入群聊名称");
                return;
            }
            if (data.createGroupReq.add_mode == null) {
                ElMessage("请选择加群方式");
                return;
            }
            data.isCreateGroupModalVisible = false;
            handleCreateGroup();
        };
        
        /**
         * 显示添加联系人模态框
         */
        const showApplyContactModal = () => {
            data.isApplyContactModalVisible = true;
        };
        
        /**
         * 关闭添加联系人模态框
         */
        const quitApplyContactModal = () => {
            data.isApplyContactModalVisible = false;
        };
        
        /**
         * 完成添加联系人
         */
        const closeApplyContactModal = () => {
            if (data.applyContactReq.contact_id == "") {
                ElMessage.error("请输入申请用户/群组id");
                return;
            }
            if (data.applyContactReq.contact_id[0] == "G") {
                handleApplyGroup();
            } else {
                handleApplyContact();
            }
        };

        /**
         * 显示新的好友模态框
         */
        const showNewContactModal = () => {
            handleNewContactList();
        };

        /**
         * 关闭新的好友模态框
         */
        const quitNewContactModal = () => {
            data.isNewContactModalVisible = false;
            data.newContactList = [];
        };
        
        /**
         * 申请加入群聊
         */
        const handleApplyGroup = async () => {
            try {
                let req = {
                    group_id: data.applyContactReq.contact_id,
                };
                let rsp = await axios.post(
                    store.state.backendUrl + "/group/checkGroupAddMode",
                    req
                );
                if (rsp.data.code == 200) {
                    if (rsp.data.data == 0) {
                        // 直接加入
                        handleEnterDirectly(data.applyContactReq.contact_id);
                        return;
                    }
                } else {
                    ElMessage.error("申请失败");
                    return;
                }
                data.applyContactReq.owner_id = data.userInfo.uuid;
                rsp = await axios.post(
                    store.state.backendUrl + "/contact/applyContact",
                    data.applyContactReq
                );
                console.log(rsp);
                if (rsp.data.code == 200) {
                    if (rsp.data.message == "申请成功") {
                        data.isApplyContactModalVisible = false;
                        ElMessage.success("申请成功");
                        return;
                    } else {
                        ElMessage.error(rsp.data.message);
                    }
                } else {
                    ElMessage.error("申请失败");
                }
            } catch (error) {
                console.error(error);
            }
        };
        
        /**
         * 申请添加好友
         */
        const handleApplyContact = async () => {
            try {
                data.applyContactReq.owner_id = data.userInfo.uuid;
                const rsp = await axios.post(
                    store.state.backendUrl + "/contact/applyContact",
                    data.applyContactReq
                );
                console.log(rsp);
                if (rsp.data.code == 200) {
                    if (rsp.data.message == "申请成功") {
                        data.isApplyContactModalVisible = false;
                        ElMessage.success("申请成功");
                        return;
                    }
                }
                ElMessage.error(rsp.data.message);
            } catch (error) {
                console.error(error);
            }
        };
        
        /**
         * 显示联系人列表
         */
        const handleShowUserList = async () => {
            try {
                data.ownListReq.owner_id = data.userInfo.uuid;
                const getUserListRsp = await axios.post(
                    store.state.backendUrl + "/contact/getUserList",
                    data.ownListReq
                );
                if (getUserListRsp.data.data) {
                    for (let i = 0; i < getUserListRsp.data.data.length; i++) {
                        if (!getUserListRsp.data.data[i].avatar.startsWith("http")) {
                            getUserListRsp.data.data[i].avatar =
                                store.state.backendUrl + getUserListRsp.data.data[i].avatar;
                        }
                    }
                }
                data.contactUserList = getUserListRsp.data.data;
            } catch (error) {
                console.error(error);
            }
        };
        
        /**
         * 隐藏联系人列表
         */
        const handleHideUserList = () => {
            data.contactUserList = [];
        };

        /**
         * 显示我创建的群聊列表
         */
        const handleShowMyGroupList = async () => {
            try {
                data.ownListReq.owner_id = data.userInfo.uuid;
                const loadMyGroupRsp = await axios.post(
                    store.state.backendUrl + "/group/loadMyGroup",
                    data.ownListReq
                );
                if (loadMyGroupRsp.data.data) {
                    for (let i = 0; i < loadMyGroupRsp.data.data.length; i++) {
                        if (!loadMyGroupRsp.data.data[i].avatar.startsWith("http")) {
                            loadMyGroupRsp.data.data[i].avatar =
                                store.state.backendUrl + loadMyGroupRsp.data.data[i].avatar;
                        }
                    }
                }
                data.myGroupList = loadMyGroupRsp.data.data;
            } catch (error) {
                console.error(error);
            }
        };
        
        /**
         * 隐藏我创建的群聊列表
         */
        const handleHideMyGroupList = () => {
            data.myGroupList = [];
        };
        
        /**
         * 显示我加入的群聊列表
         */
        const handleShowMyJoinedGroupList = async () => {
            try {
                data.ownListReq.owner_id = data.userInfo.uuid;
                const loadMyJoinedGroupRsp = await axios.post(
                    store.state.backendUrl + "/contact/loadMyJoinedGroup",
                    data.ownListReq
                );
                if (loadMyJoinedGroupRsp.data.data) {
                    for (let i = 0; i < loadMyJoinedGroupRsp.data.data.length; i++) {
                        if (!loadMyJoinedGroupRsp.data.data[i].avatar.startsWith("http")) {
                            loadMyJoinedGroupRsp.data.data[i].avatar =
                                store.state.backendUrl +
                                loadMyJoinedGroupRsp.data.data[i].avatar;
                        }
                    }
                }
                data.myJoinedGroupList = loadMyJoinedGroupRsp.data.data;
            } catch (error) {
                console.error(error);
            }
        };
        
        /**
         * 隐藏我加入的群聊列表
         */
        const handleHideMyJoinedGroupList = () => {
            data.myJoinedGroupList = [];
        };

        /**
         * 进入与用户的聊天界面
         * @param {Object} user - 用户信息
         */
        const handleToChatUser = async (user) => {
            try {
                const req = {
                    send_id: data.userInfo.uuid,
                    receive_id: user.user_id,
                };
                const rsp = await axios.post(
                    store.state.backendUrl + "/session/checkOpenSessionAllowed",
                    req
                );
                if (rsp.data.code == 200) {
                    if (rsp.data.data == true) {
                        router.push("/chat/" + user.user_id);
                    } else {
                        ElMessage.warning(rsp.data.message);
                        console.error(rsp.data.message);
                    }
                } else {
                    ElMessage.error(rsp.data.message);
                    console.error(rsp.data.message);
                }
            } catch (error) {
                ElMessage.error(error);
                console.error(error);
            }
        };

        /**
         * 进入与群聊的聊天界面
         * @param {Object} group - 群聊信息
         */
        const handleToChatGroup = async (group) => {
            try {
                const req = {
                    send_id: data.userInfo.uuid,
                    receive_id: group.group_id,
                };
                const rsp = await axios.post(
                    store.state.backendUrl + "/session/checkOpenSessionAllowed",
                    req
                );
                if (rsp.data.code == 200) {
                    if (rsp.data.data == true) {
                        router.push("/chat/" + group.group_id);
                    } else {
                        ElMessage.warning(rsp.data.message);
                        console.error(rsp.data.message);
                    }
                } else {
                    if (rsp.data.code == 400) {
                        ElMessage.warning(rsp.data.message);
                        console.error(rsp.data.message);
                    } else {
                        ElMessage.error(rsp.data.message);
                        console.error(rsp.data.message);
                    }
                }
            } catch (error) {
                console.error(error);
            }
        };

        /**
         * 获取新的好友申请列表
         */
        const handleNewContactList = async () => {
            try {
                data.ownListReq.owner_id = data.userInfo.uuid;
                const rsp = await axios.post(
                    store.state.backendUrl + "/contact/getNewContactList",
                    data.ownListReq
                );
                console.log(rsp);
                data.newContactList = rsp.data.data;
                if (data.newContactList == null) {
                    ElMessage.warning("没有新的好友申请");
                    return;
                }
                for (let i = 0; i < data.newContactList.length; i++) {
                    if (!data.newContactList[i].contact_avatar.startsWith("http")) {
                        data.newContactList[i].contact_avatar =
                            store.state.backendUrl + data.newContactList[i].contact_avatar;
                    }
                }
                data.isNewContactModalVisible = true;
                console.log(rsp);
            } catch (error) {
                console.error(error);
            }
        };

        /**
         * 同意好友申请
         * @param {string} contactId - 联系人ID
         */
        const handleAgree = async (contactId) => {
            try {
                const req = {
                    owner_id: data.userInfo.uuid,
                    contact_id: contactId,
                };
                const rsp = await axios.post(
                    store.state.backendUrl + "/contact/passContactApply",
                    req
                );
                console.log(rsp);
                if (rsp.data.code == 200) {
                    ElMessage.success(rsp.data.message);
                    data.newContactList = data.newContactList.filter(
                        (c) => c.contact_id !== contactId
                    );
                } else {
                    ElMessage.error(rsp.data.message);
                }
            } catch (error) {
                console.error(error);
            }
        };
        
        /**
         * 直接加入群聊
         * @param {string} groupId - 群聊ID
         */
        const handleEnterDirectly = async (groupId) => {
            try {
                const req = {
                    owner_id: groupId,
                    contact_id: data.userInfo.uuid,
                };
                const rsp = await axios.post(
                    store.state.backendUrl + "/group/enterGroupDirectly",
                    req
                );
                console.log(rsp);
                if (rsp.data.code == 200) {
                    ElMessage.success(rsp.data.message);
                    data.isApplyContactModalVisible = false;
                } else {
                    ElMessage.error(rsp.data.message);
                }
            } catch (error) {
                console.error(error);
            }
        };
        
        /**
         * 拒绝好友申请
         * @param {string} contactId - 联系人ID
         */
        const handleReject = async (contactId) => {
            try {
                const req = {
                    owner_id: data.userInfo.uuid,
                    contact_id: contactId,
                };
                const rsp = await axios.post(
                    store.state.backendUrl + "/contact/refuseContactApply",
                    req
                );
                console.log(rsp);
                if (rsp.data.code == 200) {
                    ElMessage.success(rsp.data.message);
                    console.log(rsp.data.message);
                    data.newContactList = data.newContactList.filter(
                        (c) => c.contact_id !== contactId
                    );
                } else if (rsp.data.code == 400) {
                    ElMessage.warning(rsp.data.message);
                    console.log(rsp.data.message);
                } else if (rsp.data.code == 500) {
                    ElMessage.error(rsp.data.message);
                    console.log(rsp.data.message);
                }
            } catch (error) {
                console.error(error);
            }
        };
        
        /**
         * 拉黑好友申请
         * @param {string} contactId - 联系人ID
         */
        const handleBlack = async (contactId) => {
            try {
                const req = {
                    owner_id: data.userInfo.uuid,
                    contact_id: contactId,
                };
                const rsp = await axios.post(
                    store.state.backendUrl + "/contact/blackApply",
                    req
                );
                if (rsp.data.code == 200) {
                    ElMessage.success(rsp.data.message);
                    console.log(rsp.data.message);
                    data.newContactList = data.newContactList.filter(
                        (c) => c.contact_id !== contactId
                    );
                } else if (rsp.data.code == 400) {
                    ElMessage.warning(rsp.data.message);
                    console.log(rsp.data.message);
                } else if (rsp.data.code == 500) {
                    ElMessage.error(rsp.data.message);
                    console.log(rsp.data.message);
                }
            } catch (error) {
                ElMessage.error(error);
                console.error(error);
            }
        };
        
        /**
         * 解除拉黑
         * @param {Object} user - 用户信息
         */
        const handleCancelBlack = async (user) => {
            try {
                const req = {
                    owner_id: data.userInfo.uuid,
                    contact_id: user.user_id,
                };
                const rsp = await axios.post(
                    store.state.backendUrl + "/contact/cancelBlackContact",
                    req
                );
                if (rsp.data.code == 200) {
                    ElMessage.success(rsp.data.message);
                    console.log(rsp.data.message);
                } else if (rsp.data.code == 400) {
                    ElMessage.warning(rsp.data.message);
                    console.log(rsp.data.message);
                } else if (rsp.data.code == 500) {
                    ElMessage.error(rsp.data.message);
                    console.log(rsp.data.message);
                }
            } catch (error) {
                ElMessage.error(error);
                console.error(error);
            }
        };

        return {
            ...toRefs(data),
            router,
            handleCreateGroup,
            showCreateGroupModal,
            closeCreateGroupModal,
            quitCreateGroupModal,
            showApplyContactModal,
            quitApplyContactModal,
            closeApplyContactModal,
            showNewContactModal,
            quitNewContactModal,
            handleShowUserList,
            handleHideUserList,
            handleShowMyGroupList,
            handleHideMyGroupList,
            handleShowMyJoinedGroupList,
            handleHideMyJoinedGroupList,
            handleToChatUser,
            handleToChatGroup,
            handleNewContactList,
            handleAgree,
            handleReject,
            handleCancelBlack,
            handleBlack,
            handleUploadSuccess,
            beforeFileUpload,
        };
    },
};
</script>

<style>
/* 头部样式 */
.contactlist-header {
    display: flex;
    flex-direction: row;
    margin-top: 10px;
    margin-bottom: 10px;
}

/* 搜索框样式 */
.contact-search-input {
    width: 185px;
    height: 30px;
    margin-left: 5px;
    margin-right: 5px;
}

/* 右侧操作按钮区域 */
.contactlist-header-right {
    width: 40px;
    height: 30px;
    display: flex;
    justify-content: center;
    align-items: center;
}

/* 创建群聊按钮 */
.create-group-btn {
    background-color: rgb(252, 210.9, 210.9);
    cursor: pointer;
    border: none;
    height: 100%;
    width: 30px;
    height: 30px;
    display: flex;
    justify-content: center;
    align-items: center;
    border-radius: 10px;
}

/* 创建群聊图标 */
.create-group-icon {
    width: 15px;
    height: 15px;
}

/* 菜单样式 */
.el-menu {
    background-color: rgb(252, 210.9, 210.9);
    width: 101%;
}

/* 菜单项样式 */
.el-menu-item {
    background-color: rgb(255, 255, 255);
    height: 45px;
}

/* 联系人标题 */
.contactlist-user-title {
    font-family: Arial, Helvetica, sans-serif;
}

/* 标题样式 */
h3 {
    font-family: Arial, Helvetica, sans-serif;
    color: rgb(69, 69, 68);
}

/* 模态框关闭按钮容器 */
.modal-quit-btn-container {
    height: 30%;
    width: 100%;
    display: flex;
    flex-direction: row-reverse;
}

/* 模态框关闭按钮 */
.modal-quit-btn {
    background-color: rgba(255, 255, 255, 0);
    color: rgb(229, 25, 25);
    padding: 15px;
    border: none;
    cursor: pointer;
    position: fixed;
    justify-content: center;
    align-items: center;
}

/* 模态框头部 */
.modal-header {
    height: 20%;
    width: 100%;
    display: flex;
    flex-direction: column;
    justify-content: center;
    align-items: center;
    /*background-color:aqua;*/
}

/* 模态框主体 */
.modal-body {
    height: 55%;
    width: 100%;
    display: flex;
    flex-direction: column;
    align-items: center;
    justify-content: center;
}

/* 创建群聊模态框主体 */
.creatgroup-modal-body {
    height: 75%;
    width: 100%;
    display: flex;
    flex-direction: column;
    align-items: center;
    justify-content: center;
}

/* 新的好友模态框主体 */
.newcontact-modal-body {
    height: 70%;
    width: 100%;
    display: flex;
    flex-direction: column;
    align-items: center;
    justify-content: center;
}

/* 新的好友模态框底部 */
.newcontact-modal-footer {
    height: 10%;
    width: 100%;
    display: flex;
    justify-content: center;
    align-items: center;
}

/* 模态框底部 */
.modal-footer {
    height: 25%;
    width: 100%;
    display: flex;
    justify-content: center;
    align-items: center;
}

/* 创建群聊模态框底部 */
.creategroup-modal-footer {
    height: 20%;
    width: 100%;
    display: flex;
    justify-content: center;
    align-items: center;
}

/* 模态框标题 */
.modal-header-title {
    height: 70%;
    width: 100%;
    display: flex;
    justify-content: center;
    align-items: center;
}

/* 群聊头像 */
.contactlist-avatar {
    width: 30px;
    height: 30px;
    margin-right: 20px;
}

/* 新的好友列表 */
.newcontact-list {
    width: 280px;
    display: flex;
    flex-direction: column;
    align-items: center;
    font-family: Arial, Helvetica, sans-serif;
}

/* 新的好友项 */
.newcontact-item {
    display: flex;
    flex-direction: row;
    justify-content: space-between;
    align-items: center;
    width: 100%;
    height: 40px;
}

/* 操作按钮 */
.action-btn {
    background-color: rgb(252, 210.9, 210.9);
    border: none;
    cursor: pointer;
    justify-content: center;
    align-items: center;
    font-family: Arial, Helvetica, sans-serif;
}

/* 联系人菜单项 */
.contactlist-user-menu-item {
    justify-content: center;
    align-items: center;
}

/* 联系人项 */
.contactlist-user-item {
    width: 221px;
    height: 45px;
    display: flex;
    align-items: center;
    color: rgba(43, 42, 42, 0.893);
}

/* 联系人头像 */
.contactlist-user-avatar {
    width: 30px;
    height: 30px;
    margin-left: 20px;
    margin-right: 20px;
}
</style>