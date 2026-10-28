// 远程空间离线兜底的界面与锁:离线时展示底部横幅并将全部编辑器锁为只读,恢复时按锁前状态还原。
// 依赖编辑器侧模块,统一在回调内动态 require,避免静态循环依赖。

import {fetchPost} from "./fetch";
import {hasDocCache} from "./offlineDocCache";
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
};

export const initOfflineSupport = () => {
    const banner = document.createElement("div");
    banner.id = "offlineBanner";
    banner.className = "offline-banner fn__none";
    document.body.prepend(banner);
    // 锁定前的只读状态,恢复时按原样还原
    const lockedBefore = new Map<ProtyleEditor, boolean>();
    onOfflineChange(offline => {
        banner.textContent = window.siyuan.languages.offlineReadonlyTip;
        banner.classList.toggle("fn__none", !offline);
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
