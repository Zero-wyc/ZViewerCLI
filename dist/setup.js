"use strict";
var __importDefault = (this && this.__importDefault) || function (mod) {
    return (mod && mod.__esModule) ? mod : { "default": mod };
};
Object.defineProperty(exports, "__esModule", { value: true });
exports.generateQrCode = generateQrCode;
exports.pollQrStatus = pollQrStatus;
exports.validateCookie = validateCookie;
const node_fetch_1 = __importDefault(require("node-fetch"));
const qrcode_1 = __importDefault(require("qrcode"));
const DEFAULT_USER_AGENT = 'Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36';
/**
 * 生成 B站 网页端扫码登录二维码。
 */
async function generateQrCode() {
    const res = await (0, node_fetch_1.default)('https://passport.bilibili.com/x/passport-login/web/qrcode/generate', {
        headers: {
            'User-Agent': DEFAULT_USER_AGENT,
            Referer: 'https://www.bilibili.com',
        },
    });
    if (!res.ok) {
        throw new Error(`生成二维码失败: ${res.status}`);
    }
    const json = (await res.json());
    if (json.code !== 0 || !json.data?.qrcode_key || !json.data?.url) {
        throw new Error(json.message || '生成二维码失败');
    }
    const qrDataUrl = await qrcode_1.default.toDataURL(json.data.url);
    return {
        qrcodeKey: json.data.qrcode_key,
        qrUrl: json.data.url,
        qrDataUrl,
        createdAt: Date.now(),
    };
}
function parseSetCookieHeader(headers) {
    const getSetCookies = headers
        .getSetCookies;
    let values = [];
    if (typeof getSetCookies === 'function') {
        values = getSetCookies.call(headers);
    }
    else {
        const single = headers.get('set-cookie');
        if (single) {
            values = single.split(',').map((s) => s.trim());
        }
    }
    return values
        .map((c) => c.split(';')[0].trim())
        .filter((c) => c.includes('='))
        .join('; ');
}
function parseSetCookieToMap(headers) {
    const getSetCookies = headers
        .getSetCookies;
    let values = [];
    if (typeof getSetCookies === 'function') {
        values = getSetCookies.call(headers);
    }
    else {
        const single = headers.get('set-cookie');
        if (single) {
            values = single.split(',').map((s) => s.trim());
        }
    }
    const map = new Map();
    for (const cookie of values) {
        const [nameValue] = cookie.split(';');
        const trimmed = nameValue.trim();
        const eq = trimmed.indexOf('=');
        if (eq > 0) {
            map.set(trimmed.slice(0, eq), trimmed.slice(eq + 1));
        }
    }
    return map;
}
function cookieMapToString(map) {
    const parts = [];
    map.forEach((value, name) => parts.push(`${name}=${value}`));
    return parts.join('; ');
}
async function fetchCookiesFromSsoUrl(ssoUrl) {
    try {
        const cookieMap = new Map();
        let currentUrl = ssoUrl;
        const seenUrls = new Set();
        const maxRedirects = 10;
        for (let i = 0; i <= maxRedirects; i++) {
            if (seenUrls.has(currentUrl))
                break;
            seenUrls.add(currentUrl);
            const res = await (0, node_fetch_1.default)(currentUrl, {
                method: 'GET',
                redirect: 'manual',
                headers: {
                    'User-Agent': DEFAULT_USER_AGENT,
                    Referer: 'https://www.bilibili.com',
                    ...(cookieMap.size > 0
                        ? { Cookie: cookieMapToString(cookieMap) }
                        : {}),
                },
            });
            const setCookies = parseSetCookieToMap(res.headers);
            for (const [name, value] of setCookies) {
                cookieMap.set(name, value);
            }
            const location = res.headers.get('location');
            if (!location || res.status < 300 || res.status >= 400)
                break;
            currentUrl = new URL(location, currentUrl).toString();
        }
        const required = ['SESSDATA', 'bili_jct', 'DedeUserID'];
        const missing = required.filter((name) => !cookieMap.has(name));
        if (missing.length > 0) {
            console.warn('[cli] sso cookie missing keys:', missing.join(', '));
            return null;
        }
        return cookieMapToString(cookieMap);
    }
    catch (err) {
        console.error('[cli] fetch sso url error:', err);
        return null;
    }
}
/**
 * 轮询 B站 二维码扫码状态。
 *
 * 返回状态码：
 * - 0: 未扫码
 * - 1: 已扫码未确认
 * - 2: 已确认登录（返回 cookie）
 * - 3: 二维码过期
 */
async function pollQrStatus(qrcodeKey) {
    const res = await (0, node_fetch_1.default)(`https://passport.bilibili.com/x/passport-login/web/qrcode/poll?qrcode_key=${qrcodeKey}`, {
        headers: {
            'User-Agent': DEFAULT_USER_AGENT,
            Referer: 'https://www.bilibili.com',
        },
    });
    if (!res.ok) {
        throw new Error(`轮询二维码状态失败: ${res.status}`);
    }
    const json = (await res.json());
    const pollData = json.data;
    const innerCode = pollData?.code;
    let status = pollData?.status ?? -1;
    if (innerCode === 0 && pollData?.url)
        status = 2;
    else if (innerCode === 86101)
        status = 0;
    else if (innerCode === 86090)
        status = 1;
    else if (innerCode === 86038)
        status = 3;
    if (status === 2) {
        const ssoUrl = pollData?.url;
        let cookie = null;
        if (ssoUrl) {
            cookie = await fetchCookiesFromSsoUrl(ssoUrl);
        }
        if (!cookie) {
            cookie = parseSetCookieHeader(res.headers);
        }
        if (cookie) {
            return {
                status,
                message: pollData?.message || '登录成功',
                cookie,
                loggedIn: true,
            };
        }
        return {
            status,
            message: '登录确认成功，但未能获取 Cookie',
            loggedIn: false,
        };
    }
    return {
        status,
        message: pollData?.message || '',
        loggedIn: false,
    };
}
/**
 * 使用给定 Cookie 验证 B站 登录状态。
 */
async function validateCookie(cookie) {
    try {
        const res = await (0, node_fetch_1.default)('https://api.bilibili.com/x/web-interface/nav', {
            headers: {
                'User-Agent': DEFAULT_USER_AGENT,
                Referer: 'https://www.bilibili.com',
                Cookie: cookie,
            },
        });
        if (!res.ok) {
            return { valid: false };
        }
        const json = (await res.json());
        if (!json.data?.isLogin) {
            return { valid: false };
        }
        return {
            valid: true,
            name: json.data.uname,
            mid: json.data.mid,
            vipStatus: json.data.vipStatus,
        };
    }
    catch (err) {
        console.error('[cli] validate cookie error:', err);
        return { valid: false };
    }
}
//# sourceMappingURL=setup.js.map