// 离线缓存管理面板:列出缓存文档(标题/大小/时间),整行点击跳转,预览小窗先睹内容;
// 支持单条删除与清空;离线时点击底部横幅打开,兼作离线文档门户;联机时从设置-关于进入做清理。
import {confirmDialog} from "../dialog/confirmDialog";
import {Dialog} from "../dialog";
import {Constants} from "../constants";
import {clearDocCache, deleteDocCache, getDocCache, listDocCache} from "./offlineDocCache";
import {backfillOfflineTitles} from "./offlineSupport";
import {isOffline} from "./offlineState";

type DocCacheEntry = import("./offlineCore").DocCacheEntry;

const escapeHtml = (value: string) => value.replace(/[&<>"']/g, ch => ({
    "&": "&amp;", "<": "&lt;", ">": "&gt;", '"': "&quot;", "'": "&#39;",
}[ch] || ch));

const formatBytes = (bytes: number) => bytes > 1024 * 1024 ? (bytes / 1024 / 1024).toFixed(1) + " MB" :
    Math.max(1, Math.round(bytes / 1024)) + " KB";

const openCachedDoc = (docId: string) => {
    const {openFileById} = require("../editor/util") as typeof import("../editor/util");
    openFileById({app: window.siyuan.ws.app, id: docId, action: [Constants.CB_GET_FOCUS]});
};

// 联机时回填旧条目缺失的标题(批量 getDocsInfo),仅写元数据不重写文档快照;复用预取侧共享回填
const backfillTitles = (entries: DocCacheEntry[]): Promise<DocCacheEntry[]> => {
    const missing = entries.filter(entry => !entry.title).map(entry => entry.docId);
    if (!missing.length || isOffline()) {
        return Promise.resolve(entries);
    }
    return backfillOfflineTitles().then(() => listDocCache());
};

const openPreview = async (entry: DocCacheEntry) => {
    const cached = await getDocCache(entry.docId);
    if (!cached) {
        return;
    }
    const title = entry.title || window.siyuan.languages.offlineUntitledDoc;
    const previewDialog = new Dialog({
        title,
        content: `<div class="offline-cache__preview b3-typography">${cached.data?.content || ""}</div>
<div class="offline-cache__preview-bar">
    <button class="b3-button b3-button--text" data-action="open">${window.siyuan.languages.offlineCacheOpen}</button>
</div>`,
        width: "720px",
        height: "80vh",
    });
    previewDialog.element.querySelector("[data-action=open]").addEventListener("click", () => {
        previewDialog.destroy();
        openCachedDoc(entry.docId);
    });
};

export const openOfflineCacheManager = () => {
    const renderList = (element: HTMLElement, emptyText: string) => {
        void listDocCache().then(async entries => {
            const withTitles = await backfillTitles(entries);
            const listElement = element.querySelector(".offline-cache__list") as HTMLElement;
            if (!withTitles.length) {
                listElement.innerHTML = `<div class="b3-list--empty">${emptyText}</div>`;
                return;
            }
            listElement.innerHTML = withTitles.map(entry => `
<div class="b3-list-item" data-doc-id="${escapeHtml(entry.docId)}">
    <span class="b3-list-item__text">${escapeHtml(entry.title || window.siyuan.languages.offlineUntitledDoc)}
        <span class="b3-list-item__meta">${formatBytes(entry.bytes)} · ${new Date(entry.at).toLocaleString()}</span>
    </span>
    <span class="b3-list-item__action" data-action="preview"><svg><use xlink:href="#iconPreview"></use></svg></span>
    <span class="b3-list-item__action offline-cache__delete" data-action="delete">
        <svg><use xlink:href="#iconTrashcan"></use></svg>
    </span>
</div>`).join("");
            listElement.querySelectorAll<HTMLElement>(".b3-list-item").forEach(item => {
                const docId = item.getAttribute("data-doc-id");
                const entry = withTitles.find(candidate => candidate.docId === docId);
                item.addEventListener("click", (event) => {
                    const action = (event.target as HTMLElement).closest("[data-action]") as HTMLElement;
                    if (action?.getAttribute("data-action") === "delete") {
                        confirmDialog(window.siyuan.languages.offlineCacheTitle,
                            window.siyuan.languages.offlineCacheDeleteConfirm.replace("{title}",
                                entry?.title || window.siyuan.languages.offlineUntitledDoc), () => {
                                void deleteDocCache(docId).then(() => renderList(element, emptyText));
                            });
                        return;
                    }
                    if (action?.getAttribute("data-action") === "preview") {
                        void openPreview(entry);
                        return;
                    }
                    // 整行点击即跳转打开
                    dialog.destroy();
                    openCachedDoc(docId);
                });
            });
        });
    };

    const dialog = new Dialog({
        title: window.siyuan.languages.offlineCacheTitle,
        content: `<div class="offline-cache">
    <div class="fn__flex b3-label">
        <div class="fn__flex-1">${window.siyuan.languages.offlineCacheHint}</div>
        <button class="b3-button b3-button--outline fn__flex-center" data-action="clear">
            ${window.siyuan.languages.offlineCacheClear}
        </button>
    </div>
    <div class="offline-cache__list b3-list b3-list--background"></div>
</div>`,
        width: "520px",
    });
    const element = dialog.element;
    renderList(element, window.siyuan.languages.offlineCacheEmpty);
    element.querySelector("[data-action=clear]").addEventListener("click", () => {
        confirmDialog(window.siyuan.languages.offlineCacheTitle,
            window.siyuan.languages.offlineCacheClearConfirm, () => {
                void clearDocCache().then(() => renderList(element, window.siyuan.languages.offlineCacheEmpty));
            });
    });
};
