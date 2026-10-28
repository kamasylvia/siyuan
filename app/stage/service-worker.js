// REF https://github.com/MicrosoftEdge/Demos/blob/main/pwamp/sw.js

const CACHE_NAME = `siyuan-${new URL(location.href).searchParams.get("v")}`;
const INITIAL_CACHED_RESOURCES = [
    "/favicon.ico",
    "/stage/icon-large.png",
    "/stage/icon.png",
    "/stage/loading-pure.svg",
    "/appearance/fonts/JetBrainsMono-2.304/JetBrainsMono-Regular.woff2",
    "/stage/protyle/js/lute/lute.min.js",
    "/stage/protyle/js/protyle-html.js"
];

self.addEventListener("install", event => {
    self.skipWaiting();
    event.waitUntil((async () => {
        const cache = await caches.open(CACHE_NAME);
        // 分别缓存启动资源，单个请求失败时保留其他资源，并记录失败地址。
        await Promise.all(INITIAL_CACHED_RESOURCES.map(async resource => {
            try {
                await cache.add(resource);
            } catch (error) {
                console.warn("预缓存资源失败：", resource, error);
            }
        }));
    })());
});

self.addEventListener("activate", event => {
    event.waitUntil((async () => {
        const names = await caches.keys();
        await Promise.all(names.map(name => {
            navigator.storage.estimate().then((storageStats) => {
                if (name !== CACHE_NAME || storageStats.usage / storageStats.quota > 0.8) {
                    return caches.delete(name)
                }
            })
        }));
        await clients.claim();
    })());
});

self.addEventListener("fetch", event => {
    const url = new URL(event.request.url);

    // Don't care about other-origin URLs.
    if (url.origin !== location.origin &&
        url.origin !== "https://assets.b3logfile.com"
    ) {
        return;
    }

    // Don't care about anything else than GET.
    if (event.request.method !== 'GET' ||
        event.request.destination === "document"
    ) {
        return;
    }

    // 外链图片(跨域)同样网络优先入离线缓存,断网时已显示过的图片仍可见;opaque 响应可入 Cache API。
    if (event.request.destination === "image") {
        event.respondWith((async () => {
            try {
                const fetchResponse = await fetch(event.request);
                if (fetchResponse.ok || fetchResponse.type === "opaque") {
                    const assetCache = await caches.open("siyuan-offline-assets");
                    await assetCache.put(event.request, fetchResponse.clone());
                }
                return fetchResponse;
            } catch (error) {
                const cachedResponse = await caches.match(event.request, {cacheName: "siyuan-offline-assets"});
                if (cachedResponse) {
                    return cachedResponse;
                }
                throw error;
            }
        })());
        return;
    }

    // 启动页外观的选择和资源由内核校验，始终请求当前版本，避免切换或更新后命中旧缓存。
    if (url.origin === location.origin &&
        (url.pathname === "/api/system/getBootAppearance" ||
            url.pathname.startsWith("/boot-appearance-assets/"))
    ) {
        event.respondWith(fetch(event.request, {cache: "no-store"}));
        return;
    }

    // 附件（图片/PDF 等）走网络优先并落入离线缓存，断网时回退缓存，保证远程空间离线只读时附件可见。
    if (url.origin === location.origin && url.pathname.startsWith("/assets/")) {
        event.respondWith((async () => {
            try {
                const fetchResponse = await fetch(event.request);
                if (fetchResponse.ok) {
                    const assetCache = await caches.open("siyuan-offline-assets");
                    await assetCache.put(event.request, fetchResponse.clone());
                }
                return fetchResponse;
            } catch (error) {
                const cachedResponse = await caches.match(event.request, {cacheName: "siyuan-offline-assets"});
                if (cachedResponse) {
                    return cachedResponse;
                }
                throw error;
            }
        })());
        return;
    }

    // Don't care about other requests.
    if (!url.pathname.startsWith("/stage/") &&
        !url.pathname.startsWith("/appearance/boot/") &&
        !url.pathname.startsWith("/appearance/fonts/") &&
        !url.pathname.startsWith("/appearance/emojis/") &&
        !url.pathname.startsWith("/appearance/langs/") &&
        !url.href.startsWith("https://assets.b3logfile.com/") &&
        url.pathname !== "/favicon.ico"
    ) {
        return;
    }

    if (url.origin === location.origin && url.pathname.startsWith("/appearance/langs/")) {
        event.respondWith(fetch(event.request, {cache: "no-store"}));
        return;
    }

    // On fetch, go to the cache first, and then network.
    event.respondWith((async () => {
        const cache = await caches.open(CACHE_NAME);
        const cachedResponse = await cache.match(event.request);
        if (cachedResponse && cachedResponse.type !== 'opaque') {
            return cachedResponse;
        } else {
            const fetchResponse = await fetch(event.request);
            cache.put(event.request, fetchResponse.clone());
            return fetchResponse;
        }
    })());
});
