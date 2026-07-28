"use strict";
Object.defineProperty(exports, "__esModule", { value: true });
exports.loadPersistedConfig = loadPersistedConfig;
exports.savePersistedConfig = savePersistedConfig;
exports.clearPersistedConfig = clearPersistedConfig;
const node_os_1 = require("node:os");
const promises_1 = require("node:fs/promises");
const node_fs_1 = require("node:fs");
const node_path_1 = require("node:path");
const CONFIG_DIR = (0, node_path_1.join)((0, node_os_1.homedir)(), ".zcontrol-cli");
const CONFIG_PATH = (0, node_path_1.join)(CONFIG_DIR, "config.json");
/**
 * 读取持久化的 CLI 配置。
 *
 * 配置文件位于用户主目录下的 .zcontrol-cli/config.json，
 * 包含 B站 Cookie 与用户信息，用于扫码一次后长期复用。
 */
async function loadPersistedConfig() {
    try {
        if (!(0, node_fs_1.existsSync)(CONFIG_PATH))
            return null;
        const raw = await (0, promises_1.readFile)(CONFIG_PATH, "utf-8");
        const parsed = JSON.parse(raw);
        if (typeof parsed.cookie !== "string" || !parsed.cookie)
            return null;
        return parsed;
    }
    catch (err) {
        console.error("[ZViewer CLI] 读取持久化配置失败:", err);
        return null;
    }
}
/**
 * 保存 CLI 配置到本地文件。
 */
async function savePersistedConfig(config) {
    try {
        await (0, promises_1.mkdir)(CONFIG_DIR, { recursive: true });
        await (0, promises_1.writeFile)(CONFIG_PATH, JSON.stringify(config, null, 2), "utf-8");
    }
    catch (err) {
        console.error("[ZViewer CLI] 保存持久化配置失败:", err);
    }
}
/**
 * 清除本地持久化配置。
 */
async function clearPersistedConfig() {
    try {
        if ((0, node_fs_1.existsSync)(CONFIG_PATH)) {
            await (0, promises_1.unlink)(CONFIG_PATH);
        }
    }
    catch (err) {
        console.error("[ZViewer CLI] 清除持久化配置失败:", err);
    }
}
//# sourceMappingURL=config.js.map