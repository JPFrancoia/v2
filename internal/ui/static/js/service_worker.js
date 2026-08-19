const OFFLINE_VERSION = 2;
const CACHE_PREFIX = `miniflux-offline-v${OFFLINE_VERSION}`;
const SHELL_CACHE = `${CACHE_PREFIX}-shell`;
const MEDIA_LIMIT = 4000000;
const BASE_PATH = OFFLINE_URL.substring(0, OFFLINE_URL.length - "/offline".length);

function openOfflineDatabase() {
    return new Promise((resolve, reject) => {
        const request = indexedDB.open("miniflux-offline", 1);
        request.onupgradeneeded = () => {
            const db = request.result;
            if (!db.objectStoreNames.contains("patches")) db.createObjectStore("patches", {keyPath: "key"});
            if (!db.objectStoreNames.contains("meta")) db.createObjectStore("meta", {keyPath: "key"});
        };
        request.onsuccess = () => resolve(request.result);
        request.onerror = () => reject(request.error);
    });
}

async function activeUserID() {
    const db = await openOfflineDatabase();
    try {
        return await new Promise((resolve, reject) => {
            const request = db.transaction("meta").objectStore("meta").get("activeUser");
            request.onsuccess = () => resolve(request.result?.value || 0);
            request.onerror = () => reject(request.error);
        });
    } finally {
        db.close();
    }
}

async function setActiveUserID(userID) {
    const db = await openOfflineDatabase();
    try {
        await new Promise((resolve, reject) => {
            const transaction = db.transaction("meta", "readwrite");
            transaction.objectStore("meta").put({key: "activeUser", value: userID});
            transaction.oncomplete = resolve;
            transaction.onerror = () => reject(transaction.error);
        });
    } finally {
        db.close();
    }
}

function pageCacheName(userID) {
    return `${CACHE_PREFIX}-pages-${userID}`;
}

function mediaCacheName(userID) {
    return `${CACHE_PREFIX}-media-${userID}`;
}

self.addEventListener("install", (event) => {
    event.waitUntil(caches.open(SHELL_CACHE).then((cache) => cache.add(new Request(OFFLINE_URL, {cache: "reload"}))));
    self.skipWaiting();
});

self.addEventListener("activate", (event) => {
    event.waitUntil((async () => {
        for (const name of await caches.keys()) {
            if (name.startsWith("miniflux-offline-v") && !name.startsWith(CACHE_PREFIX)) await caches.delete(name);
        }
        await self.clients.claim();
    })());
});

self.addEventListener("message", (event) => {
    if (event.data?.type === "active-user" && Number.isInteger(event.data.userId)) {
        event.waitUntil(setActiveUserID(event.data.userId));
    }
});

async function offlineNavigation(request) {
    try {
        const response = await fetch(request);
        const requestPath = new URL(request.url).pathname;
        const responsePath = new URL(response.url).pathname;
        if (requestPath !== `${BASE_PATH}/` && response.redirected && responsePath === `${BASE_PATH}/`) {
            await setActiveUserID(0);
        }
        return response;
    } catch (error) {
        const userID = await activeUserID();
        if (userID) {
            const cache = await caches.open(pageCacheName(userID));
            const exact = await cache.match(request);
            if (exact) return exact;

            const entryMatch = new URL(request.url).pathname.match(/\/entry\/(\d+)$/);
            if (entryMatch) {
                const canonical = await cache.match(new URL(`${BASE_PATH}/offline/entry/${entryMatch[1]}`, self.location.origin));
                if (canonical) return canonical;
            }
        }
        return (await caches.open(SHELL_CACHE)).match(OFFLINE_URL);
    }
}

async function staticAsset(request) {
    const cache = await caches.open(SHELL_CACHE);
    const cached = await cache.match(request);
    if (cached) return cached;
    const response = await fetch(request);
    if (response.ok) await cache.put(request, response.clone());
    return response;
}

async function responseBelowLimit(response) {
    const declaredSize = parseInt(response.headers.get("Content-Length") || "0", 10);
    if (declaredSize >= MEDIA_LIMIT) return null;
    if (!response.body) {
        const body = await response.arrayBuffer();
        return body.byteLength < MEDIA_LIMIT ? new Response(body, response) : null;
    }

    const reader = response.body.getReader();
    const chunks = [];
    let size = 0;
    while (true) {
        const {done, value} = await reader.read();
        if (done) break;
        size += value.byteLength;
        if (size >= MEDIA_LIMIT) {
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

async function mediaResponse(request) {
    const userID = await activeUserID();
    if (!userID) return fetch(request);
    const cache = await caches.open(mediaCacheName(userID));
    const cached = await cache.match(request);
    if (cached) return cached;

    const response = await fetch(request);
    if (response.ok) {
        const cacheable = await responseBelowLimit(response.clone());
        if (cacheable) await cache.put(request, cacheable);
    }
    return response;
}

self.addEventListener("fetch", (event) => {
    const request = event.request;
    if (request.method !== "GET") return;

    const url = new URL(request.url);
    if (request.mode === "navigate") {
        event.respondWith(offlineNavigation(request));
        return;
    }
    if (url.origin !== self.location.origin) return;
    if (url.pathname.startsWith(`${BASE_PATH}/proxy/`)) {
        if (request.headers.get("X-Miniflux-Offline-Prefetch") === "1") return;
        event.respondWith(mediaResponse(request));
        return;
    }
    if (url.pathname.startsWith(`${BASE_PATH}/stylesheets/`) ||
        url.pathname.startsWith(`${BASE_PATH}/js/`) ||
        url.pathname.startsWith(`${BASE_PATH}/icon/`) ||
        url.pathname === `${BASE_PATH}/favicon.ico`) {
        event.respondWith(staticAsset(request));
    }
});
