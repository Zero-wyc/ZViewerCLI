"use strict";
Object.defineProperty(exports, "__esModule", { value: true });
exports.startCliAgent = startCliAgent;
const server_1 = require("./server");
const socket_1 = require("./socket");
const config_1 = require("./config");
const setup_1 = require("./setup");
async function startCliAgent(options) {
    const { port, serverUrl, roomId, connectImmediately } = options;
    let { cookie } = options;
    // 加载持久化配置：命令行未提供 cookie 时尝试复用本地保存的 cookie
    let persisted = await (0, config_1.loadPersistedConfig)();
    if (!cookie && persisted?.cookie) {
        console.log("[ZViewer CLI] 检测到已保存的 Cookie，正在校验有效性...");
        const validation = await (0, setup_1.validateCookie)(persisted.cookie);
        if (validation.valid) {
            cookie = persisted.cookie;
            console.log(`[ZViewer CLI] Cookie 有效，已登录用户: ${validation.name || "未知"}`);
        }
        else {
            console.warn("[ZViewer CLI] 已保存的 Cookie 已过期，已清除");
            await (0, config_1.clearPersistedConfig)();
            persisted = null;
        }
    }
    let socket = null;
    let proxyResult = null;
    // 连接回调：由网页表单或命令行参数触发
    const doConnect = async (config) => {
        if (socket) {
            socket.disconnect();
            socket = null;
        }
        socket = (0, socket_1.connectRoomSocket)({
            serverUrl: config.serverUrl,
            roomId: config.roomId,
            proxyUrl: `http://127.0.0.1:${port}`,
        });
        console.log(`[ZViewer CLI] 已连接房间: ${config.roomId}`);
    };
    // 1. 启动本地 HTTP 代理（同时提供 setup 网页）
    proxyResult = await (0, server_1.createLocalProxy)({
        port,
        serverUrl,
        roomId,
        cookie,
    }, doConnect);
    // 2. 如果命令行提供了完整参数，立即连接
    if (connectImmediately && proxyResult.state.config) {
        await proxyResult.connect();
        console.log(`[ZViewer CLI] 本地代理已启动: ${proxyResult.url}`);
        console.log(`[ZViewer CLI] 已连接房间: ${proxyResult.state.config.roomId}`);
    }
    else {
        console.log(`[ZViewer CLI] 本地代理已启动: ${proxyResult.url}`);
        console.log(`[ZViewer CLI] 请在浏览器中打开 ${proxyResult.url} 完成配置`);
    }
    return {
        proxyUrl: proxyResult.url,
        state: proxyResult.state,
        connect: proxyResult.connect,
        close: async () => {
            if (socket) {
                socket.disconnect();
                socket = null;
            }
            await new Promise((resolve, reject) => {
                proxyResult?.server.close((err) => {
                    if (err)
                        reject(err);
                    else
                        resolve();
                });
            });
        },
    };
}
//# sourceMappingURL=agent.js.map