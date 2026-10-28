// 离线缓存管理面板:列出缓存文档(标题/大小/时间),支持单条删除与清空;
// 离线时点击底部横幅打开,兼作离线文档门户;联机时从设置-关于进入做清理。
import {confirmDialog} from "../dialog/confirmDialog";
import {Dialog} from "../dialog";
import {Constants} from "../constants";
import {clearDocCache, deleteDocCache, listDocCache} from "./offlineDocCache";

const escapeHtml = (value: string) => value.replace(/[&<>"']/g, ch => ({
    "&": "&amp;", "<": "&lt;", ">": "&gt;", '"': "&quot;", "'": "&#39;",
}[ch] || ch));

const formatBytes = (bytes: number) => bytes > 1024 * 1024 ? (bytes / 1024 / 1024).toFixed(1) + " MB" :
    Math.max(1, Math.round(bytes / 1024)) + " KB";

const openCachedDoc = (docId: string) => {
    const {openFileById} = require("../editor/util") as typeof import("../editor/util");
    openFileById({app: window.siyuan.ws.app, id: docId, action: [Constants.CB_GET_FOCUS]});
};

export const openOfflineCacheManager = () => {
    const renderList = (element: HTMLElement, emptyText: string) => {
        void listDocCache().then(entries => {
            const listElement = element.querySelector(".offline-cache__list") as HTMLElement;
            if (!entries.length) {
                listElement.innerHTML = `<div class="b3-list--empty">${emptyText}</div>`;
                return;
            }
            listElement.innerHTML = entries.map(entry => `
<div class="b3-list-item" data-doc-id="${escapeHtml(entry.docId)}">
    <span class="b3-list-item__text">${escapeHtml(entry.title || entry.docId)}
        <span class="b3-list-item__meta">${formatBytes(entry.bytes)} · ${new Date(entry.at).toLocaleString()}</span>
    </span>
    <span class="b3-list-item__action" data-action="open"><svg><use xlink:href="#iconRight"></use></svg></span>
    <span class="b3-list-item__action" data-action="delete"><svg><use xlink:href="#iconTrash"></use></svg></span>
</div>`).join("");
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
    element.addEventListener("click", (event) => {
        const target = (event.target as HTMLElement).closest("[data-action]") as HTMLElement;
        if (!target) {
            return;
        }
        if (target.getAttribute("data-action") === "clear") {
            confirmDialog(window.siyuan.languages.offlineCacheTitle,
                window.siyuan.languages.offlineCacheClearConfirm, () => {
                    void clearDocCache().then(() => renderList(element, window.siyuan.languages.offlineCacheEmpty));
                });
            return;
        }
        const item = target.closest("[data-doc-id]") as HTMLElement;
        const docId = item?.getAttribute("data-doc-id");
        if (!docId) {
            return;
        }
        if (target.getAttribute("data-action") === "delete") {
            confirmDialog(window.siyuan.languages.offlineCacheTitle,
                window.siyuan.languages.offlineCacheDeleteConfirm.replace("{title}",
                    item.querySelector(".b3-list-item__text")?.textContent || docId), () => {
                    void deleteDocCache(docId).then(() => renderList(element, window.siyuan.languages.offlineCacheEmpty));
                });
            return;
        }
        if (target.getAttribute("data-action") === "open") {
            dialog.destroy();
            openCachedDoc(docId);
        }
    });
};
