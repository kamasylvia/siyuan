// 远程空间离线兜底的核心纯函数:状态判定、防抖转换、缓存键与 LRU 淘汰。
// 不依赖 DOM 与全局状态,便于 node:test 单测;运行时接线在 offlineState.ts。

export const OFFLINE_GRACE_MS = 5000;
export const DOC_CACHE_LIMIT_BYTES = 500 * 1024 * 1024;
export const DOC_CACHE_LIMIT_COUNT = 50;
export const OFFLINE_DOC_STORE = "siyuan-offline-docs";
export const OFFLINE_META_KEY = "siyuan-offline-meta";

export interface OfflineSignals {
    navOnline: boolean;
    wsConnected: boolean;
}

export const wantOffline = (signals: OfflineSignals) => !signals.navOnline || !signals.wsConnected;

// 进入离线需要信号持续超过宽限期(容忍 WS 闪断),恢复在线立即生效。
export const nextOffline = (current: boolean, signals: OfflineSignals, changedAt: number, now: number,
    graceMs: number = OFFLINE_GRACE_MS): boolean => {
    const want = wantOffline(signals);
    if (current === want) {
        return current;
    }
    if (!want) {
        return false;
    }
    return now - changedAt >= graceMs;
};

export interface DocCacheEntry {
    docId: string;
    updated: string;
    bytes: number;
    at: number;
    rootId?: string;
    title?: string;
    path?: string;
    notebook?: string;
}

// 淘汰记录时同时给出应删除的别名(块 id → 根文档 id 映射),避免别名悬空指向已淘汰文档。
export const evictAliases = (aliases: Record<string, string>, evicted: Set<string>): Record<string, string> => {
    const kept: Record<string, string> = {};
    for (const [alias, docId] of Object.entries(aliases)) {
        if (!evicted.has(docId) && !evicted.has(alias)) {
            kept[alias] = docId;
        }
    }
    return kept;
};

// 追加/更新一条缓存记录后,超出容量上限时按最后访问时间升序给出应淘汰的记录。
export const evictOverLimit = (entries: DocCacheEntry[], limitBytes: number = DOC_CACHE_LIMIT_BYTES,
    limitCount: number = DOC_CACHE_LIMIT_COUNT): DocCacheEntry[] => {
    const overBytes = evictOverBytes(entries, limitBytes);
    const overCount = evictOverCount(entries, limitCount);
    const evicted = new Set([...overBytes, ...overCount].map(entry => entry.docId));
    return orderedByAccess(entries).filter(entry => evicted.has(entry.docId));
};

const orderedByAccess = (entries: DocCacheEntry[]) => [...entries].sort((a, b) => a.at - b.at);

const evictOverBytes = (entries: DocCacheEntry[], limitBytes: number): DocCacheEntry[] => {
    const total = entries.reduce((sum, entry) => sum + entry.bytes, 0);
    const evicted: DocCacheEntry[] = [];
    let remaining = total;
    for (const entry of orderedByAccess(entries)) {
        if (remaining <= limitBytes) {
            break;
        }
        evicted.push(entry);
        remaining -= entry.bytes;
    }
    return evicted;
};

const evictOverCount = (entries: DocCacheEntry[], limitCount: number): DocCacheEntry[] => {
    const ordered = orderedByAccess(entries);
    if (ordered.length <= limitCount) {
        return [];
    }
    return ordered.slice(0, ordered.length - limitCount);
};
