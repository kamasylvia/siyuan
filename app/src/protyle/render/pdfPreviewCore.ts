// PDF 内嵌预览的无依赖核心规则，便于单测；重依赖（viewer 模块链）留在 pdfPreview.ts
export const PDF_PREVIEW_ATTR = "data-pdf-preview";

export const isPDFAssetHref = (href: string) => {
    const lower = href.trim().toLowerCase();
    return lower.startsWith("assets/") && lower.endsWith(".pdf");
};
