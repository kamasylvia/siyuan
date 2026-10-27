// https://github.com/siyuan-note/siyuan/pull/8012
export const registerServiceWorker = (
    scriptURL: string,
    options: RegistrationOptions = {
        scope: "/",
        type: "classic",
        updateViaCache: "all",
    },
) => {
    /// #if BROWSER
    if (window.webkit?.messageHandlers || window.JSAndroid || window.JSHarmony ||
        !("serviceWorker" in window.navigator)
        || !("caches" in window)
        || !("fetch" in window)
        || navigator.serviceWorker == null
    ) {
        return;
    }

    // REF https://developer.mozilla.org/en-US/docs/Web/API/ServiceWorkerRegistration
    window.navigator.serviceWorker
        .register(scriptURL, options)
        .then(registration => {
            registration.update();
        }).catch(e => {
        console.debug(`Registration failed with ${e}`);
    });
    /// #endif
    /// #if !BROWSER && !MOBILE
    // 桌面客户端的远程工作空间同样注册 SW 以启用附件离线缓存;
    // http 远程源不是安全上下文,注册会被拒绝,静默降级(文档缓存走 IndexedDB 不受影响)
    import("./hostCapabilities").then(({isRemoteKernel}) => {
        if (!isRemoteKernel() || !("serviceWorker" in window.navigator)) {
            return;
        }
        window.navigator.serviceWorker.register(scriptURL, options).catch(e => {
            console.debug(`Remote service worker registration skipped: ${e}`);
        });
    });
    /// #endif
};
