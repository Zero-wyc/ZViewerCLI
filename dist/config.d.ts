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
/**
 * 读取持久化的 CLI 配置。
 *
 * 配置文件位于用户主目录下的 .zcontrol-cli/config.json，
 * 包含 B站 Cookie 与用户信息，用于扫码一次后长期复用。
 */
export declare function loadPersistedConfig(): Promise<PersistedCliConfig | null>;
/**
 * 保存 CLI 配置到本地文件。
 */
export declare function savePersistedConfig(config: PersistedCliConfig): Promise<void>;
/**
 * 清除本地持久化配置。
 */
export declare function clearPersistedConfig(): Promise<void>;
//# sourceMappingURL=config.d.ts.map