// 远程空间离线兜底的界面与锁:离线时展示顶部横幅并将全部编辑器锁为只读,恢复时按锁前状态还原。
// 依赖编辑器侧模块,统一在回调内动态 require,避免静态循环依赖。

import {onOfflineChange} from "./offlineState";

type ProtyleEditor = import("../protyle").Protyle;

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
