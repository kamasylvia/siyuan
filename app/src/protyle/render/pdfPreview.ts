import {getPdfViewerHTML} from "../../asset/pdf/viewerTemplate";
import {bindPdfTheme, loadPdfViewerModule} from "../../asset/pdfViewer";
import {genUUID} from "../../util/genID";
import {isPDFAssetHref, PDF_PREVIEW_ATTR} from "./pdfPreviewCore";

export {isPDFAssetHref, PDF_PREVIEW_ATTR};

// PDF 资产链接直接预览：编辑器渲染后为包含 assets/*.pdf 链接的块追加内嵌官方 PDF 查看器
// 使用 pdf.js 原生渲染（与资产页签同款）而非 iframe：远程客户端模式下子框架请求被官方安全层拦截，iframe 必然白屏
// 节点仅存在于前端 DOM，不进入事务与 .sy 数据；开关关闭时不插入并清理既有注入

// 预览容器骨架：render-node 语义 + 官方 PDF 查看器模板；高度固定保证单页可见
export const genPDFPreviewHTML = () => `<div ${PDF_PREVIEW_ATTR}="true" data-node-id="${genUUID()}" data-type="render-node" data-subtype="pdf" data-render="true" class="protyle-pdf-preview">
${getPdfViewerHTML()}
</div>`;

const pdfObjects = new Map<HTMLElement, {destroy?: () => void}>();

const loadPreview = async (container: HTMLElement, href: string) => {
    const baseURL = document.getElementById("baseURL").getAttribute("href");
    const file = href.startsWith("file") ? href : `${baseURL}/${href}`;
    try {
        const viewerModule = await loadPdfViewerModule();
        if (!container.isConnected) {
            return;
        }
        const pdfObject = viewerModule.webViewerLoad(file, container, 1, "");
        pdfObjects.set(container, pdfObject as {destroy?: () => void});
    } catch (error) {
        console.error("pdf preview load failed", error);
    }
};

export const pdfPreviewRender = (element: Element) => {
    // 清理上次注入与查看器实例，保证开关切换与重复渲染幂等
    element.querySelectorAll(`[${PDF_PREVIEW_ATTR}="true"]`).forEach(item => {
        const pdfObject = pdfObjects.get(item as HTMLElement);
        if (pdfObject?.destroy) {
            try {
                void pdfObject.destroy();
            } catch (error) {
                console.error("pdf preview destroy failed", error);
            }
        }
        pdfObjects.delete(item as HTMLElement);
        item.remove();
    });
    if (!window.siyuan.config.editor.pdfPreview) {
        return;
    }
    element.querySelectorAll(":scope > [data-node-id]").forEach((block) => {
        if (block.querySelector(".iframe, .protyle-pdf-preview")) {
            // 块内已有 iframe 或预览时视为用户自行管理，不重复注入
            return;
        }
        const linkElement = block.querySelector('span[data-type="a"][data-href]');
        const href = linkElement?.getAttribute("data-href") || "";
        if (!isPDFAssetHref(href)) {
            return;
        }
        block.insertAdjacentHTML("afterend", genPDFPreviewHTML());
        const container = block.nextElementSibling as HTMLElement;
        bindPdfTheme(container);
        void loadPreview(container, href);
    });
};
