// 远程空间离线文档缓存:getDoc 响应存 IndexedDB(http 源也可用,不依赖 Service Worker 的安全上下文),
// 断网时由 fetch 咽喉兜底读取;容量按 offlineCore 的 LRU 规则淘汰。

import {DocCacheEntry, evictAliases, evictOverLimit} from "./offlineCore";

const DB_NAME = "siyuan-offline-cache";
const STORE_DOCS = "docs";
const STORE_META = "meta";
const DOC_META_KEY = "entries";
const DOC_ALIAS_KEY = "aliases";

interface CachedDoc {
    json: string;
    at: number;
}

const openDb = (): Promise<IDBDatabase> => new Promise((resolve, reject) => {
    const request = indexedDB.open(DB_NAME, 1);
    request.onupgradeneeded = () => {
        const db = request.result;
        if (!db.objectStoreNames.contains(STORE_DOCS)) {
            db.createObjectStore(STORE_DOCS);
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

// 供缓存管理入口清空
export const clearDocCache = async () => {
    const db = await openDb();
    const tx = db.transaction([STORE_DOCS, STORE_META], "readwrite");
    tx.objectStore(STORE_DOCS).clear();
    const meta = tx.objectStore(STORE_META);
    meta.clear();
};
