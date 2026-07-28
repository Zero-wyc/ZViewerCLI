import { homedir } from "node:os";
import { readFile, writeFile, mkdir, unlink } from "node:fs/promises";
import { existsSync } from "node:fs";
import { join } from "node:path";

export interface PersistedCliConfig {
  /** B站 Cookie 字符串 */
  cookie: string;
  /** 登录用户信息 */
  userInfo?: {
    name?: string;
    mid?: number;
    vipStatus?: number;
  };
  /** 保存时间 ISO 字符串 */
  savedAt: string;
}

const CONFIG_DIR = join(homedir(), ".zcontrol-cli");
const CONFIG_PATH = join(CONFIG_DIR, "config.json");

/**
 * 读取持久化的 CLI 配置。
 *
 * 配置文件位于用户主目录下的 .zcontrol-cli/config.json，
 * 包含 B站 Cookie 与用户信息，用于扫码一次后长期复用。
 */
export async function loadPersistedConfig(): Promise<PersistedCliConfig | null> {
  try {
    if (!existsSync(CONFIG_PATH)) return null;
    const raw = await readFile(CONFIG_PATH, "utf-8");
    const parsed = JSON.parse(raw) as PersistedCliConfig;
    if (typeof parsed.cookie !== "string" || !parsed.cookie) return null;
    return parsed;
  } catch (err) {
    console.error("[ZViewer CLI] 读取持久化配置失败:", err);
    return null;
  }
}

/**
 * 保存 CLI 配置到本地文件。
 */
export async function savePersistedConfig(
  config: PersistedCliConfig,
): Promise<void> {
  try {
    await mkdir(CONFIG_DIR, { recursive: true });
    await writeFile(CONFIG_PATH, JSON.stringify(config, null, 2), "utf-8");
  } catch (err) {
    console.error("[ZViewer CLI] 保存持久化配置失败:", err);
  }
}

/**
 * 清除本地持久化配置。
 */
export async function clearPersistedConfig(): Promise<void> {
  try {
    if (existsSync(CONFIG_PATH)) {
      await unlink(CONFIG_PATH);
    }
  } catch (err) {
    console.error("[ZViewer CLI] 清除持久化配置失败:", err);
  }
}
