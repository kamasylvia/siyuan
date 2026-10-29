// 远程空间离线兜底的界面与锁:离线时展示底部横幅并将全部编辑器锁为只读,恢复时按锁前状态还原。
// 依赖编辑器侧模块,统一在回调内动态 require,避免静态循环依赖。

import {fetchPost} from "./fetch";
import {countDocCache, getCachedDocIds, getOfflineExpandableKeys, hasDocCache, listDocCache} from "./offlineDocCache";
import {isOffline, isOfflineEnabled, onOfflineChange} from "./offlineState";

type ProtyleEditor = import("../protyle").Protyle;

// 布局恢复完成后预取懒加载页签的文档:恢复自上次会话、本次未点开的页签没有缓存,
// 离线时打不开。预取与点开时走同一 getDoc 咽喉,已在缓存的跳过,离线时不发请求。
export const prefetchLazyDocCache = () => {
    if (!isOfflineEnabled() || isOffline()) {
        return;
    }
    const {getAllTabs} = require("../layout/getAll") as typeof import("../layout/getAll");
    const rootIds = new Set<string>();
    getAllTabs().forEach(tab => {
        if (tab.model) {
            const rootId = (tab.model as any)?.editor?.protyle?.block?.rootID;
            if (rootId) {
                rootIds.add(rootId as string);
            }
            return;
        }
        const initTab = tab.headElement.getAttribute("data-initdata");
        if (!initTab) {
            return;
        }
        try {
            const initTabData = JSON.parse(initTab);
            if (initTabData?.instance === "Editor" && initTabData.rootId) {
                rootIds.add(initTabData.rootId);
            }
        } catch (e) {
            console.warn("Failed to parse tab init data:", e);
        }
    });
    rootIds.forEach(rootId => {
        void hasDocCache(rootId).then(cached => {
            if (cached || isOffline()) {
                return;
            }
            fetchPost("/api/filetree/getDoc", {
                id: rootId,
                includeDocInfo: true,
                mode: 0,
                size: window.siyuan.config.editor.dynamicLoadBlocks,
            }, () => {});
        });
    });
    // 联机回填旧条目缺失的标题与路径,断网前就绪,面板不再显示文档 ID,合成目录列表也能覆盖旧条目
    void backfillOfflineTitles();
    void backfillOfflinePaths();
};

// 批量拉取无标题条目的标题并写入元数据;供预取与管理面板共用
export const backfillOfflineTitles = (): Promise<void> => {
    return listDocCache().then(async entries => {
        const missing = entries.filter(entry => !entry.title).map(entry => entry.docId);
        if (!missing.length || isOffline()) {
            return;
        }
        const {fetchSyncPost} = require("./fetch") as typeof import("./fetch");
        const {updateDocCacheTitles} = require("./offlineDocCache") as typeof import("./offlineDocCache");
        const response = await fetchSyncPost("/api/block/getDocsInfo", {ids: missing.slice(0, 200), refCount: false, av: false});
        if (response.code !== 0 || !Array.isArray(response.data)) {
            return;
        }
        const titles: Record<string, string> = {};
        response.data.forEach((info: { id: string, ial?: Record<string, string> }) => {
            const title = info.ial?.title || "";
            if (title) {
                titles[info.id] = title;
            }
        });
        if (Object.keys(titles).length) {
            await updateDocCacheTitles(titles);
        }
    });
};

// 批量回填旧条目缺失的路径与笔记本 ID(逐条 getBlockInfo,封顶 40 条),合成目录列表依赖这两项
export const backfillOfflinePaths = (): Promise<void> => {
    return listDocCache().then(async entries => {
        const missing = entries.filter(entry => !entry.path || !entry.notebook).map(entry => entry.docId);
        if (!missing.length || isOffline()) {
            return;
        }
        const {fetchSyncPost} = require("./fetch") as typeof import("./fetch");
        const {updateDocCachePaths} = require("./offlineDocCache") as typeof import("./offlineDocCache");
        const updates: Record<string, { path: string, notebook: string }> = {};
        for (const docId of missing.slice(0, 40)) {
            try {
                const response = await fetchSyncPost("/api/block/getBlockInfo", {id: docId});
                if (response.code === 0 && typeof response.data?.path === "string" &&
                    typeof response.data?.box === "string") {
                    updates[docId] = {path: response.data.path, notebook: response.data.box};
                }
            } catch (error) {
                console.warn("offline path backfill failed [" + docId + "]", error);
            }
        }
        if (Object.keys(updates).length) {
            await updateDocCachePaths(updates);
        }
    });
};

// 离线时按缓存状态标记文件树:未缓存文档灰显(点击仍会得到「未缓存」提示),恢复在线时清除标记
let treeObserver: MutationObserver;
// 灰显仅标记「死胡同」:文档未缓存 且 所在目录的展开缓存也没有(离线既打不开也无法展开定位)。
// 可展开的父目录(目录展开缓存命中)保持正常色,作为离线导航路径。
const markTreeCacheState = () => {
    const items = [...document.querySelectorAll(".b3-list-item[data-node-id]")];
    if (!items.length) {
        return;
    }
    void Promise.all([getCachedDocIds(), getOfflineExpandableKeys()]).then(([cachedDocs, cachedTrees]) => {
        items.forEach(item => {
            const nodeId = item.getAttribute("data-node-id");
            const docPath = item.getAttribute("data-path") || "";
            // 笔记本根条目(data-path="/" 且无 node-id)只是导航锚点,永不灰显;其余条目需有路径才标记
            if (!nodeId || !docPath.startsWith("/") || docPath === "/") {
                return;
            }
            const notebook = docPath.split("/")[1] || "";
            const dir = docPath.slice(0, docPath.lastIndexOf("/")) || "/";
            const normalizedDir = dir === "/" + notebook ? "/" : dir;
            // 展开键命中任一形态即算可展开:目录页预取键(无 .sy)、文档自身子列表键(带 .sy)、笔记本根列表键
            const expandable = cachedTrees.has(notebook + "\u0000" + normalizedDir) ||
                cachedTrees.has(notebook + "\u0000" + docPath) ||
                cachedTrees.has(notebook + "\u0000" + docPath.replace(/\.sy$/, "")) ||
                cachedTrees.has(notebook + "\u0000" + "/");
            const docCached = cachedDocs.has(nodeId);
            item.classList.toggle("offline-uncached", !docCached && !expandable);
        });
    });
};

const watchTreeCacheState = () => {
    if (treeObserver) {
        return;
    }
    treeObserver = new MutationObserver(() => {
        if (isOffline()) {
            markTreeCacheState();
        } else {
            document.querySelectorAll(".b3-list-item.offline-uncached").forEach(item => {
                item.classList.remove("offline-uncached");
            });
        }
    });
    const tree = document.querySelector("#fileTree, .file-tree") || document.body;
    treeObserver.observe(tree, {childList: true, subtree: true});
};

export const initOfflineSupport = () => {
    const banner = document.createElement("div");
    banner.id = "offlineBanner";
    banner.className = "offline-banner fn__none";
    document.body.prepend(banner);
    // 离线时点击横幅打开缓存管理面板,兼作离线文档门户
    banner.addEventListener("click", () => {
        const {openOfflineCacheManager} = require("./offlineCacheManager") as typeof import("./offlineCacheManager");
        openOfflineCacheManager();
    });
    // 锁定前的只读状态,恢复时按原样还原
    watchTreeCacheState();
    const lockedBefore = new Map<ProtyleEditor, boolean>();
    onOfflineChange(offline => {
        banner.textContent = window.siyuan.languages.offlineReadonlyTip;
        banner.classList.toggle("fn__none", !offline);
        // 附缓存计数,断网时可直接判断哪些页签可离线打开
        if (offline) {
            void countDocCache().then(count => {
                if (count >= 0 && !banner.classList.contains("fn__none")) {
                    banner.textContent = window.siyuan.languages.offlineReadonlyTip +
                        " · " + (window.siyuan.languages.offlineCachedCount || "cached: {n}").replace("{n}", String(count));
                }
            });
            void markTreeCacheState();
        }
        const {getAllEditor} = require("../layout/getAll") as typeof import("../layout/getAll");
        if (offline) {
            getAllEditor().forEach(editor => {
                lockedBefore.set(editor, editor.protyle.disabled);
                editor.disable();
            });
        } else {
            lockedBefore.forEach((wasDisabled, editor) => {
                if (wasDisabled) {
                    editor.disable();
                } else {
                    editor.enable();
                }
            });
            lockedBefore.clear();
        }
    });
};
