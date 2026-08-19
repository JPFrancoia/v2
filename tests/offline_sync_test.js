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
