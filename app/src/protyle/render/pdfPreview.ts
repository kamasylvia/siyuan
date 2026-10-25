import {genUUID} from "../../util/genID";

// PDF 资产链接直接预览：编辑器渲染后为包含 assets/*.pdf 链接的块追加内嵌 iframe 显示节点
// 节点仅存在于前端 DOM，不进入事务与 .sy 数据；开关关闭时不插入并清理既有注入

export const PDF_PREVIEW_ATTR = "data-pdf-preview";

export const isPDFAssetHref = (href: string) => {
    const lower = href.trim().toLowerCase();
    return lower.startsWith("assets/") && lower.endsWith(".pdf");
};

export const genPDFPreviewHTML = (src: string, id: string) => `<div ${PDF_PREVIEW_ATTR}="true" data-node-id="${id}" data-type="NodeIFrame" class="iframe" data-render="true">
<div class="iframe-content"><iframe src="/${src}" style="width: 100%; height: 60vh; border: 0" scrolling="auto" loading="lazy"></iframe></div>
<span class="protyle-action__drag" contenteditable="false"></span>
<div class="protyle-attr" contenteditable="false">​</div>
</div>`;

export const pdfPreviewRender = (element: Element) => {
    // 清理上次注入，保证开关切换与重复渲染幂等
    element.querySelectorAll(`[${PDF_PREVIEW_ATTR}="true"]`).forEach(item => item.remove());
    if (!window.siyuan.config.editor.pdfPreview) {
        return;
    }
    element.querySelectorAll(":scope > [data-node-id]").forEach((block) => {
        if (block.querySelector(".iframe")) {
            // 块内已有 iframe 时视为用户自行管理，不重复注入
            return;
        }
        const linkElement = block.querySelector('span[data-type="a"][data-href]');
        const href = linkElement?.getAttribute("data-href") || "";
        if (!isPDFAssetHref(href)) {
            return;
        }
        block.insertAdjacentHTML("afterend", genPDFPreviewHTML(href, genUUID()));
    });
};
