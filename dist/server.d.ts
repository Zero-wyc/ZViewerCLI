import type { Server } from "http";
export interface LocalProxyOptions {
    port: number;
    /** 初始后端地址（setup 模式下可被页面覆盖） */
    serverUrl?: string;
    /** 初始房间 ID（setup 模式下可被页面覆盖） */
    roomId?: string;
    /** 初始 B站 Cookie（setup 模式下可被页面覆盖） */
    cookie?: string;
}
export interface LocalProxyConfig {
    serverUrl: string;
    roomId: string;
    cookie: string;
}
export interface LocalProxyState {
    config: LocalProxyConfig | null;
    connected: boolean;
    connecting: boolean;
    lastError: string | null;
    userInfo: {
        name?: string;
        mid?: number;
        vipStatus?: number;
    } | null;
    /** Cookie 是否校验通过；null 表示尚未校验 */
    cookieValid: boolean | null;
}
export interface LocalProxyResult {
    server: Server;
    url: string;
    state: LocalProxyState;
    connect: () => Promise<void>;
}
export type OnConnectCallback = (config: LocalProxyConfig) => Promise<void>;
export declare function createLocalProxy(options: LocalProxyOptions, onConnect?: OnConnectCallback): Promise<LocalProxyResult>;
//# sourceMappingURL=server.d.ts.map