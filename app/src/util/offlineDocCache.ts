// 远程空间离线文档缓存:getDoc 响应存 IndexedDB(http 源也可用,不依赖 Service Worker 的安全上下文),
// 断网时由 fetch 咽喉兜底读取;容量按 offlineCore 的 LRU 规则淘汰。

import {DocCacheEntry, evictAliases, evictOverLimit} from "./offlineCore";

const DB_NAME = "siyuan-offline-cache";
const STORE_DOCS = "docs";
const STORE_TREES = "trees";
const STORE_META = "meta";
const DOC_META_KEY = "entries";
const DOC_ALIAS_KEY = "aliases";

interface CachedDoc {
    json: string;
    at: number;
}

const treeKey = (notebook: string, path: string) => `${notebook}\u0000${path}`;

const openDb = (): Promise<IDBDatabase> => new Promise((resolve, reject) => {
    const request = indexedDB.open(DB_NAME, 1);
    request.onupgradeneeded = () => {
        const db = request.result;
        if (!db.objectStoreNames.contains(STORE_DOCS)) {
            db.createObjectStore(STORE_DOCS);
        }
        if (!db.objectStoreNames.contains(STORE_TREES)) {
            db.createObjectStore(STORE_TREES);
        }
        if (!db.objectStoreNames.contains(STORE_META)) {
            db.createObjectStore(STORE_META);
        }
    };
    request.onsuccess = () => resolve(request.result);
    request.onerror = () => reject(request.error);
});

const requestToPromise = <T>(request: IDBRequest<T>): Promise<T> => new Promise((resolve, reject) => {
    request.onsuccess = () => resolve(request.result);
    request.onerror = () => reject(request.error);
});

const readEntries = async (db: IDBDatabase): Promise<DocCacheEntry[]> => {
    const tx = db.transaction(STORE_META, "readonly");
    return (await requestToPromise(tx.objectStore(STORE_META).get(DOC_META_KEY))) || [];
};

const readAliases = async (db: IDBDatabase): Promise<Record<string, string>> => {
    const tx = db.transaction(STORE_META, "readonly");
    return (await requestToPromise(tx.objectStore(STORE_META).get(DOC_ALIAS_KEY))) || {};
};

// 写入文档快照并按 LRU 上限淘汰;根文档 ID 写入别名,块定位打开的文档离线时按根 ID 也能命中。
// 失败仅告警,不影响在线流程
export const putDocCache = async (docId: string, response: IWebSocketData) => {
    try {
        const json = JSON.stringify(response);
        const rootId = typeof response.data?.rootID === "string" ? response.data.rootID : "";
        const entry: DocCacheEntry = {
            docId,
            updated: response.data?.ial?.updated || "",
            bytes: json.length,
            at: Date.now(),
            title: typeof response.data?.ial?.title === "string" ? response.data.ial.title : "",
            ...(rootId && rootId !== docId ? {rootId} : {}),
        };
        const db = await openDb();
        const entries = (await readEntries(db)).filter(item => item.docId !== docId).concat(entry);
        const evicted = new Set(evictOverLimit(entries).map(item => item.docId));
        const kept = entries.filter(item => !evicted.has(item.docId));
        const aliases = evictAliases(await readAliases(db), evicted);
        if (rootId && rootId !== docId) {
            aliases[rootId] = docId;
        }
        const tx = db.transaction([STORE_DOCS, STORE_META], "readwrite");
        const docs = tx.objectStore(STORE_DOCS);
        docs.put({json, at: entry.at} as CachedDoc, docId);
        evicted.forEach(id => docs.delete(id));
        const meta = tx.objectStore(STORE_META);
        meta.put(kept, DOC_META_KEY);
        meta.put(aliases, DOC_ALIAS_KEY);
    } catch (error) {
        console.warn("offline doc cache put failed [" + error + "]");
    }
};

// 断网兜底读取;请求 id 未命中时按根文档 ID 别名回退,无缓存或解析失败返回 null
export const getDocCache = async (docId: string): Promise<IWebSocketData | null> => {
    try {
        const db = await openDb();
        const docsTx = db.transaction(STORE_DOCS, "readonly");
        let cached = await requestToPromise<CachedDoc | undefined>(docsTx.objectStore(STORE_DOCS).get(docId));
        if (!cached) {
            const alias = (await readAliases(db))[docId];
            if (alias && alias !== docId) {
                const aliasTx = db.transaction(STORE_DOCS, "readonly");
                cached = await requestToPromise<CachedDoc | undefined>(aliasTx.objectStore(STORE_DOCS).get(alias));
            }
        }
        if (!cached) {
            return null;
        }
        const response = JSON.parse(cached.json) as IWebSocketData;
        return typeof response.code === "number" ? response : null;
    } catch (error) {
        console.warn("offline doc cache get failed [" + error + "]");
        return null;
    }
};

// 预取前判缓存是否已存在(含别名回退),避免每次启动重复拉取
export const hasDocCache = async (docId: string): Promise<boolean> => {    try {
        const db = await openDb();
        const docsTx = db.transaction(STORE_DOCS, "readonly");
        if (await requestToPromise(docsTx.objectStore(STORE_DOCS).getKey(docId))) {
            return true;
        }
        const alias = (await readAliases(db))[docId];
        if (!alias || alias === docId) {
            return false;
        }
        const aliasTx = db.transaction(STORE_DOCS, "readonly");
        return Boolean(await requestToPromise(aliasTx.objectStore(STORE_DOCS).getKey(alias)));
    } catch (error) {
        console.warn("offline doc cache check failed [" + error + "]");
        return false;
    }
};

// 供缓存管理入口清空
export const clearDocCache = async () => {
    const db = await openDb();
    const tx = db.transaction([STORE_DOCS, STORE_TREES, STORE_META], "readwrite");
    tx.objectStore(STORE_DOCS).clear();
    tx.objectStore(STORE_TREES).clear();
    const meta = tx.objectStore(STORE_META);
    meta.clear();
};

// 缓存管理清单:按最后访问时间倒序
export const listDocCache = async (): Promise<DocCacheEntry[]> => {
    try {
        const db = await openDb();
        return (await readEntries(db)).sort((a, b) => b.at - a.at);
    } catch (error) {
        console.warn("offline doc cache list failed [" + error + "]");
        return [];
    }
};

// 删除单条缓存并清理指向它的别名
export const deleteDocCache = async (docId: string) => {
    try {
        const db = await openDb();
        const entries = await readEntries(db);
        const kept = entries.filter(item => item.docId !== docId);
        const aliases = evictAliases(await readAliases(db), new Set([docId]));
        const tx = db.transaction([STORE_DOCS, STORE_META], "readwrite");
        tx.objectStore(STORE_DOCS).delete(docId);
        const meta = tx.objectStore(STORE_META);
        meta.put(kept, DOC_META_KEY);
        meta.put(aliases, DOC_ALIAS_KEY);
    } catch (error) {
        console.warn("offline doc cache delete failed [" + error + "]");
    }
};

// 离线横幅展示的缓存文档数;失败返回 -1 由调用方省略计数
export const countDocCache = async (): Promise<number> => {
    try {
        const db = await openDb();
        return (await readEntries(db)).length;
    } catch (error) {
        console.warn("offline doc cache count failed [" + error + "]");
        return -1;
    }
};

// 目录展开响应(listDocsByPath)缓存:联网浏览过的目录离线才可展开,数据量小不做淘汰
export const putTreeCache = async (notebook: string, path: string, response: IWebSocketData) => {
    try {
        const db = await openDb();
        const tx = db.transaction(STORE_TREES, "readwrite");
        tx.objectStore(STORE_TREES).put(JSON.stringify(response), treeKey(notebook, path));
    } catch (error) {
        console.warn("offline tree cache put failed [" + error + "]");
    }
};

export const getTreeCache = async (notebook: string, path: string): Promise<IWebSocketData | null> => {
    try {
        const db = await openDb();
        const tx = db.transaction(STORE_TREES, "readonly");
        const cached = await requestToPromise<string | undefined>(tx.objectStore(STORE_TREES).get(treeKey(notebook, path)));
        if (!cached) {
            return null;
        }
        const response = JSON.parse(cached) as IWebSocketData;
        return typeof response.code === "number" ? response : null;
    } catch (error) {
        console.warn("offline tree cache get failed [" + error + "]");
        return null;
    }
};
