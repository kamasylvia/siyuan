// 插件韧性核心纯函数:同一次启动会话内反复出错的插件,达到阈值后记入休眠名单,
// 下次启动跳过加载(设置 - 集市 中显式启用或卸载即解除)。运行时接线在 loader.ts。

export const PLUGIN_SNOOZE_THRESHOLD = 3;

export interface PluginSnoozeState {
    [name: string]: number;
}

// 记一次失败;返回累计次数
export const recordPluginFailure = (state: PluginSnoozeState, name: string): number => {
    state[name] = (state[name] || 0) + 1;
    return state[name];
};

// 达到阈值即应休眠
export const shouldSnooze = (failures: number, threshold: number = PLUGIN_SNOOZE_THRESHOLD): boolean =>
    failures >= threshold;

// 从休眠名单中移除(显式启用/卸载时解除)
export const clearPluginSnooze = (state: PluginSnoozeState, name: string): PluginSnoozeState => {
    const next = {...state};
    delete next[name];
    return next;
};

// 过滤出本轮允许加载的插件数据,返回 [放行列表, 被休眠拦下的名单]
export const filterSnoozed = <T extends {name: string}>(
    items: T[], snooze: PluginSnoozeState): [T[], string[]] => {
    const allowed: T[] = [];
    const blocked: string[] = [];
    items.forEach(item => {
        if (snooze[item.name] !== undefined) {
            blocked.push(item.name);
        } else {
            allowed.push(item);
        }
    });
    return [allowed, blocked];
};
