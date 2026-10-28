import assert from "node:assert/strict";
import {test} from "node:test";
import {clearPluginSnooze, filterSnoozed, PLUGIN_SNOOZE_THRESHOLD, recordPluginFailure, shouldSnooze} from "./pluginResilience";

test("failures accumulate per plugin and reach the threshold", () => {
    const state = {};
    assert.equal(recordPluginFailure(state, "a"), 1);
    assert.equal(recordPluginFailure(state, "a"), 2);
    assert.equal(shouldSnooze(state["a"], 3), false);
    assert.equal(recordPluginFailure(state, "a"), 3);
    assert.equal(shouldSnooze(state["a"], 3), true);
    // 其它插件互不影响
    assert.equal(recordPluginFailure(state, "b"), 1);
    assert.equal(shouldSnooze(state["b"], 3), false);
});

test("explicit enable or uninstall clears the snooze entry", () => {
    const state = {"a": 3, "b": 1};
    const next = clearPluginSnooze(state, "a");
    assert.equal(next["a"], undefined);
    assert.equal(next["b"], 1);
    // 原状态不被改动
    assert.equal(state["a"], 3);
});

test("filterSnoozed splits allowed items from blocked names", () => {
    const items = [{name: "a"}, {name: "b"}, {name: "c"}];
    const [allowed, blocked] = filterSnoozed(items, {"b": PLUGIN_SNOOZE_THRESHOLD});
    assert.deepEqual(allowed.map(i => i.name), ["a", "c"]);
    assert.deepEqual(blocked, ["b"]);
    const [allowed2, blocked2] = filterSnoozed(items, {});
    assert.equal(allowed2.length, 3);
    assert.equal(blocked2.length, 0);
});
