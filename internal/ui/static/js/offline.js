const OFFLINE_DB_NAME = "miniflux-offline";
const OFFLINE_DB_VERSION = 1;
const OFFLINE_CACHE_VERSION = 2;
const OFFLINE_MEDIA_LIMIT = 4000000;
const OFFLINE_REFRESH_INTERVAL = 15 * 60 * 1000;
const OFFLINE_ENTRY_BATCH_SIZE = 8;
const OFFLINE_MEDIA_BATCH_SIZE = 4;
let offlineFlushPromise = null;
let offlineRefreshPromise = null;
let offlineRefreshProgress = null;
let offlineRefreshPhase = null;
let offlineSkippedMedia = 0;
let offlineMediaStorageFull = false;
let offlineHTMLPolicy = null;
let offlineRetryTimer = null;
let offlineRetryDelay = 5000;
let offlineFlushError = false;
let offlineRefreshRetryTimer = null;
let offlineRefreshRetryDelay = 5000;
let offlineRefreshRetryPending = false;

function trustedOfflineHTML(html) {
    if (!offlineHTMLPolicy) offlineHTMLPolicy = trustedTypes.createPolicy("html", {createHTML: (value) => value});
    return offlineHTMLPolicy.createHTML(html);
}

function offlineUserID() {
    const value = parseInt(document.body.dataset.userId || "0", 10);
    return Number.isInteger(value) && value > 0 ? value : 0;
}

function offlinePatchKey(entryID) {
    return `${offlineUserID()}:${entryID}`;
}

function openOfflineDatabase() {
    return new Promise((resolve, reject) => {
        const request = indexedDB.open(OFFLINE_DB_NAME, OFFLINE_DB_VERSION);
        request.onupgradeneeded = () => {
            const db = request.result;
            if (!db.objectStoreNames.contains("patches")) db.createObjectStore("patches", {keyPath: "key"});
            if (!db.objectStoreNames.contains("meta")) db.createObjectStore("meta", {keyPath: "key"});
        };
        request.onsuccess = () => resolve(request.result);
        request.onerror = () => reject(request.error);
    });
}

async function offlineStoreRequest(storeName, mode, callback) {
    const db = await openOfflineDatabase();
    try {
        return await new Promise((resolve, reject) => {
            const transaction = db.transaction(storeName, mode);
            const store = transaction.objectStore(storeName);
            let result;
            try {
                result = callback(store);
            } catch (error) {
                reject(error);
                return;
            }
            transaction.oncomplete = () => resolve(result);
            transaction.onerror = () => reject(transaction.error);
            transaction.onabort = () => reject(transaction.error);
        });
    } finally {
        db.close();
    }
}

async function getOfflineRecord(storeName, key) {
    const db = await openOfflineDatabase();
    try {
        return await new Promise((resolve, reject) => {
            const request = db.transaction(storeName).objectStore(storeName).get(key);
            request.onsuccess = () => resolve(request.result || null);
            request.onerror = () => reject(request.error);
        });
    } finally {
        db.close();
    }
}

async function getOfflineRecords(storeName) {
    const db = await openOfflineDatabase();
    try {
        return await new Promise((resolve, reject) => {
            const request = db.transaction(storeName).objectStore(storeName).getAll();
            request.onsuccess = () => resolve(request.result || []);
            request.onerror = () => reject(request.error);
        });
    } finally {
        db.close();
    }
}

function putOfflineRecord(storeName, value) {
    return offlineStoreRequest(storeName, "readwrite", (store) => store.put(value));
}

function deleteOfflineRecord(storeName, key) {
    return offlineStoreRequest(storeName, "readwrite", (store) => store.delete(key));
}

function hasOfflinePatchChanges(patch) {
    return Object.keys(patch.set || {}).length > 0 || (patch.add_user_tag_ids || []).length > 0 || (patch.remove_user_tag_ids || []).length > 0;
}

function hasOfflineValue(values, field) {
    return Object.hasOwn(values, field) && values[field] !== undefined && values[field] !== null;
}

function isCompleteOfflineStatusPatch(patch) {
    const complete = ["status", "saved_for_later"].every((field) => hasOfflineValue(patch.base, field) && hasOfflineValue(patch.set, field));
    if (!complete) return false;
    if (patch.set.saved_for_later === true) return patch.set.status === "unread";
    return patch.set.status !== "read" || patch.set.saved_for_later === false;
}

function coupleOfflineStatusPatch(patch, baseValues = {}, setValues = {}) {
    if (!hasOfflineValue(patch.set, "status") && !hasOfflineValue(patch.set, "saved_for_later")) return;
    ["status", "saved_for_later"].forEach((field) => {
        if (!hasOfflineValue(patch.base, field) && hasOfflineValue(baseValues, field)) patch.base[field] = baseValues[field];
        if (!hasOfflineValue(patch.set, field)) {
            if (hasOfflineValue(setValues, field)) patch.set[field] = setValues[field];
            else if (hasOfflineValue(baseValues, field)) patch.set[field] = baseValues[field];
        }
    });
    if (patch.set.saved_for_later === true) patch.set.status = "unread";
    else if (patch.set.status === "read") patch.set.saved_for_later = false;
}

function mergeOfflineTagDelta(patch, addTagIDs, removeTagIDs) {
    const additions = new Set(patch.add_user_tag_ids || []);
    const removals = new Set(patch.remove_user_tag_ids || []);
    addTagIDs.forEach((tagID) => {
        removals.delete(tagID);
        additions.add(tagID);
    });
    removeTagIDs.forEach((tagID) => {
        additions.delete(tagID);
        removals.add(tagID);
    });
    patch.add_user_tag_ids = Array.from(additions);
    patch.remove_user_tag_ids = Array.from(removals);
}

async function queueOfflineEntryPatch(entryID, baseValues = {}, setValues = {}, addTagIDs = [], removeTagIDs = []) {
    const userID = offlineUserID();
    if (!userID) throw new Error("Offline synchronization requires an authenticated user");

    const key = offlinePatchKey(entryID);
    const db = await openOfflineDatabase();
    let patch;
    try {
        await new Promise((resolve, reject) => {
            const transaction = db.transaction("patches", "readwrite");
            const store = transaction.objectStore("patches");
            const request = store.get(key);
            request.onsuccess = () => {
                patch = request.result || {
                    key,
                    userId: userID,
                    entry_id: entryID,
                    base: {},
                    set: {},
                    add_user_tag_ids: [],
                    remove_user_tag_ids: [],
                    revision: 0,
                };
                Object.keys(setValues).forEach((field) => {
                    if (!Object.hasOwn(patch.set, field)) patch.base[field] = baseValues[field];
                    patch.set[field] = setValues[field];
                    if (patch.set[field] === patch.base[field]) {
                        delete patch.set[field];
                        delete patch.base[field];
                    }
                });
                coupleOfflineStatusPatch(patch, baseValues, setValues);
                mergeOfflineTagDelta(patch, addTagIDs, removeTagIDs);
                patch.revision += 1;
                patch.blocked = false;
                patch.conflicts = [];
                patch.updatedAt = Date.now();
                if (hasOfflinePatchChanges(patch)) store.put(patch);
                else store.delete(key);
            };
            request.onerror = () => reject(request.error);
            transaction.oncomplete = resolve;
            transaction.onerror = () => reject(transaction.error);
            transaction.onabort = () => reject(transaction.error);
        });
    } finally {
        db.close();
    }
    await updateOfflineStatus();
    flushOfflineChanges();
    return patch;
}

function offlinePatchPayload(patch) {
    return {
        entry_id: patch.entry_id,
        base: patch.base,
        set: patch.set,
        add_user_tag_ids: patch.add_user_tag_ids,
        remove_user_tag_ids: patch.remove_user_tag_ids,
    };
}

function conflictPatchFields(conflicts) {
    const fields = new Set();
    conflicts.forEach((conflict) => {
        if (conflict.field === "status_saved_for_later") {
            fields.add("status");
            fields.add("saved_for_later");
        } else {
            fields.add(conflict.field);
        }
    });
    return fields;
}

async function acknowledgeOfflinePatch(submitted, result) {
    const db = await openOfflineDatabase();
    try {
        await new Promise((resolve, reject) => {
            const transaction = db.transaction("patches", "readwrite");
            const store = transaction.objectStore("patches");
            const request = store.get(submitted.key);
            request.onsuccess = () => {
                const current = request.result;
                if (!current) return;

                if (result.result === "conflict") {
                    const conflictFields = conflictPatchFields(result.conflicts || []);
                    Object.keys(submitted.set).forEach((field) => {
                        if (!conflictFields.has(field) && current.revision === submitted.revision) {
                            delete current.base[field];
                            delete current.set[field];
                        } else if (!conflictFields.has(field) && result.state) {
                            current.base[field] = result.state[field];
                        }
                    });
                    if (current.revision === submitted.revision) {
                        current.add_user_tag_ids = [];
                        current.remove_user_tag_ids = [];
                    }
                    current.conflicts = result.conflicts || [];
                    current.blocked = current.conflicts.length > 0;
                    current.serverState = result.state || null;
                    if (hasOfflinePatchChanges(current)) store.put(current);
                    else store.delete(current.key);
                    return;
                }

                if (current.revision === submitted.revision) {
                    store.delete(current.key);
                    return;
                }
                if (result.state) {
                    Object.keys(submitted.set).forEach((field) => {
                        current.base[field] = result.state[field];
                    });
                    store.put(current);
                }
            };
            request.onerror = () => reject(request.error);
            transaction.oncomplete = resolve;
            transaction.onerror = () => reject(transaction.error);
            transaction.onabort = () => reject(transaction.error);
        });
    } finally {
        db.close();
    }
}

function scheduleOfflineRetry() {
    if (offlineRetryTimer || navigator.onLine === false) return;
    offlineRetryTimer = setTimeout(() => {
        offlineRetryTimer = null;
        flushOfflineChanges();
    }, offlineRetryDelay);
    offlineRetryDelay = Math.min(offlineRetryDelay * 2, 300000);
}

function offlineStatusValuesFromHTML(html) {
    const parsed = new DOMParser().parseFromString(trustedOfflineHTML(html), "text/html");
    const entry = parsed.querySelector("[data-id]");
    const status = entry?.querySelector(":is(a, button)[data-toggle-status]")?.dataset.value;
    const saved = entry?.querySelector(":is(a, button)[data-save-for-later-entry]");
    if (!status || !saved) return null;
    return {status, saved_for_later: saved.dataset.completed === "true"};
}

function offlineDesiredStatusValues(patch, currentValues) {
    const requestedStatus = hasOfflineValue(patch.set, "status");
    const requestedSaved = hasOfflineValue(patch.set, "saved_for_later");
    const desired = {...currentValues, ...patch.set};
    if (requestedStatus && !requestedSaved) desired.saved_for_later = patch.set.status === "read" ? false : currentValues.saved_for_later;
    if (requestedSaved && !requestedStatus) desired.status = patch.set.saved_for_later ? "unread" : currentValues.status;
    return desired;
}

async function repairOfflineStatusPatch(patch) {
    if (isCompleteOfflineStatusPatch(patch) ||
        (!hasOfflineValue(patch.set, "status") && !hasOfflineValue(patch.set, "saved_for_later"))) return true;
    const entryURL = `${document.body.dataset.offlineEntryUrl}/${patch.entry_id}`;
    const pageCache = await caches.open(offlinePageCacheName(patch.userId));
    let response = await pageCache.match(entryURL);
    if (!response) response = await fetch(entryURL, {credentials: "same-origin", headers: {"Accept": "text/html"}});
    if (response?.status === 404) {
        await deleteOfflineRecord("patches", patch.key);
        return false;
    }
    if (!response?.ok || response.redirected) throw new Error(`Unable to repair offline patch for entry ${patch.entry_id}`);
    const values = offlineStatusValuesFromHTML(await response.text());
    if (!values) throw new Error(`Unable to read offline status for entry ${patch.entry_id}`);
    coupleOfflineStatusPatch(patch, values, offlineDesiredStatusValues(patch, values));
    if (!isCompleteOfflineStatusPatch(patch)) throw new Error(`Unable to repair coupled values for entry ${patch.entry_id}`);
    await putOfflineRecord("patches", patch);
    return true;
}

async function flushOfflineChanges() {
    if (offlineFlushPromise) return offlineFlushPromise;
    const syncURL = document.body.dataset.offlineSyncUrl;
    const userID = offlineUserID();
    if (!syncURL || !userID) return null;

    let flushSucceeded = false;
    let repairFailed = false;
    offlineFlushPromise = (async () => {
        const queuedPatches = (await getOfflineRecords("patches"))
            .filter((patch) => patch.userId === userID && !patch.blocked)
            .slice(0, 100);
        if (queuedPatches.length === 0) {
            offlineFlushError = false;
            return;
        }
        const patches = [];
        let repairError = null;
        for (const patch of queuedPatches) {
            try {
                if (await repairOfflineStatusPatch(patch)) patches.push(patch);
            } catch (error) {
                repairError = error;
                console.error("Unable to repair queued offline change:", patch.entry_id, error);
            }
        }
        repairFailed = repairError !== null;
        if (patches.length === 0) {
            if (repairError) throw repairError;
            offlineFlushError = false;
            return;
        }

        const response = await fetch(syncURL, {
            method: "POST",
            credentials: "same-origin",
            headers: {
                "Accept": "application/json",
                "Content-Type": "application/json",
                "X-Csrf-Token": document.body.dataset.csrfToken || "",
            },
            body: JSON.stringify({entries: patches.map(offlinePatchPayload)}),
        });
        if (!response.ok || response.redirected || !response.headers.get("Content-Type")?.includes("application/json")) {
            throw new Error(`Offline synchronization failed with HTTP ${response.status}`);
        }
        const payload = await response.json();
        for (let i = 0; i < patches.length; i += 1) {
            await acknowledgeOfflinePatch(patches[i], payload.entries[i]);
        }
        await putOfflineRecord("meta", {key: `lastSync:${userID}`, value: Date.now()});
        flushSucceeded = true;
        offlineFlushError = repairFailed;
        offlineRetryDelay = 5000;
        if (offlineRetryTimer) clearTimeout(offlineRetryTimer);
        offlineRetryTimer = null;
        if (repairFailed) scheduleOfflineRetry();
        // ponytail: refresh the full bounded cache; add targeted reconciliation if network cost becomes material.
        refreshOfflineContent(true);
    })().catch((error) => {
        offlineFlushError = true;
        console.error("Offline synchronization failed:", error);
        scheduleOfflineRetry();
    }).finally(async () => {
        offlineFlushPromise = null;
        await applyOfflinePatchesToPage();
        await updateOfflineStatus();
        if (flushSucceeded && !repairFailed) {
            const remaining = (await getOfflineRecords("patches")).some((patch) => patch.userId === userID && !patch.blocked);
            if (remaining) flushOfflineChanges();
        }
    });
    updateOfflineStatus();
    return offlineFlushPromise;
}

async function resolveOfflineConflict(entryID, field, useLocal) {
    const key = offlinePatchKey(entryID);
    const db = await openOfflineDatabase();
    let blocked = true;
    let serverState = null;
    try {
        await new Promise((resolve, reject) => {
            const transaction = db.transaction("patches", "readwrite");
            const store = transaction.objectStore("patches");
            const request = store.get(key);
            request.onsuccess = () => {
                const patch = request.result;
                if (!patch) return;
                const conflict = (patch.conflicts || []).find((item) => item.field === field);
                if (!conflict) return;
                serverState = patch.serverState;

                const fields = field === "status_saved_for_later" ? ["status", "saved_for_later"] : [field];
                fields.forEach((name) => {
                    if (useLocal) patch.base[name] = field === "status_saved_for_later" ? conflict.server[name] : conflict.server;
                    else {
                        delete patch.base[name];
                        delete patch.set[name];
                    }
                });
                patch.conflicts = patch.conflicts.filter((item) => item.field !== field);
                patch.blocked = patch.conflicts.length > 0;
                blocked = patch.blocked;
                patch.revision += 1;
                if (hasOfflinePatchChanges(patch)) store.put(patch);
                else store.delete(patch.key);
            };
            request.onerror = () => reject(request.error);
            transaction.oncomplete = resolve;
            transaction.onerror = () => reject(transaction.error);
            transaction.onabort = () => reject(transaction.error);
        });
    } finally {
        db.close();
    }
    if (!useLocal && serverState) {
        document.querySelectorAll(`[data-id="${entryID}"]`).forEach((element) => applyOfflineServerStateToElement(element, serverState));
        refreshOfflineContent(true);
    }
    await updateOfflineStatus();
    if (!blocked) flushOfflineChanges();
}

function offlinePageCacheName(userID) {
    return `miniflux-offline-v${OFFLINE_CACHE_VERSION}-pages-${userID}`;
}

function offlineMediaCacheName(userID) {
    return `miniflux-offline-v${OFFLINE_CACHE_VERSION}-media-${userID}`;
}

async function responseBelowOfflineMediaLimit(response) {
    const declaredSize = parseInt(response.headers.get("Content-Length") || "0", 10);
    if (declaredSize >= OFFLINE_MEDIA_LIMIT) return null;
    if (!response.body) {
        const body = await response.arrayBuffer();
        return body.byteLength < OFFLINE_MEDIA_LIMIT ? new Response(body, response) : null;
    }

    const reader = response.body.getReader();
    const chunks = [];
    let size = 0;
    while (true) {
        const {done, value} = await reader.read();
        if (done) break;
        size += value.byteLength;
        if (size >= OFFLINE_MEDIA_LIMIT) {
            await reader.cancel();
            return null;
        }
        chunks.push(value);
    }
    return new Response(new Blob(chunks), {
        status: response.status,
        statusText: response.statusText,
        headers: response.headers,
    });
}

async function cacheOfflineMedia(url, cache, knownMissing = false) {
    if (!knownMissing && await cache.match(url)) return true;
    if (offlineMediaStorageFull) return false;
    if (navigator.storage?.estimate) {
        const estimate = await navigator.storage.estimate();
        if (estimate.quota && estimate.usage && estimate.quota - estimate.usage < 10000000) {
            offlineMediaStorageFull = true;
            return false;
        }
    }
    try {
        const response = await fetch(url, {
            credentials: "same-origin",
            headers: {"X-Miniflux-Offline-Prefetch": "1"},
        });
        if (!response.ok || response.type === "opaque") return false;
        const cacheable = await responseBelowOfflineMediaLimit(response);
        if (!cacheable) return false;
        await cache.put(url, cacheable);
        return true;
    } catch (error) {
        if (error?.name === "QuotaExceededError") offlineMediaStorageFull = true;
        console.debug("Unable to cache offline media:", url, error);
        return false;
    }
}

function mediaURLsFromDocument(documentNode) {
    const urls = new Set();
    documentNode.querySelectorAll("img[src], audio[src], video[src], source[src], video[poster]").forEach((element) => {
        const value = element.getAttribute("src") || element.getAttribute("poster");
        if (value) urls.add(new URL(value, location.origin).href);
    });
    documentNode.querySelectorAll("img[srcset], source[srcset]").forEach((element) => {
        element.getAttribute("srcset").split(",").forEach((candidate) => {
            const value = candidate.trim().split(/\s+/)[0];
            if (value) urls.add(new URL(value, location.origin).href);
        });
    });
    return Array.from(urls);
}

async function runOfflineBatches(items, batchSize, callback) {
    for (let offset = 0; offset < items.length; offset += batchSize) {
        await Promise.all(items.slice(offset, offset + batchSize).map(callback));
    }
}

async function deleteUnreferencedOfflineMedia(previousURLs, currentURLs, currentKey, mediaCache) {
    const referencedURLs = new Set();
    const mediaPrefix = `entryMedia:${offlineUserID()}:`;
    for (const record of await getOfflineRecords("meta")) {
        if (record.key === currentKey || !record.key.startsWith(mediaPrefix)) continue;
        for (const mediaURL of record.value || []) referencedURLs.add(mediaURL);
    }
    for (const mediaURL of previousURLs) {
        if (!currentURLs.includes(mediaURL) && !referencedURLs.has(mediaURL)) await mediaCache.delete(mediaURL);
    }
}

async function cacheOfflineEntry(entryID, version, snapshotVersion, pageCache, mediaCache) {
    const prefix = document.body.dataset.offlineEntryUrl;
    const userID = offlineUserID();
    if (!prefix || !userID) return false;
    const url = `${prefix}/${entryID}`;
    const response = await fetch(url, {credentials: "same-origin", headers: {"Accept": "text/html"}});
    if (!response.ok || response.redirected) return false;
    const html = await response.clone().text();
    await pageCache.put(url, response);
    const parsed = new DOMParser().parseFromString(trustedOfflineHTML(html), "text/html");
    const mediaURLs = mediaURLsFromDocument(parsed);
    const mediaKey = `entryMedia:${userID}:${entryID}`;
    const previousMedia = await getOfflineRecord("meta", mediaKey);
    if (previousMedia) await deleteUnreferencedOfflineMedia(previousMedia.value || [], mediaURLs, mediaKey, mediaCache);
    await putOfflineRecord("meta", {key: mediaKey, value: mediaURLs});
    await putOfflineRecord("meta", {
        key: `entryVersion:${userID}:${entryID}`,
        value: {entry_version: version, snapshot_version: snapshotVersion},
    });
    return true;
}

function offlineMediaURLs(records, userID, entryIDs) {
    const prefix = `entryMedia:${userID}:`;
    const mediaByEntryID = new Map();
    for (const record of records) {
        if (record.key.startsWith(prefix)) {
            mediaByEntryID.set(parseInt(record.key.substring(prefix.length), 10), record.value || []);
        }
    }
    const urls = new Set();
    for (const entryID of entryIDs) for (const url of mediaByEntryID.get(entryID) || []) urls.add(url);
    return Array.from(urls);
}

function offlineMediaWork(mediaURLs, cachedRequests) {
    const cachedURLs = new Set(cachedRequests.map((request) => request.url));
    const pending = mediaURLs.filter((url) => !cachedURLs.has(url));
    return {pending, completed: mediaURLs.length - pending.length, total: mediaURLs.length};
}

async function removeExpiredOfflineTagPages(pageCache, basePath, allowedPaths, userID) {
    const tagPathPrefix = `${basePath}/user-tag/`;
    for (const request of await pageCache.keys()) {
        const path = new URL(request.url).pathname;
        if (path.startsWith(tagPathPrefix) && path.endsWith("/entries") && !allowedPaths.has(path)) {
            await pageCache.delete(request);
            await deleteOfflineRecord("meta", `listSnapshotVersion:${userID}:${request.url}`);
        }
    }
}

async function cacheOfflineListPages(startURL, allowedEntryIDs, pageCache) {
    const targetPath = new URL(startURL).pathname;
    const previousRequests = (await pageCache.keys()).filter((request) => new URL(request.url).pathname === targetPath);
    const refreshedURLs = new Set();
    const remaining = new Set(allowedEntryIDs);
    let url = startURL;
    for (let page = 0; url && page < 100; page += 1) {
        const response = await fetch(url, {credentials: "same-origin", headers: {"Accept": "text/html"}});
        if (!response.ok || response.redirected) return false;
        const parsed = new DOMParser().parseFromString(trustedOfflineHTML(await response.text()), "text/html");
        parsed.body.dataset.offlineSnapshot = "true";
        parsed.querySelectorAll("article[data-id]").forEach((entry) => {
            const entryID = parseInt(entry.dataset.id, 10);
            if (allowedEntryIDs.has(entryID)) remaining.delete(entryID);
            else entry.remove();
        });
        const next = parsed.querySelector('a[rel="next"]');
        if (remaining.size === 0) parsed.querySelectorAll('a[rel="next"]').forEach((link) => link.remove());
        const headers = new Headers(response.headers);
        headers.delete("Content-Length");
        headers.delete("Content-Encoding");
        await pageCache.put(url, new Response(`<!DOCTYPE html>\n${parsed.documentElement.outerHTML}`, {
            status: response.status,
            statusText: response.statusText,
            headers,
        }));
        refreshedURLs.add(url);
        url = remaining.size > 0 && next ? new URL(next.getAttribute("href"), location.origin).href : "";
    }
    for (const request of previousRequests) if (!refreshedURLs.has(request.url)) await pageCache.delete(request);
    return true;
}

function offlineListNeedsRefresh(allowedEntryIDs, previousEntryIDs, refreshedEntryIDs, hasCachedPage, metadataChanged = false) {
    if (!hasCachedPage || metadataChanged) return true;
    if (previousEntryIDs) {
        const previous = new Set(previousEntryIDs);
        if (previous.size !== allowedEntryIDs.size) return true;
        for (const entryID of allowedEntryIDs) if (!previous.has(entryID)) return true;
    }
    for (const entryID of refreshedEntryIDs) if (allowedEntryIDs.has(entryID)) return true;
    return false;
}

function offlineListSnapshotNeedsRefresh(allowedEntryIDs, previousEntryIDs, refreshedEntryIDs, hasCachedPage, cachedSnapshotVersion, currentSnapshotVersion, metadataChanged = false) {
    return offlineListNeedsRefresh(
        allowedEntryIDs,
        previousEntryIDs,
        refreshedEntryIDs,
        hasCachedPage,
        metadataChanged || !offlineSnapshotVersionsMatch(cachedSnapshotVersion, currentSnapshotVersion),
    );
}

function offlineManifestEntryIDs(manifest) {
    const ids = new Set([
        ...(manifest.unread_entry_ids || []),
        ...(manifest.saved_for_later_entry_ids || []),
        ...(manifest.starred_entry_ids || []),
        ...(manifest.history_entry_ids || []),
    ]);
    (manifest.user_tags || []).forEach((tag) => (tag.entry_ids || []).forEach((id) => ids.add(id)));
    return ids;
}

async function removeExpiredOfflineEntries(pageCache, mediaCache, entryIDs) {
    const prefix = document.body.dataset.offlineEntryUrl;
    const cachedEntryIDs = new Set();
    for (const request of await pageCache.keys()) {
        if (!request.url.startsWith(new URL(prefix, location.origin).href + "/")) continue;
        const entryID = parseInt(request.url.substring(request.url.lastIndexOf("/") + 1), 10);
        if (entryIDs.has(entryID)) cachedEntryIDs.add(entryID);
        else await pageCache.delete(request);
    }

    const userID = offlineUserID();
    const metaRecords = await getOfflineRecords("meta");
    const mediaRecords = metaRecords.filter((record) => record.key.startsWith(`entryMedia:${userID}:`));
    const retainedMediaURLs = new Set();
    for (const record of mediaRecords) {
        const entryID = parseInt(record.key.substring(record.key.lastIndexOf(":") + 1), 10);
        if (entryIDs.has(entryID)) for (const mediaURL of record.value || []) retainedMediaURLs.add(mediaURL);
    }
    for (const record of mediaRecords) {
        const entryID = parseInt(record.key.substring(record.key.lastIndexOf(":") + 1), 10);
        if (entryIDs.has(entryID)) continue;
        for (const mediaURL of record.value || []) {
            if (!retainedMediaURLs.has(mediaURL)) await mediaCache.delete(mediaURL);
        }
        await deleteOfflineRecord("meta", record.key);
    }

    const cachedVersions = {};
    const versionPrefix = `entryVersion:${userID}:`;
    for (const record of metaRecords) {
        if (!record.key.startsWith(versionPrefix)) continue;
        const entryID = parseInt(record.key.substring(versionPrefix.length), 10);
        if (entryIDs.has(entryID)) cachedVersions[entryID] = record.value;
        else await deleteOfflineRecord("meta", record.key);
    }
    return {cachedEntryIDs, cachedVersions};
}

function offlineSnapshotVersionsMatch(cachedVersion, currentVersion) {
    if (cachedVersion === currentVersion) return true;
    const cached = String(cachedVersion || "").split(":");
    const current = String(currentVersion || "").split(":");
    return cached.length === 4 && current.length === 4 && cached[0] === current[0] &&
        cached[2] === current[2] && cached[3] === current[3];
}

function offlineEntryNeedsRefresh(entryID, cachedEntryIDs, cachedVersions, currentVersions, snapshotVersion = "", lastRefresh = 0) {
    if (!cachedEntryIDs.has(entryID)) return true;
    const cachedVersion = cachedVersions?.[entryID];
    const currentVersion = currentVersions?.[entryID];
    if (cachedVersion && typeof cachedVersion === "object") {
        return cachedVersion.entry_version !== currentVersion ||
            !offlineSnapshotVersionsMatch(cachedVersion.snapshot_version, snapshotVersion);
    }
    if (snapshotVersion) return true;
    if (cachedVersion) return cachedVersion !== currentVersion;
    return !currentVersion || !lastRefresh || Date.parse(currentVersion) > lastRefresh;
}

function scheduleOfflineRefreshRetry() {
    offlineRefreshRetryPending = true;
    if (offlineRefreshRetryTimer || navigator.onLine === false) return;
    offlineRefreshRetryTimer = setTimeout(() => {
        offlineRefreshRetryTimer = null;
        refreshOfflineContent(true);
    }, offlineRefreshRetryDelay);
    offlineRefreshRetryDelay = Math.min(offlineRefreshRetryDelay * 2, 300000);
}

function clearOfflineRefreshRetry() {
    if (offlineRefreshRetryTimer) clearTimeout(offlineRefreshRetryTimer);
    offlineRefreshRetryTimer = null;
    offlineRefreshRetryDelay = 5000;
    offlineRefreshRetryPending = false;
}

async function refreshOfflineContent(force = false) {
    if (offlineRefreshPromise) return offlineRefreshPromise;
    const userID = offlineUserID();
    const manifestURL = document.body.dataset.offlineManifestUrl;
    if (!userID || !manifestURL) return null;

    const lastRefresh = await getOfflineRecord("meta", `lastRefresh:${userID}`);
    if (!force && lastRefresh && Date.now() - lastRefresh.value < OFFLINE_REFRESH_INTERVAL) return null;

    offlineRefreshPhase = "manifest";
    offlineRefreshPromise = (async () => {
        offlineSkippedMedia = 0;
        offlineMediaStorageFull = false;
        const response = await fetch(manifestURL, {credentials: "same-origin", headers: {"Accept": "application/json"}});
        if (!response.ok || response.redirected) throw new Error(`Offline manifest failed with HTTP ${response.status}`);
        const manifest = await response.json();
        if (manifest.user_id !== userID) throw new Error("Offline manifest user mismatch");

        const pageCache = await caches.open(offlinePageCacheName(userID));
        const mediaCache = await caches.open(offlineMediaCacheName(userID));
        const entryIDs = offlineManifestEntryIDs(manifest);
        const {cachedEntryIDs, cachedVersions} = await removeExpiredOfflineEntries(pageCache, mediaCache, entryIDs);
        startOfflineRefreshPhase("articles");
        const previousManifest = await getOfflineRecord("meta", `manifest:${userID}`);
        const entriesToRefresh = Array.from(entryIDs).filter((entryID) => offlineEntryNeedsRefresh(
            entryID,
            cachedEntryIDs,
            cachedVersions,
            manifest.entry_versions,
            manifest.snapshot_version,
            lastRefresh?.value,
        ));
        offlineRefreshProgress = {completed: entryIDs.size - entriesToRefresh.length, total: entryIDs.size};
        updateOfflineProgress();

        let refreshFailures = 0;
        const refreshedEntryIDs = [];
        await runOfflineBatches(entriesToRefresh, OFFLINE_ENTRY_BATCH_SIZE, async (entryID) => {
            try {
                if (await cacheOfflineEntry(entryID, manifest.entry_versions?.[entryID], manifest.snapshot_version, pageCache, mediaCache)) {
                    refreshedEntryIDs.push(entryID);
                    cachedEntryIDs.add(entryID);
                    offlineRefreshProgress.completed += 1;
                    updateOfflineProgress();
                } else {
                    refreshFailures += 1;
                }
            } catch (error) {
                refreshFailures += 1;
                console.debug("Unable to cache offline entry:", entryID, error);
            }
        });

        startOfflineRefreshPhase("lists");
        const basePath = document.body.dataset.basePath || "";
        const previous = previousManifest?.value;
        const previousTags = new Map((previous?.user_tags || []).map((tag) => [tag.id, tag]));
        const listSpecs = [
            ["/unread", new Set(manifest.unread_entry_ids || []), previous?.unread_entry_ids],
            ["/saved-for-later", new Set(manifest.saved_for_later_entry_ids || []), previous?.saved_for_later_entry_ids],
            ["/starred", new Set(manifest.starred_entry_ids || []), previous?.starred_entry_ids],
            ["/history", new Set(manifest.history_entry_ids || []), previous?.history_entry_ids],
        ];
        (manifest.user_tags || []).forEach((tag) => {
            const previousTag = previousTags.get(tag.id);
            listSpecs.push([
                `/user-tag/${tag.id}/entries`,
                new Set(tag.entry_ids || []),
                previousTag?.entry_ids,
                Boolean(previousTag && previousTag.title !== tag.title),
            ]);
        });
        const allowedTagPaths = new Set((manifest.user_tags || []).map((tag) => `${basePath}/user-tag/${tag.id}/entries`));
        await removeExpiredOfflineTagPages(pageCache, basePath, allowedTagPaths, userID);
        const refreshedEntryIDSet = new Set(refreshedEntryIDs);
        for (const [path, allowed, previousIDs, metadataChanged] of listSpecs) {
            const listURL = new URL(basePath + path, location.origin).href;
            const hasCachedPage = Boolean(await pageCache.match(listURL));
            const snapshotKey = `listSnapshotVersion:${userID}:${listURL}`;
            const cachedSnapshot = await getOfflineRecord("meta", snapshotKey);
            if (!offlineListSnapshotNeedsRefresh(
                allowed,
                previousIDs,
                refreshedEntryIDSet,
                hasCachedPage,
                cachedSnapshot?.value,
                manifest.snapshot_version,
                metadataChanged,
            )) continue;
            try {
                if (await cacheOfflineListPages(listURL, allowed, pageCache)) {
                    await putOfflineRecord("meta", {key: snapshotKey, value: manifest.snapshot_version});
                } else {
                    refreshFailures += 1;
                }
            } catch (error) {
                refreshFailures += 1;
                console.debug("Unable to cache offline list:", listURL, error);
            }
        }
        startOfflineRefreshPhase("media");
        const mediaURLs = offlineMediaURLs(await getOfflineRecords("meta"), userID, entryIDs);
        const mediaWork = offlineMediaWork(mediaURLs, await mediaCache.keys());
        offlineRefreshProgress = {completed: mediaWork.completed, total: mediaWork.total};
        updateOfflineProgress();
        await runOfflineBatches(mediaWork.pending, OFFLINE_MEDIA_BATCH_SIZE, async (mediaURL) => {
            if (!await cacheOfflineMedia(mediaURL, mediaCache, true)) offlineSkippedMedia += 1;
            offlineRefreshProgress.completed += 1;
            updateOfflineProgress();
        });

        if (refreshFailures === 0) {
            await putOfflineRecord("meta", {key: `manifest:${userID}`, value: manifest});
            await putOfflineRecord("meta", {key: `lastRefresh:${userID}`, value: Date.now()});
            clearOfflineRefreshRetry();
        } else {
            scheduleOfflineRefreshRetry();
        }
        await putOfflineRecord("meta", {key: `mediaSkipped:${userID}`, value: offlineSkippedMedia});
    })().catch((error) => {
        window.offlineSyncLastError = String(error?.stack || error);
        console.error("Offline content refresh failed:", error);
        scheduleOfflineRefreshRetry();
    }).finally(async () => {
        offlineRefreshPromise = null;
        offlineRefreshProgress = null;
        offlineRefreshPhase = null;
        updateOfflineActivity();
        await updateOfflineStatus();
    });
    updateOfflineActivity();
    updateOfflineStatus();
    return offlineRefreshPromise;
}

function applyOfflineServerStateToElement(element, state) {
    const statusButton = element.querySelector(":is(a, button)[data-toggle-status]");
    if (statusButton && state.status) {
        const oldStatus = statusButton.dataset.value;
        setReadStatusButtonState(statusButton, state.status);
        element.classList.replace(`item-status-${oldStatus}`, `item-status-${state.status}`);
        if (oldStatus !== state.status) updateUnreadCounterValue(state.status === "read" ? -1 : 1);
    }
    const savedButton = element.querySelector(":is(a, button)[data-save-for-later-entry]");
    if (savedButton) setSaveForLaterButtonState(savedButton, state.saved_for_later === true);
    const starButton = element.querySelector(":is(a, button)[data-toggle-starred]");
    if (starButton) setStarredButtonState(starButton, state.starred ? "star" : "unstar");
    element.querySelectorAll(":is(a, button)[data-vote-entry]").forEach((button) => {
        button.dataset.currentVote = state.vote;
        button.classList.toggle("vote-active", parseInt(button.dataset.voteValue, 10) === state.vote);
    });
}

function applyOfflinePatchToElement(element, patch) {
    const statusButton = element.querySelector(":is(a, button)[data-toggle-status]");
    if (patch.set.status && statusButton) {
        const oldStatus = statusButton.dataset.value;
        setReadStatusButtonState(statusButton, patch.set.status);
        element.classList.replace(`item-status-${oldStatus}`, `item-status-${patch.set.status}`);
    }
    const savedButton = element.querySelector(":is(a, button)[data-save-for-later-entry]");
    if (Object.hasOwn(patch.set, "saved_for_later") && savedButton) setSaveForLaterButtonState(savedButton, patch.set.saved_for_later);
    const starButton = element.querySelector(":is(a, button)[data-toggle-starred]");
    if (Object.hasOwn(patch.set, "starred") && starButton) setStarredButtonState(starButton, patch.set.starred ? "star" : "unstar");
    if (Object.hasOwn(patch.set, "vote")) {
        element.querySelectorAll(":is(a, button)[data-vote-entry]").forEach((button) => {
            button.dataset.currentVote = patch.set.vote;
            button.classList.toggle("vote-active", parseInt(button.dataset.voteValue, 10) === patch.set.vote);
        });
    }
    (patch.add_user_tag_ids || []).forEach((tagID) => {
        const checkbox = element.querySelector(`input[name="user_tag_ids"][value="${tagID}"]`);
        if (checkbox) checkbox.checked = true;
    });
    (patch.remove_user_tag_ids || []).forEach((tagID) => {
        const checkbox = element.querySelector(`input[name="user_tag_ids"][value="${tagID}"]`);
        if (checkbox) checkbox.checked = false;
    });

    const path = location.pathname;
    if (path.endsWith("/unread") && patch.set.status === "read") element.remove();
    if (path.endsWith("/history") && patch.set.status === "unread") element.remove();
    if (path.endsWith("/starred") && patch.set.starred === false) element.remove();
    if (path.endsWith("/saved-for-later") && patch.set.saved_for_later === false) element.remove();
    const tagMatch = path.match(/\/user-tag\/(\d+)\/entries$/);
    if (tagMatch && (patch.remove_user_tag_ids || []).includes(parseInt(tagMatch[1], 10))) element.remove();
}

function applyOfflineManifestToPage(manifest) {
    const tags = new Map((manifest.user_tags || []).map((tag) => [tag.id, tag]));
    document.querySelectorAll(".entry-user-tags").forEach((container) => {
        const entryID = parseInt(container.closest("[data-id]")?.dataset.id, 10);
        if (!entryID) return;
        const assigned = container.querySelector(".entry-user-tags-assigned");
        assigned?.querySelectorAll("[data-user-tag-id]").forEach((label) => {
            const tag = tags.get(parseInt(label.dataset.userTagId, 10));
            if (tag) label.textContent = tag.title;
            else label.remove();
        });
        const checklist = container.querySelector(".entry-user-tags-checklist");
        if (!checklist) return;
        checklist.querySelectorAll('input[name="user_tag_ids"]').forEach((input) => {
            const tag = tags.get(parseInt(input.value, 10));
            if (!tag) {
                input.closest("li")?.remove();
                return;
            }
            const label = input.closest("label");
            label?.childNodes.forEach((node) => {
                if (node.nodeType === 3) node.remove();
            });
            label?.append(document.createTextNode(` ${tag.title}`));
            if ((tag.entry_ids || []).includes(entryID)) input.checked = true;
        });
        for (const tag of tags.values()) {
            const manifestAssigned = (tag.entry_ids || []).includes(entryID);
            if (manifestAssigned && assigned && !assigned.querySelector(`[data-user-tag-id="${tag.id}"]`)) {
                const assignedLabel = document.createElement("span");
                assignedLabel.className = "entry-user-tag-label";
                assignedLabel.dataset.userTagId = String(tag.id);
                assignedLabel.textContent = tag.title;
                assigned.append(assignedLabel);
            }
            if (checklist.querySelector(`input[value="${tag.id}"]`)) continue;
            const input = document.createElement("input");
            input.type = "checkbox";
            input.name = "user_tag_ids";
            input.value = String(tag.id);
            input.checked = manifestAssigned;
            const label = document.createElement("label");
            label.append(input, document.createTextNode(` ${tag.title}`));
            const item = document.createElement("li");
            item.append(label);
            checklist.append(item);
        }
    });
}

async function applyOfflinePatchesToPage() {
    const userID = offlineUserID();
    if (!userID) return;
    const patches = (await getOfflineRecords("patches")).filter((patch) => patch.userId === userID);
    patches.forEach((patch) => {
        document.querySelectorAll(`[data-id="${patch.entry_id}"]`).forEach((element) => applyOfflinePatchToElement(element, patch));
    });

    const manifest = await getOfflineRecord("meta", `manifest:${userID}`);
    const offlinePage = navigator.onLine === false || document.body.dataset.offlineSnapshot === "true";
    if (manifest && offlinePage) applyOfflineManifestToPage(manifest.value);
    if (manifest && offlinePage && location.pathname.endsWith("/unread")) {
        const allowed = new Set(manifest.value.unread_entry_ids || []);
        document.querySelectorAll("article[data-id]").forEach((element) => {
            if (!allowed.has(parseInt(element.dataset.id, 10))) element.remove();
        });
    }
}

function offlineProgressText(completed, total, label) {
    return `${completed}/${total} ${label}`;
}

function updateOfflineProgress() {
    const status = document.getElementById("offline-sync-status");
    if (!status) return;
    status.dataset.syncing = offlineFlushPromise || offlineRefreshPromise ? "true" : "false";
    const progress = status.querySelector("[data-offline-progress]");
    if (!progress) return;
    progress.hidden = !offlineRefreshProgress;
    if (offlineRefreshProgress) {
        progress.textContent = offlineProgressText(
            offlineRefreshProgress.completed,
            offlineRefreshProgress.total,
            offlineRefreshPhase === "media" ? status.dataset.labelCachingMedia : status.dataset.labelArticlesCached,
        );
    }
}

function offlineStateLabel(status) {
    if (offlineFlushPromise) return status.dataset.labelSendingChanges;
    if (offlineFlushError) return status.dataset.labelRetryingChanges;
    if (offlineRefreshPhase === "articles") return status.dataset.labelCachingArticles;
    if (offlineRefreshPhase === "lists") return status.dataset.labelCachingLists;
    if (offlineRefreshPhase === "media") return status.dataset.labelCachingMedia;
    if (offlineRefreshPromise) return status.dataset.labelSyncing;
    return navigator.onLine === false ? status.dataset.labelOffline : status.dataset.labelOnline;
}

function startOfflineRefreshPhase(phase) {
    offlineRefreshPhase = phase;
    offlineRefreshProgress = null;
    updateOfflineActivity();
}

function updateOfflineActivity() {
    const status = document.getElementById("offline-sync-status");
    if (!status) return;
    status.hidden = false;
    status.querySelector("[data-offline-state]").textContent = offlineStateLabel(status);
    updateOfflineProgress();
}

async function updateOfflineStatus() {
    const status = document.getElementById("offline-sync-status");
    const userID = offlineUserID();
    if (!status || !userID) return;
    const patches = (await getOfflineRecords("patches")).filter((patch) => patch.userId === userID);
    const conflicts = patches.reduce((count, patch) => count + (patch.conflicts || []).length, 0);
    const queued = patches.filter((patch) => !patch.blocked).length;
    const lastSync = await getOfflineRecord("meta", `lastSync:${userID}`);
    const lastRefresh = await getOfflineRecord("meta", `lastRefresh:${userID}`);
    const lastSyncAt = Math.max(lastSync?.value || 0, lastRefresh?.value || 0);
    const skippedMedia = await getOfflineRecord("meta", `mediaSkipped:${userID}`);
    status.hidden = false;
    status.dataset.offline = navigator.onLine === false ? "true" : "false";
    status.querySelector("[data-offline-queued]").textContent = String(queued);
    status.querySelector("[data-offline-conflicts]").textContent = String(conflicts);
    status.querySelector("[data-offline-last-sync]").textContent = lastSyncAt ? new Date(lastSyncAt).toLocaleString() : status.dataset.labelNever;
    status.querySelector("[data-offline-media-skipped]").textContent = String(skippedMedia?.value || 0);
    updateOfflineActivity();
    const review = status.querySelector("[data-offline-review]");
    if (review) review.hidden = conflicts === 0;
}

function formatOfflineConflictValue(value) {
    return typeof value === "object" ? JSON.stringify(value) : String(value);
}

async function showOfflineConflictDialog() {
    const dialog = document.getElementById("offline-conflict-dialog");
    const list = dialog?.querySelector("[data-offline-conflict-list]");
    if (!dialog || !list) return;
    list.replaceChildren();

    const userID = offlineUserID();
    const patches = (await getOfflineRecords("patches")).filter((patch) => patch.userId === userID && patch.blocked);
    patches.forEach((patch) => {
        (patch.conflicts || []).forEach((conflict) => {
            const article = document.createElement("article");
            article.className = "offline-conflict";
            const title = document.createElement("h3");
            title.textContent = `${dialog.dataset.labelEntry} #${patch.entry_id}: ${conflict.field}`;
            const values = document.createElement("p");
            values.textContent = `${dialog.dataset.labelServer}: ${formatOfflineConflictValue(conflict.server)} · ${dialog.dataset.labelPhone}: ${formatOfflineConflictValue(conflict.client)}`;
            const usePhone = document.createElement("button");
            usePhone.type = "button";
            usePhone.className = "button button-primary";
            usePhone.textContent = dialog.dataset.labelUsePhone;
            usePhone.addEventListener("click", async () => {
                await resolveOfflineConflict(patch.entry_id, conflict.field, true);
                await showOfflineConflictDialog();
            });
            const useServer = document.createElement("button");
            useServer.type = "button";
            useServer.className = "button";
            useServer.textContent = dialog.dataset.labelUseServer;
            useServer.addEventListener("click", async () => {
                await resolveOfflineConflict(patch.entry_id, conflict.field, false);
                await showOfflineConflictDialog();
            });
            article.append(title, values, usePhone, document.createTextNode(" "), useServer);
            list.append(article);
        });
    });
    if (list.childElementCount === 0) {
        dialog.close();
        return;
    }
    if (!dialog.open) dialog.showModal();
}

async function clearOfflineData() {
    const userID = offlineUserID();
    if (!userID) return;
    await caches.delete(offlinePageCacheName(userID));
    await caches.delete(offlineMediaCacheName(userID));
    const patches = await getOfflineRecords("patches");
    for (const patch of patches) if (patch.userId === userID) await deleteOfflineRecord("patches", patch.key);
    const meta = await getOfflineRecords("meta");
    for (const record of meta) {
        if (record.key.endsWith(`:${userID}`) || record.key.startsWith(`entryMedia:${userID}:`) ||
            record.key.startsWith(`entryVersion:${userID}:`) || record.key.startsWith(`listSnapshotVersion:${userID}:`) ||
            record.key === "activeUser") {
            await deleteOfflineRecord("meta", record.key);
        }
    }
    await updateOfflineStatus();
}

async function clearPreviousOfflineUser(previousUserID) {
    await caches.delete(offlinePageCacheName(previousUserID));
    await caches.delete(offlineMediaCacheName(previousUserID));
    const meta = await getOfflineRecords("meta");
    for (const record of meta) {
        if (record.key.endsWith(`:${previousUserID}`) || record.key.startsWith(`entryMedia:${previousUserID}:`) ||
            record.key.startsWith(`entryVersion:${previousUserID}:`) || record.key.startsWith(`listSnapshotVersion:${previousUserID}:`)) {
            await deleteOfflineRecord("meta", record.key);
        }
    }
}

async function initializeOfflineSync() {
    const userID = offlineUserID();
    if (!userID || !("indexedDB" in window) || !("caches" in window)) return;
    if (navigator.storage?.persist) navigator.storage.persist().catch(() => false);
    const previousUser = await getOfflineRecord("meta", "activeUser");
    if (previousUser?.value && previousUser.value !== userID) await clearPreviousOfflineUser(previousUser.value);
    await putOfflineRecord("meta", {key: "activeUser", value: userID});
    const notifyWorker = (worker) => worker?.postMessage({type: "active-user", userId: userID});
    if (navigator.serviceWorker) {
        notifyWorker(navigator.serviceWorker.controller);
        navigator.serviceWorker.ready.then((registration) => notifyWorker(registration.active));
    }
    await applyOfflinePatchesToPage();
    await updateOfflineStatus();
    await flushOfflineChanges();
    refreshOfflineContent();

    window.addEventListener("online", () => {
        updateOfflineStatus();
        flushOfflineChanges();
        refreshOfflineContent(offlineRefreshRetryPending);
    });
    window.addEventListener("offline", updateOfflineStatus);
    document.addEventListener("visibilitychange", () => {
        if (!document.hidden) {
            flushOfflineChanges();
            refreshOfflineContent();
        }
    });

    document.querySelector("[data-offline-refresh]")?.addEventListener("click", () => {
        flushOfflineChanges();
        refreshOfflineContent(true);
    });
    document.querySelector("[data-offline-review]")?.addEventListener("click", showOfflineConflictDialog);
    document.querySelector("[data-offline-conflict-close]")?.addEventListener("click", () => document.getElementById("offline-conflict-dialog")?.close());
    document.querySelector("[data-offline-clear]")?.addEventListener("click", async () => {
        const status = document.getElementById("offline-sync-status");
        if (confirm(status?.dataset.labelClearConfirm || "Clear offline data?")) await clearOfflineData();
    });

    document.querySelectorAll('a[href$="/logout"]').forEach((link) => {
        link.addEventListener("click", async (event) => {
            event.preventDefault();
            await clearOfflineData();
            location.href = link.href;
        });
    });
}
