import { type LocalProxyResult } from "./server";
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
export declare function startCliAgent(options: CliAgentOptions): Promise<CliAgent>;
//# sourceMappingURL=agent.d.ts.map