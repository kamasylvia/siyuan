import * as assert from "node:assert/strict";
import {randomFillSync} from "node:crypto";
import {test} from "node:test";
import {genPDFPreviewHTML, isPDFAssetHref, PDF_PREVIEW_ATTR, pdfPreviewRender} from "./pdfPreview";

test("only assets pdf hrefs qualify for inline preview", () => {
    assert.equal(isPDFAssetHref("assets/demo.pdf"), true);
    assert.equal(isPDFAssetHref("assets/DEMO.PDF"), true);
    assert.equal(isPDFAssetHref("assets/sub/dir/demo.pdf"), true);
    assert.equal(isPDFAssetHref("https://example.com/a.pdf"), false);
    assert.equal(isPDFAssetHref("assets/demo.png"), false);
    assert.equal(isPDFAssetHref(""), false);
});

test("preview html targets the asset path and carries the marker attribute", () => {
    const html = genPDFPreviewHTML("assets/demo.pdf", "20260101000000-abcdefg");
    assert.ok(html.includes(`${PDF_PREVIEW_ATTR}="true"`));
    assert.ok(html.includes("src=\"/assets/demo.pdf\""));
    assert.ok(html.includes("data-type=\"NodeIFrame\""));
});

interface FakeBlock {
    href: string | null;
    withIframe?: boolean;
    injected?: string[];
    failOnInject?: boolean;
}

const buildElement = (blocks: FakeBlock[], removed: string[]) => ({
    querySelectorAll: (selector: string) => {
        if (selector.includes(PDF_PREVIEW_ATTR)) {
            return [{remove: () => removed.push("marker")}];
        }
        if (selector.includes("data-node-id")) {
            return blocks.map(block => ({
                querySelector: (inner: string) => {
                    if (inner === ".iframe") {
                        return block.withIframe ? {} : null;
                    }
                    if (inner === "span[data-type=\"a\"][data-href]") {
                        return block.href
                            ? {getAttribute: (name: string) => name === "data-href" ? block.href : "a"}
                            : null;
                    }
                    return null;
                },
                insertAdjacentHTML: (position: string, html: string) => {
                    if (block.failOnInject || position !== "afterend") {
                        throw new Error("unexpected injection");
                    }
                    block.injected?.push(html);
                },
            }));
        }
        return [];
    },
});

test("pdfPreviewRender injects after qualifying blocks and cleans previous injections", () => {
    const globalWindow = (globalThis as { window?: unknown }).window;
    (globalThis as {window?: unknown}).window = {
        crypto: {getRandomValues: (array: Uint8Array) => randomFillSync(array)},
        siyuan: {config: {editor: {pdfPreview: true}}},
    };
    const removed: string[] = [];
    const pdfBlock: FakeBlock = {href: "assets/demo.pdf", injected: []};
    const pngBlock: FakeBlock = {href: "assets/demo.png", failOnInject: true};
    const iframeBlock: FakeBlock = {href: "assets/demo.pdf", withIframe: true, failOnInject: true};
    const externalBlock: FakeBlock = {href: "https://example.com/a.pdf", failOnInject: true};
    try {
        pdfPreviewRender(buildElement([pdfBlock, pngBlock, iframeBlock, externalBlock], removed) as unknown as Element);
    } finally {
        (globalThis as {window?: unknown}).window = globalWindow;
    }
    assert.deepEqual(removed, ["marker"]);
    assert.equal(pdfBlock.injected?.length, 1);
    assert.ok(pdfBlock.injected?.[0].includes("assets/demo.pdf"));
});

test("pdfPreviewRender skips injection when the setting is off", () => {
    const globalWindow = (globalThis as { window?: unknown }).window;
    (globalThis as {window?: unknown}).window = {
        siyuan: {config: {editor: {pdfPreview: false}}},
    };
    const removed: string[] = [];
    const pdfBlock: FakeBlock = {href: "assets/demo.pdf", failOnInject: true};
    try {
        pdfPreviewRender(buildElement([pdfBlock], removed) as unknown as Element);
    } finally {
        (globalThis as {window?: unknown}).window = globalWindow;
    }
    assert.deepEqual(removed, ["marker"]);
});
