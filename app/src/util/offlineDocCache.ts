// 远程空间离线文档缓存:getDoc 响应存 IndexedDB(http 源也可用,不依赖 Service Worker 的安全上下文),
// 断网时由 fetch 咽喉兜底读取;容量按 offlineCore 的 LRU 规则淘汰。

import {DOC_CACHE_LIMIT_COUNT, DocCacheEntry, evictAliases, evictOverLimit} from "./offlineCore";

// 离线缓存篇数上限复用「最近的文档最大数量」设置(同一群最近浏览的文档),未初始化时回落默认值
const cacheLimitCount = () =>
    (typeof window !== "undefined" && window.siyuan?.config?.fileTree?.recentDocsMaxListCount) || DOC_CACHE_LIMIT_COUNT;

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
            updated: response.data?.docInfo?.ial?.updated || response.data?.ial?.updated || "",
            bytes: json.length,
            at: Date.now(),
            title: typeof response.data?.docInfo?.ial?.title === "string" ? response.data.docInfo.ial.title : "",
            ...(rootId && rootId !== docId ? {rootId} : {}),
        };
        const db = await openDb();
        const entries = (await readEntries(db)).filter(item => item.docId !== docId).concat(entry);
        const evicted = new Set(evictOverLimit(entries, undefined, cacheLimitCount()).map(item => item.docId));
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

// 文件树灰显用:已缓存目录树键(notebook\u0000dir)集合,判断目录离线可否展开
export const getCachedTreeKeys = async (): Promise<Set<string>> => {
    try {
        const db = await openDb();
        const tx = db.transaction(STORE_TREES, "readonly");
        const keys = await requestToPromise(tx.objectStore(STORE_TREES).getAllKeys());
        return new Set(keys.map(key => String(key)));
    } catch (error) {
        console.warn("offline tree cache key list failed [" + error + "]");
        return new Set();
    }
};

// 文件树灰显用:已缓存文档 ID 与别名目标 ID 的合集
export const getCachedDocIds = async (): Promise<Set<string>> => {
    try {
        const db = await openDb();
        const tx = db.transaction([STORE_DOCS, STORE_META], "readonly");
        const keys = await requestToPromise(tx.objectStore(STORE_DOCS).getAllKeys());
        const result = new Set<string>(keys.map(key => String(key)));
        const aliases = (await requestToPromise(tx.objectStore(STORE_META).get(DOC_ALIAS_KEY))) || {};
        Object.keys(aliases).forEach(alias => result.add(aliases[alias]));
        return result;
    } catch (error) {
        console.warn("offline doc cache id list failed [" + error + "]");
        return new Set();
    }
};

// 缓存管理清单:按最后访问时间倒序;缺标题的条目(旧版写入或元数据回填未跑)直接从缓存快照派生,离线也能显示标题
export const listDocCache = async (): Promise<DocCacheEntry[]> => {
    try {
        const db = await openDb();
        const allEntries = (await readEntries(db)).sort((a, b) => b.at - a.at);
        const docsTx = db.transaction(STORE_DOCS, "readonly");
        const keys = await requestToPromise(docsTx.objectStore(STORE_DOCS).getAllKeys());
        const keySet = new Set(keys.map(key => String(key)));
        // 联表校验:无快照的孤儿条目(异常路径写入)不列出并顺手清理,防止管理窗出现无法离线打开的幽灵项
        const entries = allEntries.filter(entry => keySet.has(entry.docId));
        const orphans = allEntries.filter(entry => !keySet.has(entry.docId)).map(entry => entry.docId);
        if (orphans.length) {
            const repairTx = db.transaction([STORE_DOCS, STORE_META], "readwrite");
            const meta = repairTx.objectStore(STORE_META);
            repairTx.objectStore(STORE_DOCS);
            meta.put(allEntries.filter(entry => keySet.has(entry.docId)), DOC_META_KEY);
        }
        const missing = entries.filter(entry => !entry.title);
        if (!missing.length) {
            return entries;
        }
        const derived: Record<string, string> = {};
        const deriveTx = db.transaction(STORE_DOCS, "readonly");
        const store = deriveTx.objectStore(STORE_DOCS);
        await Promise.all(missing.map(entry => new Promise<void>(resolve => {
            const request = store.get(entry.docId);
            request.onsuccess = () => {
                try {
                    const parsed = JSON.parse((request.result as CachedDoc)?.json || "{}") as IWebSocketData;
                    const docInfo = parsed?.data?.docInfo;
                    const title = docInfo?.ial?.title ?? parsed?.data?.ial?.title;
                    if (typeof title === "string" && title) {
                        derived[entry.docId] = title;
                    }
                } catch {
                    // 快照解析失败保留原样
                }
                resolve();
            };
            request.onerror = () => resolve();
        })));
        if (!Object.keys(derived).length) {
            return entries;
        }
        const patched = entries.map(entry => derived[entry.docId] ? {...entry, title: derived[entry.docId]} : entry);
        void updateDocCacheTitles(derived);
        return patched;
    } catch (error) {
        console.warn("offline doc cache list failed [" + error + "]");
        return [];
    }
};

// 删除单条缓存并清理指向它的别名
export const deleteDocCache = async (docId: string) => {
    await deleteDocCaches([docId]);
};

// 批量删除:文档被删除时联动清理其快照、元数据条目与别名;docId 与别名命中都清
export const deleteDocCaches = async (docIds: string[]) => {
    if (!docIds.length) {
        return;
    }
    try {
        const db = await openDb();
        const entries = await readEntries(db);
        const idSet = new Set(docIds);
        const aliases = await readAliases(db);
        // 别名(块 ID → 根文档 ID)反向命中:要删的块 ID 是某条目的别名入口时,该条目一并清除
        const hitIds = new Set(docIds);
        Object.entries(aliases).forEach(([alias, target]) => {
            if (idSet.has(alias) || idSet.has(target)) {
                hitIds.add(alias);
                hitIds.add(target);
            }
        });
        const kept = entries.filter(item => !hitIds.has(item.docId));
        const tx = db.transaction([STORE_DOCS, STORE_META], "readwrite");
        const docs = tx.objectStore(STORE_DOCS);
        hitIds.forEach(id => docs.delete(id));
        const meta = tx.objectStore(STORE_META);
        meta.put(kept, DOC_META_KEY);
        meta.put(evictAliases(aliases, hitIds), DOC_ALIAS_KEY);
    } catch (error) {
        console.warn("offline doc cache delete failed [" + error + "]");
    }
};

// 管理面板联机回填标题:仅更新元数据,不重写文档快照
export const updateDocCacheTitles = async (titles: Record<string, string>) => {
    try {
        const db = await openDb();
        const entries = (await readEntries(db)).map(entry => ({...entry, title: titles[entry.docId] || entry.title}));
        const tx = db.transaction(STORE_META, "readwrite");
        tx.objectStore(STORE_META).put(entries, DOC_META_KEY);
    } catch (error) {
        console.warn("offline doc cache title update failed [" + error + "]");
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
