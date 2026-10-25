import {escapeAttr, escapeHtml} from "../../util/escape";

// 嵌入块渲染模式：块属性 custom-render-mode 控制（table 轻量表格 / raw 原始 SQL 行 / recent 最近浏览）
export const getEmbedRenderMode = (element: Element) => element.getAttribute("custom-render-mode") || "";

export const isTableEmbed = (element: Element) => getEmbedRenderMode(element) === "table";

// 解码内核 html.EscapeString 产生的实体，hPath 中的笔记本名经过转义（&amp; 置于最后避免二次解码）
const decodeKernelEntities = (text: string) =>
    text.replace(/&lt;/g, "<").replace(/&gt;/g, ">").replace(/&#39;/g, "'").replace(/&#34;/g, "\"").replace(/&amp;/g, "&");

// 将嵌入块查询结果构建为可点击表格，行点击跳转到对应块，用于在父文档中建立子文档目录
// raw/recent 模式的原始行表格：内核返回有序列名与字符串行数据
export const genRawTableHTML = (columns: string[], rows: string[][]) => {
    let head = "";
    columns.forEach((column) => {
        head += `<th>${escapeHtml(column)}</th>`;
    });
    let body = "";
    rows.forEach((row) => {
        body += "<tr>";
        row.forEach((cell) => {
            body += `<td>${escapeHtml(cell)}</td>`;
        });
        body += "</tr>";
    });
    return `<div class="embed-table" contenteditable="false"><table><thead><tr>${head}</tr></thead><tbody>${body}</tbody></table></div>`;
};

export const genEmbedTableHTML = (blocks: {
    block: IBlock,
    blockPaths: IBreadcrumb[],
    allowChildOperation: boolean,
    rawRows?: {columns: string[], rows: string[][]}
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
