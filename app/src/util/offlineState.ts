// 远程空间离线状态运行时接线:汇聚浏览器网络事件与主连接 WebSocket 信号,
// 经 offlineCore 宽限防抖后广播离线/恢复;订阅方:离线横幅、只读锁、transaction 拦截、getDoc 缓存兜底。
// 仅在远程工作空间初始化;本地工作空间恒在线。

import {nextOffline, OFFLINE_GRACE_MS, OfflineSignals, wantOffline} from "./offlineCore";

let signals: OfflineSignals = {navOnline: true, wsConnected: true};
let signalsChangedAt = Date.now();
let offline = false;
let enabled = false;
let graceTimer: ReturnType<typeof setTimeout>;
const listeners = new Set<(offline: boolean) => void>();

const scheduleGraceRecheck = () => {
    clearTimeout(graceTimer);
    const wait = Math.max(100, signalsChangedAt + OFFLINE_GRACE_MS - Date.now());
    graceTimer = setTimeout(evaluate, wait);
};

const evaluate = () => {
    if (!enabled) {
        return;
    }
    const next = nextOffline(offline, signals, signalsChangedAt, Date.now());
    if (next !== offline) {
        offline = next;
        listeners.forEach(callback => callback(offline));
    }
    if (!offline && wantOffline(signals)) {
        // 信号仍指向离线但尚未过宽限期,到点复查一次
        scheduleGraceRecheck();
    }
};

const updateSignal = (patch: Partial<OfflineSignals>) => {
    if (!enabled) {
        return;
    }
    const before = wantOffline(signals);
    signals = {...signals, ...patch};
    if (wantOffline(signals) !== before) {
        signalsChangedAt = Date.now();
    }
    evaluate();
};

// 仅远程工作空间调用;重复调用幂等
export const initOfflineState = () => {
    if (enabled) {
        return;
    }
    enabled = true;
    signals = {navOnline: navigator.onLine, wsConnected: false};
    signalsChangedAt = Date.now();
    window.addEventListener("online", () => updateSignal({navOnline: true}));
    window.addEventListener("offline", () => updateSignal({navOnline: false}));
    evaluate();
};

export const setWsConnected = (connected: boolean) => updateSignal({wsConnected: connected});

export const isOffline = () => offline;

export const onOfflineChange = (callback: (offline: boolean) => void) => {
    listeners.add(callback);
    return () => listeners.delete(callback);
};
