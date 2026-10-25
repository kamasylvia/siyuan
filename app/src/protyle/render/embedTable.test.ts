import * as assert from "node:assert/strict";
import {test} from "node:test";
import {genEmbedTableHTML, genRawTableHTML} from "./embedTable";

// genEmbedTableHTML 依赖多语言表头，测试环境提供最小 mock
const mockLanguages = () => {
    const globalWindow = (globalThis as { window?: unknown }).window;
    (globalThis as {window?: unknown}).window = {
        siyuan: {languages: {embedTableIndex: "序号", fileName: "文件名", embedTableDir: "文件目录"}},
    };
    return () => {
        (globalThis as {window?: unknown}).window = globalWindow;
    };
};

const block = (id: string, content: string, hPath: string) => ({
    block: {id, content, hPath, ial: {}},
    blockPaths: [] as IBreadcrumb[],
    allowChildOperation: false,
});

test("table rows render index, file name and directory columns with row click ids", () => {
    const restore = mockLanguages();
    try {
        const html = genEmbedTableHTML([
            block("20260101000000-abcdefg", "00-Apple", "Obsidian/10-Resources/11-CS/00-Apple"),
            block("20260101000000-bcdefgh", "root doc", "Obsidian"),
        ]);
        assert.ok(html.includes("<thead><tr><th>序号</th><th>文件名</th><th>文件目录</th></tr></thead>"));
        assert.ok(html.includes("data-id=\"20260101000000-abcdefg\""));
        assert.ok(html.includes(">1<"));
        assert.ok(html.includes(">2<"));
        assert.ok(html.includes(">00-Apple<"));
        assert.ok(html.includes("Obsidian/10-Resources/11-CS"));
        assert.ok(html.includes('class="embed-table__row"'));
    } finally {
        restore();
    }
});

test("root-level docs show an empty directory column and escaped cells", () => {
    const restore = mockLanguages();
    try {
        const html = genEmbedTableHTML([block("20260101000000-abcdefg", "<title>", "Obsidian")]);
        assert.ok(html.includes("&lt;title>"));
    } finally {
        restore();
    }
});

test("raw table renders escaped headers and rows", () => {
    const html = genRawTableHTML(["指标", "数值"], [["全库文档总数", "840"]]);
    assert.ok(html.includes("<th>指标</th>"));
    assert.ok(html.includes("<td>840</td>"));
});
