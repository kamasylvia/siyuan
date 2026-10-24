import * as assert from "node:assert/strict";
import {test} from "node:test";
import {genEmbedTableHTML, isTableEmbed} from "./embedTable";

test("table render mode requires the custom-render-mode attribute", () => {
    const fakeElement = (value: string | null) => ({getAttribute: (name: string): string | null => name === "custom-render-mode" ? value : null}) as unknown as Element;
    assert.equal(isTableEmbed(fakeElement("table")), true);
    assert.equal(isTableEmbed(fakeElement(null)), false);
    assert.equal(isTableEmbed(fakeElement("list")), false);
});

test("table rows escape text and keep the block id for navigation", () => {
    const html = genEmbedTableHTML([{
        block: {id: "20260101000000-abcdefg", rootID: "20260101000000-abcdefg", hPath: "/Notebook/Dir/Doc", content: "Doc <title> & \"quote\"", ial: {}},
        blockPaths: [],
        allowChildOperation: false,
    }]);
    assert.ok(html.includes("data-id=\"20260101000000-abcdefg\""));
    assert.ok(html.includes("Doc &lt;title> &amp; \"quote\""));
    assert.ok(html.includes("/Notebook/Dir/Doc"));
    assert.ok(html.includes("class=\"embed-table__row\""));
    assert.ok(html.includes("class=\"popover__block\""));
});

test("table rows decode escaped notebook names in hPath", () => {
    const html = genEmbedTableHTML([{
        block: {id: "20260101000000-abcdefg", hPath: "&lt;Notebook&gt;/Dir&#34;s/Doc", content: "Doc", ial: {}},
        blockPaths: [],
        allowChildOperation: false,
    }]);
    assert.ok(html.includes("&lt;Notebook>/Dir\"s/Doc"));
    assert.ok(!html.includes("&amp;lt;"));
    assert.ok(!html.includes("&amp;#34;"));
});

test("empty blocks render an empty table body without throwing", () => {
    const html = genEmbedTableHTML([]);
    assert.ok(html.startsWith("<div class=\"embed-table\""));
    assert.ok(html.endsWith("</tbody></table></div>"));
    assert.ok(!html.includes("<tr"));
});
