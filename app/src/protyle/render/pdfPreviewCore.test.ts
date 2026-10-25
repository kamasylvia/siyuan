import * as assert from "node:assert/strict";
import {test} from "node:test";
import {isPDFAssetHref, PDF_PREVIEW_ATTR} from "./pdfPreviewCore";

test("only assets pdf hrefs qualify for inline preview", () => {
    assert.equal(isPDFAssetHref("assets/demo.pdf"), true);
    assert.equal(isPDFAssetHref("assets/DEMO.PDF"), true);
    assert.equal(isPDFAssetHref("assets/sub/dir/demo.pdf"), true);
    assert.equal(isPDFAssetHref("https://example.com/a.pdf"), false);
    assert.equal(isPDFAssetHref("assets/demo.png"), false);
    assert.equal(isPDFAssetHref(""), false);
});

test("preview marker attribute stays stable for cleanup", () => {
    assert.equal(PDF_PREVIEW_ATTR, "data-pdf-preview");
});
