// 远程空间离线文档缓存:getDoc 响应存 IndexedDB(http 源也可用,不依赖 Service Worker 的安全上下文),
// 断网时由 fetch 咽喉兜底读取;容量按 offlineCore 的 LRU 规则淘汰。

import {DocCacheEntry, evictOverLimit} from "./offlineCore";

const DB_NAME = "siyuan-offline-cache";
const STORE_DOCS = "docs";
const STORE_META = "meta";
const DOC_META_KEY = "entries";

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

// 写入文档快照并按 LRU 上限淘汰;失败仅告警,不影响在线流程
export const putDocCache = async (docId: string, response: IWebSocketData) => {
    try {
        const json = JSON.stringify(response);
        const entry: DocCacheEntry = {
            docId,
            updated: response.data?.ial?.updated || "",
            bytes: json.length,
            at: Date.now(),
        };
        const db = await openDb();
        const entries = (await readEntries(db)).filter(item => item.docId !== docId).concat(entry);
        const evicted = new Set(evictOverLimit(entries).map(item => item.docId));
        const kept = entries.filter(item => !evicted.has(item.docId));
        const tx = db.transaction([STORE_DOCS, STORE_META], "readwrite");
        const docs = tx.objectStore(STORE_DOCS);
        docs.put({json, at: entry.at} as CachedDoc, docId);
        evicted.forEach(id => docs.delete(id));
        tx.objectStore(STORE_META).put(kept, DOC_META_KEY);
    } catch (error) {
        console.warn("offline doc cache put failed [" + error + "]");
    }
};

// 断网兜底读取;无缓存或解析失败返回 null
export const getDocCache = async (docId: string): Promise<IWebSocketData | null> => {
    try {
        const db = await openDb();
        const tx = db.transaction(STORE_DOCS, "readonly");
        const cached = await requestToPromise<CachedDoc | undefined>(tx.objectStore(STORE_DOCS).get(docId));
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
    tx.objectStore(STORE_META).clear();
};
