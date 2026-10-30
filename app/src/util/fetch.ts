import {fetchWithAppId} from "./fetchWithAppId";
import {Constants} from "../constants";
/// #if !BROWSER
import {ipcRenderer} from "electron";
/// #endif
import {processMessage} from "./processMessage";
import {kernelError} from "./kernelFault";
import {withFetchTimeout} from "./fetchTimeout";
import type {FetchGet, FetchPost, FetchSyncPost} from "../types/api";
import {trackSettingRequest} from "../config/setting/pending";
import {trackEditorSaveRequest} from "../protyle/util/editorSave";
import {showMessage} from "../dialog/message";
import {deleteDocCaches, getDocCache, getSyntheticTreeListing, getTreeCache, putDocCache, putTreeCache} from "./offlineDocCache";
import {isOffline} from "./offlineState";
import {isRemoteKernel} from "./hostCapabilities";
import {isBrowser} from "./functions";

const reportTransactionFailure = (url: string, error: Error) => {
    if (url !== "/api/transactions" || error?.name === "AbortError") {
        return;
    }
    // 保留当前编辑内容并提示核对保存结果，避免重发已在内核执行的事务。
    showMessage(window.siyuan.languages.transactionSaveUnconfirmed, 0, "error", "transaction-save-unconfirmed");
    void kernelError();
};

// 离线只读兜底:getDoc 直读缓存快照,getBlockInfo 由快照合成(打开文档的前置调用)。
// 返回 null 表示未接管;接管后 code=0 为命中,否则为快速失败(getDoc 缺缓存时已就地提示)
const serveOfflineDocRequest = (url: string, data: any): Promise<IWebSocketData> | null => {
    if (!isOffline() || !data) {
        return null;
    }
    if (url === "/api/filetree/getDoc") {
        return getDocCache(data.id).then(cached => {
            if (cached) {
                return cached;
            }
            showMessage(window.siyuan.languages.offlineDocNotCached);
            return {data: null, msg: "offline", code: 400} as IWebSocketData;
        });
    }
    if (url === "/api/block/getBlockInfo") {
        return getDocCache(data.id).then(cached => {
            if (!cached) {
                return {data: null, msg: "offline", code: 400} as IWebSocketData;
            }
            const ial = cached.data?.docInfo?.ial || cached.data?.ial || {};
            return {
                code: 0,
                msg: "",
                data: {
                    id: data.id,
                    rootID: cached.data?.rootID || data.id,
                    rootTitle: ial.title || "",
                    rootTitleEmpty: !ial.title,
                    rootIcon: ial.icon || "",
                    box: cached.data?.box || "",
                    path: cached.data?.path || "",
                },
            } as IWebSocketData;
        });
    }
    if (url === "/api/block/getDocInfo") {
        // openFileById 以此响应决定页签标题,离线缺它则页签回退「未命名文档」
        return getDocCache(data.id).then(cached => {
            const docInfo = cached?.data?.docInfo;
            if (!docInfo) {
                return {data: null, msg: "offline", code: 400} as IWebSocketData;
            }
            return {code: 0, msg: "", data: docInfo} as IWebSocketData;
        });
    }
    if (url === "/api/filetree/listDocsByPath" && typeof data.notebook === "string") {
        const dir = data.path || "/";
        return Promise.all([getTreeCache(data.notebook, dir), getSyntheticTreeListing(data.notebook, dir)])
            .then(([cached, synthetic]) => {
                if (cached && synthetic?.data?.files?.length) {
                    // 合并:真实列表 + 缓存条目合成的子文档,按 id 去重
                    const seen = new Set(cached.data?.files?.map((file: { id?: string }) => file.id) || []);
                    const extra = synthetic.data.files.filter((file: { id?: string }) => !seen.has(file.id));
                    return {...cached, data: {...cached.data, files: [...(cached.data?.files || []), ...extra]}};
                }
                if (cached) {
                    return cached;
                }
                if (synthetic) {
                    return synthetic;
                }
                showMessage(window.siyuan.languages.offlineTreeNotCached);
                return {data: null, msg: "offline", code: 400} as IWebSocketData;
            });
    }
    return null;
};

export const fetchPost = ((
    url: string,
    data?: any,
    cb?: (response: IWebSocketData) => void,
    headers?: Record<string, string>,
    failCallback?: (response: IWebSocketData) => void,
    signal?: AbortSignal,
    timeout = url === "/api/system/exit" || url === "/api/system/setWorkspaceDir" ||
        (url === "/api/system/setUILayout" && data?.errorExit) ? 30000 : 0) => {
    // 离线只读期间阻止全部写事务,避免编辑内容只落在本地 DOM
    if (isOffline() && url === "/api/transactions") {
        showMessage(window.siyuan.languages.offlineTransactionBlocked, 0, "error");
        return;
    }
    // 离线只读期间 getDoc/getBlockInfo 直接由缓存接管,避免请求挂起导致无限 loading
    const offlineServed = serveOfflineDocRequest(url, data);
    if (offlineServed) {
        offlineServed.then(response => {
            if (response.code === 0) {
                cb?.(response);
                return;
            }
            failCallback?.({
                data: null,
                msg: response.msg,
                code: response.code,
            });
        });
        return;
    }
    const init: RequestInit = {
        method: "POST",
    };
    if (data) {
        if (["/api/search/searchRefBlock", "/api/graph/getGraph", "/api/graph/getLocalGraph"].includes(url)) {
            window.siyuan.reqIds[url] = Date.now();
            if (data.type === "local" && url === "/api/graph/getLocalGraph") {
                // 当打开文档A的关系图、关系图、文档A后刷新，由于防止请求重复处理，文档A关系图无法渲染。
            } else {
                data.reqId = window.siyuan.reqIds[url];
            }
        }
        // 并发导出后端接受顺序不一致
        if (url === "/api/transactions") {
            data.reqId = Date.now();
        }
        if (data instanceof FormData) {
            init.body = data;
        } else {
            init.body = JSON.stringify(data);
        }
    }
    if (headers) {
        init.headers = headers;
    }
    if (signal) {
        init.signal = signal;
    }
    let isGetFile202 = false;
    return trackEditorSaveRequest(url, data, trackSettingRequest(url, withFetchTimeout((requestSignal) => fetchWithAppId(url, {...init, signal: requestSignal}).then((response): Promise<IWebSocketData | string> | IWebSocketData => {
        switch (response.status) {
            case 403:
            case 404:
                return {
                    data: null,
                    msg: response.statusText,
                    code: -response.status,
                };
            case 401:
                // 401 可能来自插件经内核代理访问的第三方服务(与会话失效无关),盲目整页重载会造成刷新循环;
                // 真正的会话失效由 WS logoutAuth 推送接管跳转认证页,这里只提示并放行给调用方
                console.error("api 401 [" + url + "]");
                showMessage(window.siyuan.languages.apiUnauthorizedTip, 0, "error", "api-unauthorized");
                return {
                    data: null,
                    msg: response.statusText,
                    code: -response.status,
                };
            default:
                // /api/file/getFile 接口返回202时表示文件没有正常读取
                if (response.status === 202 && url === "/api/file/getFile") {
                    isGetFile202 = true;
                }
                if (response.headers.get("content-type")?.indexOf("application/json") > -1) {
                    return response.json();
                } else {
                    return response.text();
                }
        }
    }), signal, timeout), url === "/api/storage/setLocalStorageVal" && data?.key === Constants.LOCAL_SLASH_USAGE)).catch(error => {
        reportTransactionFailure(url, error);
        throw error;
    }).then((response: IWebSocketData) => {
        if (failCallback && url === "/api/file/getFile" && isGetFile202) {
            failCallback(response);
            return;
        }
        // 在线打开的文档与浏览过的目录写入离线缓存(远程模式或浏览器访问,内核均不在本进程),断网兜底用
        if ((isRemoteKernel() || isBrowser()) && response && typeof response === "object" && response.code === 0 && !isOffline()) {
            if (url === "/api/filetree/getDoc" && data?.id) {
                void putDocCache(data.id, response);
            } else if (url === "/api/filetree/listDocsByPath" && typeof data?.notebook === "string") {
                void putTreeCache(data.notebook, data.path || "/", response);
            } else if (url === "/api/filetree/removeDoc" && data?.path) {
                // 文档删除联动清理离线缓存快照,避免管理窗与离线打开残留已删文档
                void deleteDocCaches([String(data.path).split("/").pop().replace(/\.sy$/, "")]);
            } else if (url === "/api/filetree/removeDocs" && Array.isArray(data?.paths)) {
                void deleteDocCaches(data.paths.map((p: string) => String(p).split("/").pop().replace(/\.sy$/, "")).filter(Boolean));
            } else if (url === "/api/filetree/removeDocByID" && data?.id) {
                void deleteDocCaches([String(data.id)]);
            }
        }
        if (typeof response === "string") {
            if (cb) {
                cb(response);
            }
            return;
        }
        if (["/api/search/searchRefBlock", "/api/graph/getGraph", "/api/graph/getLocalGraph"].includes(url)) {
            if (response.data.reqId && window.siyuan.reqIds[url] && window.siyuan.reqIds[url] > response.data.reqId) {
                return;
            }
        }
        if (typeof response === "object" && typeof response.msg === "string" && typeof response.code === "number") {
            if (processMessage(response) && cb) {
                cb(response);
            }
        } else if (cb) {
            cb(response);
        }
    }).catch((e) => {
        if (e?.name === "AbortError") {
            return;
        }
        // 网络不可达时 getDoc 从离线缓存兜底;isOffline() 有 5 秒宽限,用 navigator.onLine 即时补盲区,
        // 在线状态下内核报错不吞进旧缓存
        if (url === "/api/filetree/getDoc" && data?.id && (isOffline() || !navigator.onLine)) {
            void getDocCache(data.id).then(cached => {
                if (cached && cb) {
                    cb(cached);
                    // 快照非完整(动态加载中途缓存)时提示可读范围,避免误以为已到文末
                    if (cached.data?.eof === false) {
                        showMessage(window.siyuan.languages.offlineLongDocTruncated, 6000, "info");
                    }
                    return;
                }
                if (isOffline()) {
                    showMessage(window.siyuan.languages.offlineDocNotCached);
                }
                failCallback?.({
                    data: null,
                    msg: e.message,
                    code: 400,
                });
            });
            return;
        }
        if (failCallback) {
            failCallback({
                data: null,
                msg: e.message,
                code: 400,
            });
            return;
        }
        console.warn("fetch post failed [" + e + "], url [" + url + "]");
        /// #if !BROWSER
        if (url === "/api/system/setWorkspaceDir" || (
            ["/api/system/setUILayout"].includes(url) && data.errorExit // 内核中断，点关闭处理
        )) {
            ipcRenderer.send(Constants.SIYUAN_QUIT, location.port);
        }
        /// #endif
    });
}) as FetchPost<IWebSocketData>;

export const fetchSyncPost = (async (url: string, data?: any, headers?: Record<string, string>, process = true,
                                    signal?: AbortSignal) => {
    const offlineServed = serveOfflineDocRequest(url, data);
    if (offlineServed) {
        return offlineServed.then(response => {
            if (process && response.code === 0) {
                processMessage(response);
            }
            return response;
        });
    }
    const init: RequestInit = {
        method: "POST",
        signal,
    };
    if (headers) {
        init.headers = headers;
    }
    if (data) {
        if (data instanceof FormData) {
            init.body = data;
        } else {
            init.body = JSON.stringify(data);
        }
    }
    return trackSettingRequest(url, trackEditorSaveRequest(url, data,
        fetchWithAppId(url, init).then(res => res.json() as Promise<IWebSocketData>)).catch(error => {
        reportTransactionFailure(url, error);
        throw error;
    }).then(res2 => {
        // 在线打开的文档与浏览过的目录写入离线缓存(fetchSyncPost 通道,与 fetchPost 咽喉对称)
        if ((isRemoteKernel() || isBrowser()) && res2 && typeof res2 === "object" && res2.code === 0 && !isOffline()) {
            if (url === "/api/filetree/getDoc" && data?.id) {
                void putDocCache(data.id, res2);
            } else if (url === "/api/filetree/listDocsByPath" && typeof data?.notebook === "string") {
                void putTreeCache(data.notebook, data.path || "/", res2);
            }
        }
        if (process) {
            processMessage(res2);
        }
        return res2;
    }), url === "/api/storage/setLocalStorageVal" && data?.key === Constants.LOCAL_SLASH_USAGE);
}) as FetchSyncPost<IWebSocketData>;

export const fetchGet = ((url: string, cb: (response: IWebSocketData | IObject | string) => void) => {
    fetchWithAppId(url, {cache: "no-store"}).then((response) => {
        if (response.headers.get("content-type")?.indexOf("application/json") > -1) {
            return response.json();
        } else {
            return response.text();
        }
    }).then((response) => {
        cb(response);
    });
}) as FetchGet<IWebSocketData | IObject | string>;
