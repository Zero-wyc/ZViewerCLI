export interface QrSession {
    qrcodeKey: string;
    qrUrl: string;
    qrDataUrl: string;
    createdAt: number;
}
export interface QrPollResult {
    status: number;
    message: string;
    cookie?: string;
    loggedIn: boolean;
}
/**
 * 生成 B站 网页端扫码登录二维码。
 */
export declare function generateQrCode(): Promise<QrSession>;
/**
 * 轮询 B站 二维码扫码状态。
 *
 * 返回状态码：
 * - 0: 未扫码
 * - 1: 已扫码未确认
 * - 2: 已确认登录（返回 cookie）
 * - 3: 二维码过期
 */
export declare function pollQrStatus(qrcodeKey: string): Promise<QrPollResult>;
/**
 * 使用给定 Cookie 验证 B站 登录状态。
 */
export declare function validateCookie(cookie: string): Promise<{
    valid: boolean;
    name?: string;
    mid?: number;
    vipStatus?: number;
}>;
//# sourceMappingURL=setup.d.ts.map