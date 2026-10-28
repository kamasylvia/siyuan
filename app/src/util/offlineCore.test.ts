import assert from "node:assert/strict";
import {test} from "node:test";
import {DocCacheEntry, evictAliases, evictOverLimit, nextOffline, OFFLINE_GRACE_MS, wantOffline} from "./offlineCore";

test("wantOffline covers both signals", () => {
    assert.equal(wantOffline({navOnline: true, wsConnected: true}), false);
    assert.equal(wantOffline({navOnline: false, wsConnected: true}), true);
    assert.equal(wantOffline({navOnline: true, wsConnected: false}), true);
    assert.equal(wantOffline({navOnline: false, wsConnected: false}), true);
});

test("entering offline requires the grace period while recovery is immediate", () => {
    const signals: Parameters<typeof wantOffline>[0] = {navOnline: false, wsConnected: true};
    // 信号刚出现时仍在线
    assert.equal(nextOffline(false, signals, 1000, 1000 + OFFLINE_GRACE_MS - 1), false);
    // 持续超过宽限期后进入离线
    assert.equal(nextOffline(false, signals, 1000, 1000 + OFFLINE_GRACE_MS), true);
    // 已离线时保持
    assert.equal(nextOffline(true, signals, 1000, 1000 + OFFLINE_GRACE_MS + 5000), true);
    // 恢复在线立即生效
    assert.equal(nextOffline(true, {navOnline: true, wsConnected: true}, 1000, 1001), false);
});

test("offline state follows the latest signal change time", () => {
    // WS 闪断后 2 秒恢复,未过宽限期,始终在线
    assert.equal(nextOffline(false, {navOnline: true, wsConnected: false}, 0, 2000), false);
    assert.equal(nextOffline(false, {navOnline: true, wsConnected: true}, 2000, 2100), false);
    assert.equal(nextOffline(false, {navOnline: true, wsConnected: true}, 2000, 2100), false);
});

test("evictOverLimit keeps recent entries and drops oldest first", () => {
    const entries: DocCacheEntry[] = [
        {docId: "old", updated: "u1", bytes: 400, at: 1},
        {docId: "mid", updated: "u2", bytes: 400, at: 2},
        {docId: "new", updated: "u3", bytes: 400, at: 3},
    ];
    assert.deepEqual(evictOverLimit(entries, 900), [entries[0]]);
    assert.deepEqual(evictOverLimit(entries, 500), [entries[0], entries[1]]);
    assert.deepEqual(evictOverLimit(entries, 1200), []);
    // 更新 at 后不再是最旧
    const refreshed = [{...entries[0], at: 9}, entries[1], entries[2]];
    assert.deepEqual(evictOverLimit(refreshed, 900), [entries[1]]);
});

test("evictAliases drops entries pointing at evicted documents and evicted aliases", () => {
    const aliases = {"block-1": "doc-1", "block-2": "doc-2", "doc-1": "doc-1"};
    // doc-1 被淘汰:指向它的别名全部清除;doc-2 仍在:别名保留
    assert.deepEqual(evictAliases(aliases, new Set(["doc-1"])), {"block-2": "doc-2"});
    // 别名自身作为键被淘汰时同样清除
    assert.deepEqual(evictAliases(aliases, new Set(["block-2"])), {
        "block-1": "doc-1", "doc-1": "doc-1",
    });
    // 空淘汰集原样保留
    assert.deepEqual(evictAliases(aliases, new Set()), aliases);
});
