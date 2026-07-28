import type { Server } from "http";
import type { Socket } from "socket.io-client";
import {
  createLocalProxy,
  type LocalProxyConfig,
  type LocalProxyResult,
} from "./server";
import { connectRoomSocket } from "./socket";
import { loadPersistedConfig, clearPersistedConfig } from "./config";
import { validateCookie } from "./setup";

export interface CliAgentOptions {
  /** 本地代理端口 */
  port: number;
  /** 后端地址（setup 模式下可后续填写） */
  serverUrl?: string;
  /** 房间 ID（setup 模式下可后续填写） */
  roomId?: string;
  /** B站 Cookie（setup 模式下可扫码获取） */
  cookie?: string;
  /** 是否立即连接（connect 命令为 true；setup 命令为 false） */
  connectImmediately?: boolean;
}

export interface CliAgent {
  proxyUrl: string;
  /** 本地代理状态（setup 模式下供页面读取） */
  state: LocalProxyResult["state"];
  /** 手动触发连接（setup 模式下由网页调用） */
  connect: () => Promise<void>;
  close: () => Promise<void>;
}

export async function startCliAgent(
  options: CliAgentOptions,
): Promise<CliAgent> {
  const { port, serverUrl, roomId, connectImmediately } = options;
  let { cookie } = options;

  // 加载持久化配置：命令行未提供 cookie 时尝试复用本地保存的 cookie
  let persisted = await loadPersistedConfig();
  if (!cookie && persisted?.cookie) {
    console.log("[ZViewer CLI] 检测到已保存的 Cookie，正在校验有效性...");
    const validation = await validateCookie(persisted.cookie);
    if (validation.valid) {
      cookie = persisted.cookie;
      console.log(
        `[ZViewer CLI] Cookie 有效，已登录用户: ${validation.name || "未知"}`,
      );
    } else {
      console.warn("[ZViewer CLI] 已保存的 Cookie 已过期，已清除");
      await clearPersistedConfig();
      persisted = null;
    }
  }

  let socket: Socket | null = null;
  let proxyResult: LocalProxyResult | null = null;

  // 连接回调：由网页表单或命令行参数触发
  const doConnect = async (config: LocalProxyConfig) => {
    if (socket) {
      socket.disconnect();
      socket = null;
    }
    socket = connectRoomSocket({
      serverUrl: config.serverUrl,
      roomId: config.roomId,
      proxyUrl: `http://127.0.0.1:${port}`,
    });

    console.log(`[ZViewer CLI] 已连接房间: ${config.roomId}`);
  };

  // 1. 启动本地 HTTP 代理（同时提供 setup 网页）
  proxyResult = await createLocalProxy(
    {
      port,
      serverUrl,
      roomId,
      cookie,
    },
    doConnect,
  );

  // 2. 如果命令行提供了完整参数，立即连接
  if (connectImmediately && proxyResult.state.config) {
    await proxyResult.connect();
    console.log(`[ZViewer CLI] 本地代理已启动: ${proxyResult.url}`);
    console.log(
      `[ZViewer CLI] 已连接房间: ${proxyResult.state.config.roomId}`,
    );
  } else {
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
      await new Promise<void>((resolve, reject) => {
        proxyResult?.server.close((err) => {
          if (err) reject(err);
          else resolve();
        });
      });
    },
  };
}
