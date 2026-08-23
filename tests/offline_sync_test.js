// SPDX-FileCopyrightText: Copyright The Miniflux Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

const fs = require("node:fs");
const test = require("node:test");
const assert = require("node:assert/strict");
const vm = require("node:vm");

const context = vm.createContext({
    Blob,
    Response,
    URL,
    console,
    setTimeout,
});
const offlineSource = fs.readFileSync("internal/ui/static/js/offline.js", "utf8");
vm.runInContext(offlineSource, context);

test("service worker script parses", () => {
    assert.doesNotThrow(() => new vm.Script(fs.readFileSync("internal/ui/static/js/service_worker.js", "utf8")));
});

test("clear offline data reloads into a fresh sync", () => {
    assert.match(offlineSource, /await clearOfflineData\(\);\s*location\.reload\(\);/);
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

test("extracts direct audio and video media", () => {
    const element = (attributes) => ({getAttribute: (name) => attributes[name] || null});
    const image = element({src: "/image.jpg"});
    const audio = element({src: "/audio.mp3"});
    const video = element({src: "/video.mp4"});
    const poster = element({poster: "/poster.jpg"});
    const documentNode = {
        querySelectorAll: (selector) => [
            ...(selector.includes("img[src]") ? [image] : []),
            ...(selector.includes("audio[src]") ? [audio] : []),
            ...(selector.includes("video[src]") ? [video] : []),
            ...(selector.includes("video[poster]") ? [poster] : []),
        ],
    };
    context.location = {origin: "https://miniflux.test"};

    assert.deepEqual(Array.from(context.mediaURLsFromDocument(documentNode)), [
        "https://miniflux.test/image.jpg",
        "https://miniflux.test/audio.mp3",
        "https://miniflux.test/video.mp4",
        "https://miniflux.test/poster.jpg",
    ]);
});

test("selects unique media for retained entries", () => {
    const records = [
        {key: "entryMedia:7:1", value: ["a", "b"]},
        {key: "entryMedia:7:2", value: ["b", "c"]},
        {key: "entryMedia:8:1", value: ["d"]},
    ];
    assert.deepEqual(Array.from(context.offlineMediaURLs(records, 7, new Set([1, 2]))), ["a", "b", "c"]);
    assert.deepEqual(Array.from(context.offlineMediaURLs(records, 7, new Set([2, 1]))), ["b", "c", "a"]);
    assert.deepEqual(Array.from(context.offlineMediaURLs(records, 7, new Set([2]))), ["b", "c"]);
});

test("starts media progress from files already cached", () => {
    const work = context.offlineMediaWork(
        ["https://example.test/a", "https://example.test/b", "https://example.test/c"],
        [{url: "https://example.test/a"}, {url: "https://example.test/c"}],
    );
    assert.equal(work.completed, 2);
    assert.equal(work.total, 3);
    assert.deepEqual(Array.from(work.pending), ["https://example.test/b"]);
});

test("offline entry views preserve mark-as-read behavior", () => {
    const unreadEntry = {
        querySelector: (selector) => selector === ":is(a, button)[data-toggle-status]" ? {dataset: {value: "unread"}} : null,
    };
    const mediaEntry = {
        querySelector: (selector) => selector === "[data-mark-read-on-completion]"
            ? {}
            : unreadEntry.querySelector(selector),
    };
    const readEntry = {
        querySelector: (selector) => selector === ":is(a, button)[data-toggle-status]" ? {dataset: {value: "read"}} : null,
    };

    assert.equal(context.offlineEntryShouldMarkReadOnView(
        {dataset: {offlineSnapshot: "true", markAsReadOnView: "true"}},
        "/unread/entry/7",
        unreadEntry,
    ), true);
    assert.equal(context.offlineEntryShouldMarkReadOnView(
        {dataset: {offlineSnapshot: "true", markAsReadOnView: "true"}},
        "/unread/entry/7",
        mediaEntry,
    ), false);
    assert.equal(context.offlineEntryShouldMarkReadOnView(
        {dataset: {offlineSnapshot: "true", markAsReadOnView: "false"}},
        "/unread/entry/7",
        unreadEntry,
    ), false);
    assert.equal(context.offlineEntryShouldMarkReadOnView(
        {dataset: {offlineSnapshot: "true", markAsReadOnView: "true"}},
        "/unread/entry/7",
        readEntry,
    ), false);
    assert.equal(context.offlineEntryShouldMarkReadOnView(
        {dataset: {offlineSnapshot: "true", markAsReadOnView: "true"}},
        "/unread",
        unreadEntry,
    ), false);
});

test("offline tag patches restore assigned labels", () => {
    let removed = false;
    const assignedLabels = new Map();
    const assigned = {
        querySelector: (selector) => assignedLabels.get(selector.match(/\"(\d+)\"/)?.[1]) || null,
        appendChild: (label) => assignedLabels.set(label.dataset.userTagId, label),
    };
    const checkbox = {checked: false, closest: () => ({textContent: " Research "})};
    const element = {
        querySelector: (selector) => {
            if (selector === ".entry-user-tags-assigned") return assigned;
            if (selector.includes('input[name="user_tag_ids"]')) return checkbox;
            return null;
        },
        querySelectorAll: () => [],
    };
    context.document = {createElement: () => ({dataset: {}, remove: () => { removed = true; }})};
    context.location = {pathname: "/history/entry/1"};

    context.applyOfflinePatchToElement(element, {set: {}, add_user_tag_ids: [7], remove_user_tag_ids: []});
    assert.equal(checkbox.checked, true);
    assert.equal(assignedLabels.get("7").textContent, "Research");

    context.applyOfflinePatchToElement(element, {set: {}, add_user_tag_ids: [], remove_user_tag_ids: [7]});
    assert.equal(checkbox.checked, false);
    assert.equal(removed, true);
});

test("status and saved-for-later conflicts stay coupled", () => {
    const fields = context.conflictPatchFields([{field: "status_saved_for_later"}]);
    assert.deepEqual(Array.from(fields).sort(), ["saved_for_later", "status"]);
});

test("reads saved-for-later false when the marker is absent", () => {
    context.trustedTypes = {createPolicy: (_name, policy) => policy};
    context.DOMParser = class {
        parseFromString(html) {
            return {querySelector: () => ({
                querySelector: (selector) => selector.includes("toggle-status")
                    ? {dataset: {value: "unread"}}
                    : {dataset: html.includes("data-completed") ? {completed: "true"} : {}},
            })};
        }
    };
    assert.deepEqual({...context.offlineStatusValuesFromHTML("<button></button>")}, {status: "unread", saved_for_later: false});
    assert.deepEqual({...context.offlineStatusValuesFromHTML("<button data-completed></button>")}, {status: "unread", saved_for_later: true});
});

test("repairs legacy status-only patches", () => {
    const patch = {base: {status: "unread"}, set: {status: "read"}};
    const current = {status: "unread", saved_for_later: true};
    context.coupleOfflineStatusPatch(patch, current, context.offlineDesiredStatusValues(patch, current));
    assert.deepEqual({...patch.base}, {status: "unread", saved_for_later: true});
    assert.deepEqual({...patch.set}, {status: "read", saved_for_later: false});
    assert.equal(context.isCompleteOfflineStatusPatch(patch), true);
});

test("repairs legacy saved-only patches", () => {
    const patch = {base: {saved_for_later: false}, set: {saved_for_later: true}};
    const current = {status: "read", saved_for_later: false};
    context.coupleOfflineStatusPatch(patch, current, context.offlineDesiredStatusValues(patch, current));
    assert.deepEqual({...patch.base}, {saved_for_later: false, status: "read"});
    assert.deepEqual({...patch.set}, {saved_for_later: true, status: "unread"});
    assert.equal(context.isCompleteOfflineStatusPatch(patch), true);
});

test("does not fake companion values during unrelated edits", () => {
    const patch = {base: {status: "unread"}, set: {status: "read", saved_for_later: undefined}};
    context.coupleOfflineStatusPatch(patch);
    assert.equal(Object.hasOwn(patch.base, "saved_for_later"), false);
    assert.equal(context.isCompleteOfflineStatusPatch(patch), false);
});

test("formats offline article progress", () => {
    assert.equal(context.offlineProgressText(12, 40, "articles cached"), "12/40 articles cached");
});

test("identifies and counts the media caching phase", () => {
    const progress = {hidden: true, textContent: ""};
    const status = {
        dataset: {labelCachingMedia: "Caching media", labelArticlesCached: "articles cached"},
        querySelector: () => progress,
    };
    context.document = {getElementById: () => status};
    vm.runInContext('offlineRefreshPromise = {}; offlineRefreshPhase = "media"; offlineRefreshProgress = {completed: 12, total: 40}', context);
    assert.equal(context.offlineStateLabel(status), "Caching media");
    context.updateOfflineProgress();
    assert.equal(progress.textContent, "12/40 Caching media");
    vm.runInContext("offlineRefreshPromise = null; offlineRefreshPhase = null; offlineRefreshProgress = null", context);
});

test("clears stale progress when a new phase starts", () => {
    const state = {textContent: ""};
    const progress = {hidden: false, textContent: "7972/7972 articles cached"};
    const status = {
        dataset: {labelCachingMedia: "Caching media"},
        querySelector: (selector) => selector === "[data-offline-state]" ? state : progress,
    };
    context.document = {getElementById: () => status};
    vm.runInContext('offlineRefreshPromise = {}; offlineRefreshPhase = "articles"; offlineRefreshProgress = {completed: 7972, total: 7972}', context);

    context.startOfflineRefreshPhase("media");

    assert.equal(state.textContent, "Caching media");
    assert.equal(progress.hidden, true);
    vm.runInContext("offlineRefreshPromise = null; offlineRefreshPhase = null; offlineRefreshProgress = null", context);
});

test("clears completed offline activity immediately", () => {
    const state = {textContent: ""};
    const progress = {hidden: false};
    const status = {
        dataset: {
            labelCachingArticles: "Caching articles",
            labelOnline: "Online",
        },
        hidden: true,
        querySelector: (selector) => selector === "[data-offline-state]" ? state : progress,
    };
    context.document = {getElementById: () => status};
    context.navigator = {onLine: true};

    vm.runInContext('offlineRefreshPromise = {}; offlineRefreshPhase = "articles"; offlineRefreshProgress = {completed: 1, total: 1}', context);
    context.updateOfflineActivity();
    assert.equal(state.textContent, "Caching articles");
    assert.equal(status.dataset.syncing, "true");

    vm.runInContext("offlineRefreshPromise = null; offlineRefreshPhase = null; offlineRefreshProgress = null", context);
    context.updateOfflineActivity();
    assert.equal(state.textContent, "Online");
    assert.equal(status.dataset.syncing, "false");
    assert.equal(progress.hidden, true);
});

test("shows queued change retry failures", () => {
    vm.runInContext("offlineFlushError = true", context);
    assert.equal(context.offlineStateLabel({dataset: {labelRetryingChanges: "Retrying changes"}}), "Retrying changes");
    vm.runInContext("offlineFlushError = false", context);
});

test("groups offline snapshots into bounded requests", () => {
    const batches = context.offlineEntryBatches(Array.from({length: 51}, (_value, index) => index + 1));
    assert.deepEqual(Array.from(batches, (batch) => Array.from(batch)), [
        Array.from({length: 25}, (_value, index) => index + 1),
        Array.from({length: 25}, (_value, index) => index + 26),
        [51],
    ]);
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
    assert.equal(context.offlineEntryNeedsRefresh(2, cached, undefined, {2: "2026-08-19T10:00:00Z"}, "", Date.parse("2026-08-19T11:00:00Z")), false);
});

test("refreshes stale snapshots without repeating completed entries", () => {
    const cached = new Set([1]);
    const current = {1: "entry-v1"};
    assert.equal(context.offlineEntryNeedsRefresh(1, cached, {1: "entry-v1"}, current, "ui-v2"), true);
    assert.equal(context.offlineEntryNeedsRefresh(1, cached, {1: {entry_version: "entry-v1", snapshot_version: "ui-v1"}}, current, "ui-v2"), true);
    assert.equal(context.offlineEntryNeedsRefresh(1, cached, {1: {entry_version: "entry-v1", snapshot_version: "ui-v2"}}, current, "ui-v2"), false);
});

test("keeps article snapshots across JavaScript-only deployments", () => {
    const cached = new Set([1]);
    const current = {1: "entry-v1"};
    const version = {1: {entry_version: "entry-v1", snapshot_version: "1:old-js:light-css:en_US"}};
    assert.equal(context.offlineEntryNeedsRefresh(1, cached, version, current, "1:new-js:light-css:en_US"), false);
    assert.equal(context.offlineEntryNeedsRefresh(1, cached, version, current, "2:new-js:light-css:en_US"), true);
    assert.equal(context.offlineEntryNeedsRefresh(1, cached, version, current, "1:new-js:dark-css:en_US"), true);
    assert.equal(context.offlineEntryNeedsRefresh(1, cached, version, current, "1:new-js:light-css:fr_FR"), true);
});

test("refreshes offline lists only when needed", () => {
    const allowed = new Set([1, 2]);
    assert.equal(context.offlineListNeedsRefresh(allowed, [1, 2], new Set(), true), false);
    assert.equal(context.offlineListNeedsRefresh(allowed, [1], new Set(), true), true);
    assert.equal(context.offlineListNeedsRefresh(allowed, [1, 2], new Set([2]), true), true);
    assert.equal(context.offlineListNeedsRefresh(allowed, [1, 2], new Set(), false), true);
    assert.equal(context.offlineListSnapshotNeedsRefresh(allowed, [1, 2], new Set(), true, "ui-v1", "ui-v2"), true);
    assert.equal(context.offlineListSnapshotNeedsRefresh(allowed, [1, 2], new Set(), true, "ui-v2", "ui-v2"), false);
    assert.equal(context.offlineListSnapshotNeedsRefresh(
        allowed,
        [1, 2],
        new Set(),
        true,
        "1:old-js:light-css:en_US",
        "1:new-js:light-css:en_US",
    ), false);
});
