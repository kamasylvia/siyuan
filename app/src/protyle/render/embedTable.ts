import {escapeAttr, escapeHtml} from "../../util/escape";

// 嵌入块表格渲染模式：块属性 custom-render-mode="table" 时生效
export const isTableEmbed = (element: Element) => element.getAttribute("custom-render-mode") === "table";

// 解码内核 html.EscapeString 产生的实体，hPath 中的笔记本名经过转义（&amp; 置于最后避免二次解码）
const decodeKernelEntities = (text: string) =>
    text.replace(/&lt;/g, "<").replace(/&gt;/g, ">").replace(/&#39;/g, "'").replace(/&#34;/g, "\"").replace(/&amp;/g, "&");

// 将嵌入块查询结果构建为可点击表格，行点击跳转到对应块，用于在父文档中建立子文档目录
export const genEmbedTableHTML = (blocks: {
    block: IBlock,
    blockPaths: IBreadcrumb[],
    allowChildOperation: boolean
}[]) => {
    let rows = "";
    blocks.forEach((blocksItem) => {
        const id = blocksItem.block.id || "";
        // 内核表格模式下 content 为纯文本名称
        const title = blocksItem.block.content || "";
        const hPath = decodeKernelEntities(blocksItem.block.hPath || "");
        rows += `<tr class="embed-table__row" data-id="${escapeAttr(id)}">
<td class="embed-table__title"><span class="popover__block" data-id="${escapeAttr(id)}">${escapeHtml(title)}</span></td>
<td class="embed-table__path">${escapeHtml(hPath)}</td>
</tr>`;
    });
    return `<div class="embed-table" contenteditable="false"><table><tbody>${rows}</tbody></table></div>`;
};
