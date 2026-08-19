// SPDX-FileCopyrightText: Copyright The Miniflux Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

const fs = require("node:fs");
const test = require("node:test");
const assert = require("node:assert/strict");
const vm = require("node:vm");

const context = vm.createContext({
    Blob,
    Response,
    console,
    setTimeout,
});
vm.runInContext(fs.readFileSync("internal/ui/static/js/offline.js", "utf8"), context);

test("service worker script parses", () => {
    assert.doesNotThrow(() => new vm.Script(fs.readFileSync("internal/ui/static/js/service_worker.js", "utf8")));
});

test("offline media accepts responses below four million bytes", async () => {
    const response = new Response(new Uint8Array(3999999));
    const cached = await context.responseBelowOfflineMediaLimit(response);
    assert.notEqual(cached, null);
    assert.equal((await cached.arrayBuffer()).byteLength, 3999999);
});

test("offline media rejects responses at four million bytes", async () => {
    const response = new Response(new Uint8Array(4000000));
    const cached = await context.responseBelowOfflineMediaLimit(response);
    assert.equal(cached, null);
});

test("status and saved-for-later conflicts stay coupled", () => {
    const fields = context.conflictPatchFields([{field: "status_saved_for_later"}]);
    assert.deepEqual(Array.from(fields).sort(), ["saved_for_later", "status"]);
});

test("formats offline article progress", () => {
    assert.equal(context.offlineProgressText(12, 40, "articles cached"), "12/40 articles cached");
});

test("limits offline batch concurrency", async () => {
    let active = 0;
    let maximum = 0;
    await context.runOfflineBatches([1, 2, 3, 4, 5], 2, async () => {
        active += 1;
        maximum = Math.max(maximum, active);
        await new Promise((resolve) => setTimeout(resolve, 1));
        active -= 1;
    });
    assert.equal(maximum, 2);
});

test("refreshes only missing or changed offline entries", () => {
    const cached = new Set([1, 2]);
    assert.equal(context.offlineEntryNeedsRefresh(3, cached, {}, {3: "2026-08-19T10:00:00Z"}), true);
    assert.equal(context.offlineEntryNeedsRefresh(1, cached, {1: "v1"}, {1: "v1"}), false);
    assert.equal(context.offlineEntryNeedsRefresh(1, cached, {1: "v1"}, {1: "v2"}), true);
    assert.equal(context.offlineEntryNeedsRefresh(2, cached, undefined, {2: "2026-08-19T10:00:00Z"}), true);
    assert.equal(context.offlineEntryNeedsRefresh(2, cached, undefined, {2: "2026-08-19T10:00:00Z"}, Date.parse("2026-08-19T11:00:00Z")), false);
});

test("refreshes offline lists only when needed", () => {
    const allowed = new Set([1, 2]);
    assert.equal(context.offlineListNeedsRefresh(allowed, [1, 2], new Set(), true), false);
    assert.equal(context.offlineListNeedsRefresh(allowed, [1], new Set(), true), true);
    assert.equal(context.offlineListNeedsRefresh(allowed, [1, 2], new Set([2]), true), true);
    assert.equal(context.offlineListNeedsRefresh(allowed, [1, 2], new Set(), false), true);
});
