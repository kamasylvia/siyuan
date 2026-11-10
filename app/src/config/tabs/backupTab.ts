import {fetchSyncPost} from "../../util/fetch";
import {confirmDialog} from "../../dialog/confirmDialog";
import {showMessage} from "../../dialog/message";
import {escapeHtmlTextAndAttr} from "../../util/escape";
import type {SettingTabBuilder} from "../setting/builder";

type BackupType = "pg" | "s3" | "webdav";

type Retention = {keepLast?: number; maxAgeDays?: number};
type PGConfig = {host: string; port?: number; user?: string; password?: string; database: string; sslMode?: string; schema?: string; dsn?: string};
type S3Config = {endpoint: string; accessKey?: string; secretKey?: string; bucket: string; region?: string;
    pathStyle?: boolean; skipTlsVerify?: boolean; timeout?: number};
type WebDAVConfig = {endpoint: string; username?: string; password?: string; skipTlsVerify?: boolean; timeout?: number};

type BackupTarget = {
    id: string;
    name?: string;
    type: BackupType;
    enabled: boolean;
    pg?: PGConfig;
    s3?: S3Config;
    webdav?: WebDAVConfig;
    retention?: Retention;
    schedule?: {cron: string};
};

type BackupConfig = {
    enabled: boolean;
    include: {data: boolean; conf: boolean; history: boolean; localStorage: boolean};
    targets: BackupTarget[];
    encryption?: {enabled: boolean; passphrase?: string};
};

type TargetState = {
    lastSnapshot?: string;
    lastOk: boolean;
    lastError?: string;
    lastRun?: string;
    uploadedBlobs?: number;
    uploadedBytes?: number;
};

type BackupState = {updatedAt?: string; targets: Record<string, TargetState>};

type BackupData = {config: BackupConfig; state?: BackupState; progress?: {
    backup?: {startedAt: number; target: string; done: number; total: number};
    restore?: {startedAt: number; target: string; done: number; total: number};
}};

type DeleteCheckResult = {target: string; deletable: boolean; reason?: string; snapshots: number; duplicate: boolean};

const text = (key: string, fallback: string): string =>
    (window.siyuan.languages as unknown as Record<string, string>)[key] || fallback;

const typeLabel = (type: BackupType): string => {
    switch (type) {
        case "pg": return "PostgreSQL";
        case "s3": return "S3";
        case "webdav": return "WebDAV";
    }
};

const newTargetID = (): string =>
    "backup-" + Date.now().toString(36) + "-" + Math.random().toString(36).slice(2, 8);

const defaultTarget = (): BackupTarget => ({
    id: "",
    name: "",
    type: "pg",
    enabled: true,
    retention: {keepLast: 30, maxAgeDays: 90},
});

const defaultConfig = (): BackupConfig => ({
    enabled: true,
    include: {data: true, conf: true, history: false, localStorage: false},
    targets: [],
    encryption: {enabled: false, passphrase: ""},
});

let currentConfig: BackupConfig = defaultConfig();
let currentState: BackupState = {targets: {}};
let editingID: string | null = null;
let draftID = "";
let draft: BackupTarget = defaultTarget();
// 连接测试门禁:记录通过测试时的连接指纹;连接字段一变即失效,未通过前不可保存
let testedConnectionKey: string | null = null;

const connectionKey = (target: BackupTarget): string =>
    JSON.stringify({type: target.type, pg: target.pg, s3: target.s3, webdav: target.webdav});
let panelRoot: HTMLElement | null = null;

const field = (id: string, label: string, value: string, type = "text", attrs = ""): string =>
    `<label class="fn__flex b3-label" style="align-items:center;margin:0 0 6px 0">
        <span class="fn__flex-1">${escapeHtmlTextAndAttr(label)}</span>
        <input id="${id}" class="b3-text-field fn__flex-1" type="${type}" value="${escapeHtmlTextAndAttr(value)}" ${attrs}>
    </label>`;

const checkbox = (id: string, label: string, checked: boolean): string =>
    `<label class="fn__flex b3-label" style="align-items:center;margin:0 0 6px 0">
        <span class="fn__flex-1">${escapeHtmlTextAndAttr(label)}</span>
        <input id="${id}" type="checkbox" class="b3-switch" ${checked ? "checked" : ""}>
    </label>`;

const selectType = (value: BackupType): string =>
    `<label class="fn__flex b3-label" style="align-items:center;margin:0 0 6px 0">
        <span class="fn__flex-1">${escapeHtmlTextAndAttr(text("backupType", "Type"))}</span>
        <select id="backupTargetType" class="b3-select fn__flex-1">
            ${(["pg", "s3", "webdav"] as BackupType[]).map(type =>
                `<option value="${type}" ${type === value ? "selected" : ""}>${typeLabel(type)}</option>`).join("")}
        </select>
    </label>`;

const typeFields = (target: BackupTarget): string => {
    switch (target.type) {
        case "pg":
            return field("backupPGHost", "Host", target.pg?.host || "") +
                field("backupPGPort", "Port", String(target.pg?.port || 5432), "number") +
                field("backupPGUser", text("backupUsername", "Username"), target.pg?.user || "") +
                field("backupPGPassword", text("backupPassword", "Password"), target.pg?.password || "", "password") +
                field("backupPGDatabase", "Database", target.pg?.database || "") +
                `<label class="fn__flex b3-label" style="align-items:center;margin:0 0 6px 0">
                    <span class="fn__flex-1">${escapeHtmlTextAndAttr(text("backupSSLMode", "SSL mode"))}</span>
                    <select id="backupPGSSLMode" class="b3-select fn__flex-1">
                        ${["disable", "prefer", "require"].map(mode => `<option value="${mode}" ${(target.pg?.sslMode || "disable") === mode ? "selected" : ""}>${mode}</option>`).join("")}
                    </select>
                </label>`;
        case "s3":
            return field("backupS3Endpoint", text("backupEndpoint", "Endpoint"), target.s3?.endpoint || "") +
                field("backupS3AccessKey", text("backupAccessKey", "Access key"), target.s3?.accessKey || "") +
                field("backupS3SecretKey", text("backupSecretKey", "Secret key"), target.s3?.secretKey || "", "password") +
                field("backupS3Bucket", text("backupBucket", "Bucket"), target.s3?.bucket || "") +
                field("backupS3Region", text("backupRegion", "Region"), target.s3?.region || "") +
                field("backupS3Timeout", text("backupTimeout", "Timeout (s)"), String(target.s3?.timeout || 60), "number") +
                checkbox("backupS3PathStyle", text("backupPathStyle", "Path style"), target.s3?.pathStyle !== false) +
                checkbox("backupS3SkipTLS", text("backupSkipTLSVerify", "Skip TLS verify"), target.s3?.skipTlsVerify === true);
        case "webdav":
            return field("backupWebDAVEndpoint", text("backupEndpoint", "Endpoint"), target.webdav?.endpoint || "") +
                field("backupWebDAVUsername", text("backupUsername", "Username"), target.webdav?.username || "") +
                field("backupWebDAVPassword", text("backupPassword", "Password"), target.webdav?.password || "", "password") +
                field("backupWebDAVTimeout", text("backupTimeout", "Timeout (s)"), String(target.webdav?.timeout || 60), "number") +
                checkbox("backupWebDAVSkipTLS", text("backupSkipTLSVerify", "Skip TLS verify"), target.webdav?.skipTlsVerify === true);
    }
};

const render = () => {
    if (!panelRoot) {
        return;
    }
    const targets = currentConfig.targets || [];
    panelRoot.innerHTML = `<div class="b3-label">
    <div class="fn__flex" style="align-items:center;gap:8px;margin-bottom:10px">
        <button class="b3-button b3-button--outline" id="backupAdd">${escapeHtmlTextAndAttr(text("backupAddTarget", "Add target"))}</button>
        <button class="b3-button b3-button--outline" id="backupRun">${escapeHtmlTextAndAttr(text("backupRunNow", "Back up"))}</button>
        <button class="b3-button b3-button--outline" id="backupDelete">${escapeHtmlTextAndAttr(window.siyuan.languages.delete)}</button>
        <span class="fn__space"></span>
        <label class="fn__flex" style="align-items:center;gap:4px"><input id="backupIncludeData" type="checkbox" class="b3-switch" ${currentConfig.include?.data ? "checked" : ""}>${escapeHtmlTextAndAttr(text("backupIncludeData", "Data"))}</label>
        <label class="fn__flex" style="align-items:center;gap:4px"><input id="backupIncludeConf" type="checkbox" class="b3-switch" ${currentConfig.include?.conf ? "checked" : ""}>${escapeHtmlTextAndAttr(text("backupIncludeConf", "Configuration"))}</label>
        <label class="fn__flex" style="align-items:center;gap:4px"><input id="backupIncludeHistory" type="checkbox" class="b3-switch" ${currentConfig.include?.history ? "checked" : ""}>${escapeHtmlTextAndAttr(text("backupIncludeHistory", "History"))}</label>
    </div>
    <div class="fn__flex" style="align-items:center;gap:8px;margin-bottom:10px">
        <label class="fn__flex" style="align-items:center;gap:4px"><input id="backupEncryptionEnabled" type="checkbox" class="b3-switch" ${currentConfig.encryption?.enabled ? "checked" : ""}>${escapeHtmlTextAndAttr(text("backupEncryption", "Encryption"))}</label>
        <input id="backupEncryptionPassphrase" class="b3-text-field fn__flex-1" type="password" placeholder="${escapeHtmlTextAndAttr(text("backupPassphrase", "Passphrase"))}" value="${escapeHtmlTextAndAttr(currentConfig.encryption?.passphrase || "")}">
    </div>
    <div id="backupRunProgress" class="ft__on-surface fn__none" style="margin-bottom:10px;font-size:12px"></div>
    <div id="backupTargetList">${targets.length ? targets.map(targetRowHTML).join("") :
        `<div class="b3-label__text">${escapeHtmlTextAndAttr(text("backupNoTargets", "No backup targets configured"))}</div>`}</div>
    <div id="backupForm" class="fn__none" style="margin-top:12px;border-top:1px solid var(--b3-border-color);padding-top:10px">
        <div class="b3-label__text" id="backupFormTitle"></div>
        <div style="margin-top:8px">
            ${field("backupTargetNote", text("backupTargetNote", "Note (optional)"), "")}
            ${selectType("pg")}
            ${field("backupKeepLast", text("backupKeepLast", "Keep last"), "30", "number")}
            ${field("backupMaxAgeDays", text("backupMaxAgeDays", "Max age (days)"), "90", "number", 'placeholder="0 = unlimited"')}
            ${field("backupScheduleCron", text("backupScheduleCron", "Scheduled backup (cron)"), "", "text", 'placeholder="0 2 * * *"')}
            <div class="ft__on-surface" style="margin:-4px 0 6px;font-size:12px">${escapeHtmlTextAndAttr(text("backupScheduleCronTip",
                "Standard 5-field cron; leave empty to disable. Example: 0 2 * * * = daily at 02:00. Runs on the kernel that stays up, such as the Docker container."))}</div>
            <div id="backupTypeFields"></div>
            <div class="fn__flex" style="gap:8px;margin-top:8px">
                <button class="b3-button b3-button--outline" id="backupSave">${escapeHtmlTextAndAttr(window.siyuan.languages.save)}</button>
                <button class="b3-button b3-button--outline" id="backupTest">${escapeHtmlTextAndAttr(text("backupTestTarget", "Test connection"))}</button>
                <button class="b3-button b3-button--outline" id="backupCancel">${escapeHtmlTextAndAttr(window.siyuan.languages.cancel)}</button>
            </div>
            <div id="backupTestHint" class="ft__on-surface" style="margin:-2px 0 6px;font-size:12px"></div>
        </div>
    </div>
</div>`;
    bind();
};

const targetLabel = (target: BackupTarget): string => {
    if (target.name) {
        return target.name;
    }
    switch (target.type) {
        case "pg":
            return `PostgreSQL · ${target.pg?.host || ""}/${target.pg?.database || ""}`;
        case "s3":
            return `S3 · ${target.s3?.bucket || ""}`;
        case "webdav":
            return `WebDAV · ${target.webdav?.endpoint || ""}`;
    }
};

const targetRowHTML = (target: BackupTarget): string => {
    const state = currentState.targets?.[target.id];
    const status = state ? `${state.lastOk ? "OK" : "FAIL"} ${state.lastSnapshot || ""} ${state.lastError || ""}` : "";
    return `<div class="fn__flex" data-id="${escapeHtmlTextAndAttr(target.id)}" style="align-items:center;gap:8px;padding:6px 0;border-bottom:1px solid var(--b3-border-color)">
        <input type="checkbox" class="backupTargetSelect" data-id="${escapeHtmlTextAndAttr(target.id)}" checked>
        <span class="fn__flex-1"><b>${escapeHtmlTextAndAttr(targetLabel(target))}</b>
            <span class="ft__on-surface">${typeLabel(target.type)}</span>
            ${status ? `<div class="ft__on-surface">${escapeHtmlTextAndAttr(status)}</div>` : ""}
        </span>
        <button class="b3-button b3-button--outline" data-action="backup">${escapeHtmlTextAndAttr(text("backupRunOne", "Back up"))}</button>
        <button class="b3-button b3-button--outline" data-action="restore">${escapeHtmlTextAndAttr(text("backupRestoreOne", "Restore"))}</button>
        <button class="b3-button b3-button--outline" data-action="snapshots">${escapeHtmlTextAndAttr(text("backupSnapshotsOne", "Snapshots"))}</button>
        <button class="b3-button b3-button--outline" data-action="edit">${escapeHtmlTextAndAttr(window.siyuan.languages.edit)}</button>
        <button class="b3-button b3-button--outline" data-action="test">${escapeHtmlTextAndAttr(text("backupTestTarget", "Test"))}</button>
        <button class="b3-button b3-button--outline" data-action="delete">${escapeHtmlTextAndAttr(window.siyuan.languages.delete)}</button>
    </div>`;
};

const bind = () => {
    if (!panelRoot) {
        return;
    }
    panelRoot.querySelector("#backupAdd")?.addEventListener("click", () => openForm(defaultTarget()));
    panelRoot.querySelector("#backupRun")?.addEventListener("click", () => void runBackup());
    panelRoot.querySelector("#backupDelete")?.addEventListener("click", () => void deleteTargets());
    for (const id of ["backupIncludeData", "backupIncludeConf", "backupIncludeHistory"]) {
        panelRoot.querySelector(`#${id}`)?.addEventListener("change", () => void saveInclude());
    }
    panelRoot.querySelector("#backupEncryptionEnabled")?.addEventListener("change", () => void saveEncryption());
    panelRoot.querySelector("#backupEncryptionPassphrase")?.addEventListener("change", () => void saveEncryption());
    panelRoot.querySelectorAll("#backupTargetList [data-action]").forEach(button => {
        button.addEventListener("click", (event) => {
            const row = (event.currentTarget as HTMLElement).closest("[data-id]") as HTMLElement;
            const id = row?.getAttribute("data-id") || "";
            const action = (event.currentTarget as HTMLElement).getAttribute("data-action");
            if (action === "edit") {
                const target = currentConfig.targets.find(item => item.id === id);
                if (target) {
                    openForm(JSON.parse(JSON.stringify(target)) as BackupTarget);
                }
            } else if (action === "delete") {
                void deleteTarget(id);
            } else if (action === "backup") {
                void runBackup([id]);
            } else if (action === "restore") {
                void openRestoreDialog(id);
            } else if (action === "snapshots") {
                void openSnapshotsDialog(id);
            } else if (action === "test") {
                const target = currentConfig.targets.find(item => item.id === id);
                if (target) {
                    void testTarget(target);
                }
            }
        });
    });
    panelRoot.querySelector("#backupTargetType")?.addEventListener("change", (event) => {
        draft.type = (event.target as HTMLSelectElement).value as BackupType;
        draft.pg = undefined;
        draft.s3 = undefined;
        draft.webdav = undefined;
        if (draft.type === "pg") draft.pg = {host: "", port: 5432, user: "", password: "", database: "", sslMode: "disable"};
        if (draft.type === "s3") draft.s3 = {endpoint: "", bucket: "", region: "", pathStyle: true, timeout: 60};
        if (draft.type === "webdav") draft.webdav = {endpoint: "", username: "", password: "", timeout: 60};
        const container = panelRoot?.querySelector("#backupTypeFields");
        if (container) container.innerHTML = typeFields(draft);
    });
    panelRoot.querySelector("#backupSave")?.addEventListener("click", () => void saveForm());
    panelRoot.querySelector("#backupTest")?.addEventListener("click", () => void testFormTarget());
    panelRoot.querySelector("#backupCancel")?.addEventListener("click", () => closeForm());
    // 连接字段变化使已通过的测试失效:委托到表单容器,类型字段重渲染后依然生效
    const formContainer = panelRoot.querySelector("#backupForm");
    formContainer?.addEventListener("input", () => applyTestGate());
    formContainer?.addEventListener("change", () => applyTestGate());
};

// 测试门禁:保存按钮只在「连接测试已通过且连接字段未再变动」时可用
const applyTestGate = (): void => {
    const saveButton = panelRoot?.querySelector("#backupSave") as HTMLButtonElement | undefined;
    const hint = panelRoot?.querySelector("#backupTestHint") as HTMLElement | undefined;
    if (!saveButton || !hint) {
        return;
    }
    const passed = testedConnectionKey !== null && testedConnectionKey === connectionKey(readForm());
    saveButton.disabled = !passed;
    hint.textContent = passed
        ? text("backupTestPassedHint", "Connection test passed")
        : text("backupSaveNeedsTest", "Run a connection test before saving");
};

// 表单内测试:结果写入门禁状态,行内「测试」按钮走 testTarget,不影响表单门禁
const testFormTarget = async () => {
    const target = readForm();
    const response = await fetchSyncPost("/api/backup/testTarget", {target});
    const data = response.data as {ok?: boolean; message?: string} | undefined;
    if (response.code === 0 && data?.ok) {
        testedConnectionKey = connectionKey(target);
        showMessage(text("backupTestOK", "Target connection OK"), 3000);
    } else {
        testedConnectionKey = null;
        showMessage(`${text("backupTestFailed", "Target test failed")}: ${data?.message || response.msg}`, 7000, "error");
    }
    applyTestGate();
};

const openForm = (target: BackupTarget) => {
    draft = target;
    editingID = target.id || null;
    draftID = target.id || newTargetID();
    const form = panelRoot?.querySelector("#backupForm") as HTMLElement;
    if (!form) {
        return;
    }
    form.classList.remove("fn__none");
    const title = panelRoot?.querySelector("#backupFormTitle");
    if (title) {
        title.textContent = text(editingID ? "backupEditTarget" : "backupAddTarget", editingID ? "Edit target" : "Add target");
    }
    (panelRoot?.querySelector("#backupTargetNote") as HTMLInputElement).value = target.name || "";
    (panelRoot?.querySelector("#backupTargetType") as HTMLSelectElement).value = target.type;
    (panelRoot?.querySelector("#backupKeepLast") as HTMLInputElement).value = String(target.retention?.keepLast ?? 30);
    (panelRoot?.querySelector("#backupMaxAgeDays") as HTMLInputElement).value = String(target.retention?.maxAgeDays ?? 90);
    (panelRoot?.querySelector("#backupScheduleCron") as HTMLInputElement).value = target.schedule?.cron || "";
    const fields = panelRoot?.querySelector("#backupTypeFields");
    if (fields) {
        fields.innerHTML = typeFields(target);
    }
    // 编辑且连接字段未动:视为已通过(存储在保存时已验证过);新增或改动连接必须重新测试
    testedConnectionKey = editingID ? connectionKey(target) : null;
    applyTestGate();
};

const closeForm = () => {
    editingID = null;
    draftID = "";
    draft = defaultTarget();
    panelRoot?.querySelector("#backupForm")?.classList.add("fn__none");
};

const readForm = (): BackupTarget => {
    const value = (id: string): string => (panelRoot?.querySelector(`#${id}`) as HTMLInputElement)?.value || "";
    const checked = (id: string): boolean => (panelRoot?.querySelector(`#${id}`) as HTMLInputElement)?.checked === true;
    const target: BackupTarget = {
        id: editingID || draftID,
        name: value("backupTargetNote").trim(),
        type: (panelRoot?.querySelector("#backupTargetType") as HTMLSelectElement)?.value as BackupType || "pg",
        enabled: true,
        retention: {
            keepLast: Number(value("backupKeepLast")) || 0,
            maxAgeDays: Number(value("backupMaxAgeDays")) || 0,
        },
    };
    // 定时备份:标准 5 段 cron,留空表示不启用;格式由内核保存时校验
    const cron = value("backupScheduleCron").trim();
    if (cron) {
        target.schedule = {cron};
    }
    switch (target.type) {
        case "pg":
            target.pg = {
                host: value("backupPGHost").trim(), port: Number(value("backupPGPort")) || 5432,
                user: value("backupPGUser").trim(), password: value("backupPGPassword"),
                database: value("backupPGDatabase").trim(),
                sslMode: (panelRoot?.querySelector("#backupPGSSLMode") as HTMLSelectElement)?.value || "disable",
            };
            break;
        case "s3":
            target.s3 = {
                endpoint: value("backupS3Endpoint").trim(), accessKey: value("backupS3AccessKey").trim(),
                secretKey: value("backupS3SecretKey"), bucket: value("backupS3Bucket").trim(),
                region: value("backupS3Region").trim(), pathStyle: checked("backupS3PathStyle"),
                skipTlsVerify: checked("backupS3SkipTLS"), timeout: Number(value("backupS3Timeout")) || 60,
            };
            break;
        case "webdav":
            target.webdav = {
                endpoint: value("backupWebDAVEndpoint").trim(), username: value("backupWebDAVUsername").trim(),
                password: value("backupWebDAVPassword"), skipTlsVerify: checked("backupWebDAVSkipTLS"),
                timeout: Number(value("backupWebDAVTimeout")) || 60,
            };
            break;
    }
    return target;
};

const loadPanel = async () => {
    const response = await fetchSyncPost("/api/backup/getConfig", {});
    if (response.code !== 0) {
        showMessage(response.msg, 7000, "error");
        return;
    }
    const data = response.data as BackupData;
    currentConfig = data.config || defaultConfig();
    currentState = data.state || {targets: {}};
    render();
    // 面板打开时备份或还原正在跑:显示运行态(轮询由点击流程接管,这里只展示)
    const progressEl = panelRoot?.querySelector("#backupRunProgress") as HTMLElement;
    const run = data.progress?.backup;
    const restore = data.progress?.restore;
    setBackupBusy(Boolean(run || restore), run?.target || restore?.target);
    if (progressEl && (run || restore)) {
        if (restore) {
            const seconds = Math.max(1, Math.round(Date.now() / 1000) - restore.startedAt);
            progressEl.textContent =
                text("backupProgressRestore", "Restoring: ${done}/${total} · ${seconds}s")
                    .replace("${done}", String(restore.done))
                    .replace("${total}", String(restore.total))
                    .replace("${seconds}", String(seconds));
        } else if (run) {
            const seconds = Math.max(1, Math.round(Date.now() / 1000) - run.startedAt);
            progressEl.textContent =
                text("backupProgressBackup", "Backing up ${target}: ${done}/${total} · ${seconds}s")
                    .replace("${target}", currentConfig.targets.find(item => item.id === run.target)?.name || run.target)
                    .replace("${done}", String(run.done))
                    .replace("${total}", String(run.total))
                    .replace("${seconds}", String(seconds));
        }
        progressEl.classList.remove("fn__none");
    }
    // 备份或还原进行中:动作按钮统一灰掉,防止并发操作
    setBackupBusy(Boolean(run || restore));
};

// 备份或还原运行期间,运行中的目标行全部按钮(含编辑、测试连接、快照管理)
// 无豁免禁用,顶部全局按钮同步禁用;其余目标行保持可用。targetID 省略时
// 先禁用全部行,待进度回报运行目标后收窄
const setBackupBusy = (on: boolean, targetID?: string): void => {
    panelRoot?.querySelectorAll("#backupRun, #backupDelete, #backupRestore").forEach(button => {
        (button as HTMLButtonElement).disabled = on;
    });
    panelRoot?.querySelectorAll("#backupTargetList [data-action]").forEach(button => {
        const row = button.closest("[data-id]") as HTMLElement;
        const rowTarget = row?.getAttribute("data-id") || "";
        (button as HTMLButtonElement).disabled = on && (!targetID || rowTarget === targetID);
    });
};

// storageIdentityOf 与内核 storageIdentity 同规则:连接标识相同即视为同一
// 存储,与 IP/域名的写法差异无关
const storageIdentityOf = (target: BackupTarget): string => {
    const endpoint = (value?: string) =>
        (value || "").trim().toLowerCase().replace(/\/+$/, "").replace(/^https?:\/\//, "");
    switch (target.type) {
        case "pg":
            if (target.pg?.dsn?.trim()) {
                return "pg|dsn|" + target.pg.dsn.trim();
            }
            return ["pg", (target.pg?.host || "").trim().toLowerCase(), String(target.pg?.port || 5432),
                (target.pg?.database || "").trim().toLowerCase(), (target.pg?.schema || "public").trim().toLowerCase()].join("|");
        case "s3":
            return "s3|" + endpoint(target.s3?.endpoint) + "|" + (target.s3?.bucket || "").trim().toLowerCase();
        case "webdav":
            return "webdav|" + endpoint(target.webdav?.endpoint);
    }
    return target.type;
};

const saveConfig = async () => {
    currentConfig.enabled = currentConfig.targets.length > 0;
    currentConfig.targets.forEach(target => { target.enabled = true; });
    const response = await fetchSyncPost("/api/backup/setConfig", {config: currentConfig});
    if (response.code !== 0) {
        showMessage(response.msg, 7000, "error");
        return false;
    }
    const data = response.data as BackupData;
    currentConfig = data.config || currentConfig;
    currentState = data.state || currentState;
    render();
    showMessage(text("backupConfigSaved", "Backup configuration saved"), 3000);
    return true;
};

const saveEncryption = async () => {
    currentConfig.encryption = {
        enabled: (panelRoot?.querySelector("#backupEncryptionEnabled") as HTMLInputElement)?.checked === true,
        passphrase: (panelRoot?.querySelector("#backupEncryptionPassphrase") as HTMLInputElement)?.value || "",
    };
    await saveConfig();
};

const saveInclude = async () => {
    currentConfig.include = {
        data: (panelRoot?.querySelector("#backupIncludeData") as HTMLInputElement)?.checked === true,
        conf: (panelRoot?.querySelector("#backupIncludeConf") as HTMLInputElement)?.checked === true,
        history: (panelRoot?.querySelector("#backupIncludeHistory") as HTMLInputElement)?.checked === true,
        localStorage: currentConfig.include?.localStorage === true,
    };
    await saveConfig();
};

const saveForm = async () => {
    const target = readForm();
    // 兜底守卫:按钮禁用之外,拦截任何绕过测试门禁的保存路径
    if (testedConnectionKey !== connectionKey(target)) {
        showMessage(text("backupSaveNeedsTest", "Run a connection test before saving"), 7000, "error");
        return;
    }
    currentConfig.enabled = true;
    // 连接标识唯一性:同一 host/端口/库或桶不允许出现第二条配置
    const identity = storageIdentityOf(target);
    const clash = currentConfig.targets.find(item => item.id !== target.id && storageIdentityOf(item) === identity);
    if (clash) {
        showMessage(text("backupDuplicateTarget", "Backup target \${a} points to the same storage as \${b}; each target must use a unique host, port, database or bucket")
            .replace("\${a}", targetLabel(target) || target.id)
            .replace("\${b}", targetLabel(clash)), 7000, "error");
        return;
    }
    // 编辑模式下连接信息变化时先过存储门禁:同一物理存储或原存储已清空才放行,
    // 防止更换存储后把有快照的存储留在无人认领的状态
    if (editingID) {
        const original = currentConfig.targets.find(item => item.id === editingID);
        const storageOf = (item: BackupTarget): string =>
            JSON.stringify({type: item.type, pg: item.pg, s3: item.s3, webdav: item.webdav});
        if (original && storageOf(original) !== storageOf(target)) {
            const response = await fetchSyncPost("/api/backup/editCheck", {id: editingID, target});
            const result = response.data as {allowed?: boolean; reason?: string; oldSnapshots?: number} | undefined;
            if (response.code !== 0 || !result?.allowed) {
                const note = result?.reason === "snapshots"
                    ? text("backupEditBlockedSnapshots", "Still has ${x} snapshots; empty them in snapshot management or keep pointing at the same storage")
                        .replace("${x}", String(result?.oldSnapshots ?? 0))
                    : text("backupEditBlockedUnreachable", "The original target is unreachable; cannot verify it is emptied");
                showMessage(note, 7000, "error");
                return;
            }
        }
    }
    const index = currentConfig.targets.findIndex(item => item.id === target.id);
    if (index >= 0) {
        currentConfig.targets[index] = target;
    } else {
        currentConfig.targets.push(target);
    }
    if (await saveConfig()) {
        closeForm();
    }
};

const deleteTarget = async (id: string) => {
    if (!window.confirm(text("backupDeleteConfirm", "Delete this backup target?"))) {
        return;
    }
    currentConfig.targets = currentConfig.targets.filter(target => target.id !== id);
    await saveConfig();
};

const testTarget = async (target: BackupTarget) => {
    const response = await fetchSyncPost("/api/backup/testTarget", {target});
    const data = response.data as {ok?: boolean; message?: string} | undefined;
    if (response.code === 0 && data?.ok) {
        showMessage(text("backupTestOK", "Target connection OK"), 3000);
    } else {
        showMessage(`${text("backupTestFailed", "Target test failed")}: ${data?.message || response.msg}`, 7000, "error");
    }
};

const runBackup = async (targetIds?: string[], takeover = false): Promise<void> => {
    if (currentConfig.targets.length > 0 && !currentConfig.enabled) {
        currentConfig.enabled = true;
        if (!await saveConfig()) {
            return;
        }
    }
    // 备份支持批量:无参时备份勾选的目标;行内按钮传单目标单独备份
    const selected = targetIds || Array.from(panelRoot?.querySelectorAll(".backupTargetSelect:checked") || [])
        .map((el: HTMLInputElement) => el.dataset.id).filter(Boolean) as string[];
    if (!selected.length) {
        showMessage(text("backupSelectTargets", "Select at least one backup target"), 6000, "error");
        return;
    }
    // 备份是同步 HTTP,大工作空间要跑数分钟:点击即进入全面板 busy 态并轮询
    // 运行态,防止重复点击与无反馈
    if (panelRoot?.querySelector("#backupRun")?.hasAttribute("disabled")) {
        return;
    }
    setBackupBusy(true);
    const progressEl = panelRoot?.querySelector("#backupRunProgress") as HTMLElement;
    const startedTip = text("backupRunStarted", "Backup started; uploading snapshot");
    if (progressEl) {
        progressEl.classList.remove("fn__none");
        progressEl.textContent = startedTip;
    }
    showMessage(startedTip, 3000);
    const poll = window.setInterval(async () => {
        const response = await fetchSyncPost("/api/backup/getConfig", {});
        if (response.code !== 0) {
            return;
        }
        const run = (response.data as BackupData)?.progress?.backup;
        if (run) {
            setBackupBusy(true, run.target);
            if (progressEl) {
            const seconds = Math.max(1, Math.round(Date.now() / 1000) - run.startedAt);
            progressEl.textContent =
                text("backupProgressBackup", "Backing up ${target}: ${done}/${total} · ${seconds}s")
                    .replace("${target}", currentConfig.targets.find(item => item.id === run.target)?.name || run.target)
                    .replace("${done}", String(run.done))
                    .replace("${total}", String(run.total))
                    .replace("${seconds}", String(seconds));
            }
        }
    }, 2000);
    try {
        const response = await fetchSyncPost("/api/backup/run", {targets: selected, takeover});
        if (response.code !== 0) {
            showMessage(`${text("backupRunFailed", "Backup failed")}: ${response.msg}`, 7000, "error");
            return;
        }
        const result = response.data as {snapshot?: string; targets?: Array<{id: string; ok: boolean; error?: string; claimedBy?: string; claimedHost?: string}>};
        const failed = (result.targets || []).filter(target => !target.ok);
        if (failed.length) {
            // 归属被其它实例占用:确认后接管重试(最后配置的实例生效)
            const claimed = failed.find(target => target.claimedBy);
            if (claimed && !takeover) {
                confirmDialog(text("backupRunFailed", "Backup failed"),
                    `<div class="b3-dialog__content" style="padding:12px">${escapeHtmlTextAndAttr(
                        text("backupTargetClaimed", "Backup target \${target} is already used by instance \${instance} on \${host}. Take over so backups run from this instance?")
                            .replace("\${target}", currentConfig.targets.find(item => item.id === claimed.id)?.name || claimed.id)
                            .replace("\${instance}", claimed.claimedBy || "")
                            .replace("\${host}", claimed.claimedHost || ""))}</div>`,
                    () => {
                        void runBackup([claimed.id], true);
                    });
                return;
            }
            showMessage(`${text("backupRunFailed", "Backup failed")}: ${failed.map(target => target.error).join("; ")}`, 7000, "error");
        } else {
            showMessage(`${text("backupRunOK", "Backup completed")}: ${result.snapshot || ""}`, 4000);
        }
    } finally {
        window.clearInterval(poll);
        setBackupBusy(false);
        if (progressEl) {
            progressEl.classList.add("fn__none");
            progressEl.textContent = "";
        }
        await loadPanel();
    }
};

// 删除是批量配置操作,但有删除门禁:仅确认已清空的目标可删,防止在存储上遗留
// 孤儿快照;同一存储的重复配置(如 IP 与域名两种写法)由内核指纹比对放行
const deleteTargets = async () => {
    const selected = Array.from(panelRoot?.querySelectorAll(".backupTargetSelect:checked") || [])
        .map((el: HTMLInputElement) => el.dataset.id).filter(Boolean) as string[];
    if (!selected.length) {
        showMessage(text("backupSelectTargets", "Select at least one backup target"), 6000, "error");
        return;
    }
    const response = await fetchSyncPost("/api/backup/deleteCheck", {targets: selected});
    const data = response.data as {results?: DeleteCheckResult[]} | undefined;
    if (response.code !== 0 || !data?.results) {
        showMessage(`${text("backupDeleteCheckFailed", "Could not verify the selected targets")}: ${response.msg}`, 7000, "error");
        return;
    }
    const labelOf = (id: string): string => {
        const target = currentConfig.targets.find(item => item.id === id);
        return target ? targetLabel(target) : id;
    };
    const verdictLine = (result: DeleteCheckResult): string => {
        const label = `<b>${escapeHtmlTextAndAttr(labelOf(result.target))}</b>`;
        if (result.deletable) {
            const note = result.duplicate
                ? text("backupDeleteDuplicateEmpty", "Points at the same storage as a reachable, emptied target")
                : text("backupDeleteOK", "Emptied, ready to delete");
            return `<div class="fn__flex" style="align-items:center;gap:6px;padding:3px 0">
                <span class="ft__success">✓</span><span class="fn__flex-1">${label}</span>
                <span class="ft__on-surface">${escapeHtmlTextAndAttr(note)}</span></div>`;
        }
        const note = result.reason === "snapshots"
            ? text("backupDeleteBlockedSnapshots", "Still has ${x} snapshots; empty the target storage manually first")
                .replace("${x}", String(result.snapshots))
            : text("backupDeleteBlockedUnreachable", "Unreachable; cannot verify it is emptied");
        return `<div class="fn__flex" style="align-items:center;gap:6px;padding:3px 0">
            <span class="ft__error">✗</span><span class="fn__flex-1">${label}</span>
            <span class="ft__on-surface">${escapeHtmlTextAndAttr(note)}</span></div>`;
    };
    const deletable = data.results.filter(result => result.deletable);
    if (!deletable.length) {
        showMessage(text("backupDeleteNoneDeletable", "No target can be deleted: a target must be emptied first"), 7000, "error");
        return;
    }
    const stepTitle = text("backupDeleteStepTitle", "Delete backup targets");
    // 第一步确认:展示每个目标的门禁校验结果
    const step1HTML = `<div class="b3-dialog__content" style="padding:12px">
    <div class="ft__on-surface" style="margin-bottom:8px">${escapeHtmlTextAndAttr(text("backupDeleteStep1Body",
        "Only emptied targets can be deleted; targets that still have snapshots or cannot be reached stay locked to prevent orphan snapshots."))}</div>
    ${data.results.map(verdictLine).join("")}
</div>`;
    confirmDialog(stepTitle, step1HTML, async () => {
        // 第二步确认:最终列出将删除的目标,红色确认按钮
        const step2HTML = `<div class="b3-dialog__content" style="padding:12px">
    <div class="ft__on-surface" style="margin-bottom:8px">${escapeHtmlTextAndAttr(text("backupDeleteStep2Body",
        "This removes the backup configuration of the listed targets (no data on the storage is touched). Please confirm these targets are already emptied; this action cannot be undone."))}</div>
    ${deletable.map(verdictLine).join("")}
</div>`;
        confirmDialog(stepTitle, step2HTML, async () => {
            const deletedIds = new Set(deletable.map(result => result.target));
            currentConfig.targets = currentConfig.targets.filter(target => !deletedIds.has(target.id));
            if (await saveConfig()) {
                showMessage(text("backupDeleteDone", "Deleted ${x} backup target(s)")
                    .replace("${x}", String(deletable.length)), 3000);
                await loadPanel();
            }
        }, undefined, true);
    }, undefined, true);
};

// 快照管理:展示目标上的快照列表,支持删除单个快照(共享数据块按引用计数
// 保留)与清空全部;操作后原位重拉列表,备份或还原进行中由内核拒绝
const openSnapshotsDialog = async (targetId: string) => {
    const target = (currentConfig.targets || []).find(item => item.id === targetId && item.enabled);
    if (!target) {
        showMessage(text("backupRestoreNoTargets", "No enabled backup target to restore from"), 6000, "error");
        return;
    }
    const snapshotRow = (snapshot: {id: string; createdAt: string; files: number; encrypted: boolean}): string =>
        `<div class="fn__flex" data-snapshot="${escapeHtmlTextAndAttr(snapshot.id)}" style="align-items:center;gap:8px;padding:4px 0;border-bottom:1px solid var(--b3-border-color)">
            <span class="fn__flex-1">${escapeHtmlTextAndAttr(snapshot.createdAt)}
                <span class="ft__on-surface">${snapshot.files} files${snapshot.encrypted ? " · 🔒" : ""}</span></span>
            <button class="b3-button b3-button--outline backupSnapshotDelete" data-snapshot="${escapeHtmlTextAndAttr(snapshot.id)}">${escapeHtmlTextAndAttr(window.siyuan.languages.delete)}</button>
        </div>`;
    let busy = false;
    const setBusy = (on: boolean, message = ""): void => {
        busy = on;
        const busyEl = document.querySelector("#backupSnapshotBusy");
        if (busyEl?.isConnected) {
            busyEl.classList.toggle("fn__none", !on);
            busyEl.textContent = on ? message : "";
        }
        document.querySelectorAll("#backupSnapshotList .backupSnapshotDelete").forEach(button => {
            (button as HTMLButtonElement).disabled = on;
        });
    };
    const loadAndBind = async (): Promise<number> => {
        const listEl = document.querySelector("#backupSnapshotList");
        if (!listEl?.isConnected) {
            return 0;
        }
        const listResponse = await fetchSyncPost("/api/backup/listSnapshots", {target: targetId});
        const snapshots = listResponse.code === 0
            ? (listResponse.data as {snapshots?: Array<{id: string; createdAt: string; files: number; encrypted: boolean}>}).snapshots || []
            : [];
        listEl.innerHTML = snapshots.length ? snapshots.map(snapshotRow).join("")
            : `<div class="b3-label__text">${escapeHtmlTextAndAttr(text("backupRestoreEmpty", "No snapshots on this target"))}</div>`;
        listEl.querySelectorAll(".backupSnapshotDelete").forEach(button => {
            button.addEventListener("click", (event) => {
                if (busy) {
                    return;
                }
                const snapshotId = (event.currentTarget as HTMLElement).getAttribute("data-snapshot") || "";
                confirmDialog(text("backupSnapshotsTitle", "Snapshots"),
                    text("backupSnapshotDeleteConfirm", "This deletes snapshot ${x} and its blobs not referenced by other snapshots; this action cannot be undone.")
                        .replace("${x}", snapshotId),
                    async () => {
                        setBusy(true, text("backupWorking", "Working…"));
                        try {
                            const response = await fetchSyncPost("/api/backup/deleteSnapshot", {target: targetId, snapshot: snapshotId});
                            const result = response.data as {blobs?: number} | undefined;
                            if (response.code !== 0) {
                                showMessage(`${text("backupSnapshotDeleteFailed", "Failed to delete the snapshot")}: ${response.msg}`, 7000, "error");
                                return;
                            }
                            showMessage(text("backupSnapshotDeleteDone", "Snapshot deleted; ${x} blob(s) removed")
                                .replace("${x}", String(result?.blobs || 0)), 3000);
                            await loadAndBind();
                            await loadPanel();
                        } finally {
                            setBusy(false);
                        }
                    }, undefined, true);
            });
        });
        return snapshots.length;
    };
    const dialogTitle = text("backupSnapshotsTitle", "Snapshots");
    const dialogHTML = `<div class="b3-dialog__content" style="padding:12px">
    <div id="backupSnapshotBusy" class="ft__on-surface fn__none" style="margin-bottom:8px"></div>
    <div id="backupSnapshotList" style="max-height:320px;overflow:auto;min-width:320px"></div>
</div>`;
    confirmDialog(dialogTitle, dialogHTML, async () => {
        // 确认按钮 = 清空全部:先取当前快照数,空目标直接提示
        const listResponse = await fetchSyncPost("/api/backup/listSnapshots", {target: targetId});
        const snapshots = listResponse.code === 0
            ? (listResponse.data as {snapshots?: unknown[]}).snapshots || [] : [];
        if (!snapshots.length) {
            showMessage(text("backupEmptyAlready", "The target storage is already empty"), 3000);
            return;
        }
        confirmDialog(dialogTitle,
            text("backupEmptyAllConfirm", "This deletes all ${x} snapshots and their data on the target; this action cannot be undone. The ownership marker is kept.")
                .replace("${x}", String(snapshots.length)),
            async () => {
                // 清空可能逐个删除大量数据块,耗时较长:先给进行中提示,结束后有成功/失败消息
                showMessage(text("backupEmptyRunning", "Emptying: deleting data on the target storage…"), 30000);
                const response = await fetchSyncPost("/api/backup/emptyTarget", {target: targetId});
                const result = response.data as {snapshots?: number; blobs?: number} | undefined;
                if (response.code !== 0) {
                    showMessage(`${text("backupEmptyFailed", "Failed to empty the target")}: ${response.msg}`, 7000, "error");
                    return;
                }
                showMessage(text("backupEmptyDone", "Emptied ${x} snapshot(s) and ${y} blob(s)")
                    .replace("${x}", String(result?.snapshots || 0)).replace("${y}", String(result?.blobs || 0)), 4000);
                await loadPanel();
            }, undefined, true, undefined, text("backupEmptyAll", "Empty all"));
    }, undefined, true, undefined, text("backupEmptyAll", "Empty all"));
    await loadAndBind();
};

// 还原是单源操作:仅能从目标行的还原按钮发起,源目标在对话框内钉死,禁止批量还原
const openRestoreDialog = async (targetId: string) => {
    const target = (currentConfig.targets || []).find(item => item.id === targetId && item.enabled);
    if (!target) {
        showMessage(text("backupRestoreNoTargets", "No enabled backup target to restore from"), 6000, "error");
        return;
    }
    // 快照列表按该目标拉取
    const snapshotOptionsHTML = (snapshots: Array<{id: string; createdAt: string; files: number; encrypted: boolean}>) =>
        snapshots.map(snapshot =>
            `<option value="${escapeHtmlTextAndAttr(snapshot.id)}">${escapeHtmlTextAndAttr(snapshot.createdAt)} · ${snapshot.files} files${snapshot.encrypted ? " · 🔒" : ""}</option>`).join("");
    const loadSnapshots = async (id: string): Promise<string> => {
        const listResponse = await fetchSyncPost("/api/backup/listSnapshots", {target: id});
        if (listResponse.code !== 0) {
            return `<option value="">${escapeHtmlTextAndAttr(`${text("backupRestoreListFailed", "Could not list snapshots")}: ${listResponse.msg}`)}</option>`;
        }
        const snapshots = (listResponse.data as {snapshots?: Array<{id: string; createdAt: string; files: number; encrypted: boolean}>}).snapshots || [];
        if (!snapshots.length) {
            return `<option value="">${escapeHtmlTextAndAttr(text("backupRestoreEmpty", "No snapshots on this target"))}</option>`;
        }
        return snapshotOptionsHTML(snapshots);
    };
    const snapshotOptions = await loadSnapshots(target.id);
    const dialogHTML = `<div class="b3-dialog__content" style="padding:12px">
    <div class="fn__flex" style="align-items:center;gap:8px;margin-bottom:8px">
        <span class="fn__flex-center ft__on-surface">${escapeHtmlTextAndAttr(text("backupRestoreTargetLabel", "Target"))}</span>
        <span class="fn__flex-1">${escapeHtmlTextAndAttr(targetLabel(target))}</span>
    </div>
    <select id="backupRestoreSnapshot" class="b3-select" style="width:100%">${snapshotOptions}</select>
    <input id="backupRestorePassphrase" type="password" class="b3-text-field fn__block" style="margin-top:8px"
        placeholder="${escapeHtmlTextAndAttr(text("backupRestorePassphrase", "Passphrase (encrypted backups only)"))}">
    <label class="fn__flex" style="align-items:center;gap:6px;margin-top:8px;cursor:pointer">
        <input id="backupRestoreForce" type="checkbox" checked>
        <span>${escapeHtmlTextAndAttr(text("backupRestoreForceLabel", "Converge workspace to the snapshot"))}</span>
    </label>
    <label class="fn__flex" style="align-items:center;gap:6px;margin-top:8px;cursor:pointer">
        <input id="backupRestoreKeepIdentity" type="checkbox">
        <span>${escapeHtmlTextAndAttr(text("backupRestoreKeepIdentity", "Keep system identity and lock-screen passphrase"))}</span>
    </label>
    <div class="ft__on-surface" style="margin-top:4px;font-size:12px">${escapeHtmlTextAndAttr(text("backupRestoreKeepIdentityTip", "By default the restored copy gets a fresh system identity and no lock-screen passphrase to avoid clashing with the source instance; check this only when migrating the whole instance."))}</div>
    <div class="ft__on-surface" style="margin-top:8px;font-size:12px">${escapeHtmlTextAndAttr(text("backupRestoreWarn", "Restoring overwrites data/ and conf/ in the current workspace. With converge enabled the workspace becomes an exact copy of the snapshot: documents and assets not in the snapshot are deleted; excluded files such as backup credentials are kept"))}</div>
</div>`;
    confirmDialog(text("backupRestoreTitle", "Restore from backup"), dialogHTML, async () => {
        const snapshot = (document.querySelector("#backupRestoreSnapshot") as HTMLSelectElement)?.value || "";
        if (!snapshot) {
            showMessage(text("backupRestoreEmpty", "No snapshots on this target"), 6000, "error");
            return;
        }
        const passphrase = (document.querySelector("#backupRestorePassphrase") as HTMLInputElement)?.value || "";
        const force = (document.querySelector("#backupRestoreForce") as HTMLInputElement)?.checked || false;
        const dryRun = await fetchSyncPost("/api/backup/restoreDryRun", {target: targetId, snapshot, passphrase, force});
        if (dryRun.code !== 0) {
            showMessage(`${text("backupRestoreFailed", "Restore failed")}: ${dryRun.msg}`, 7000, "error");
            return;
        }
        const plan = dryRun.data as {conflicts?: number; removeCount?: number} | undefined;
        // 未勾选覆盖还原且工作空间已有数据:还原必然被拒绝,提前拦截并提示
        if (!force && (plan?.conflicts || 0) > 0) {
            showMessage(text("backupRestoreNeedForce", "The workspace already has data; enable converge to restore"), 7000, "error");
            return;
        }
        // 还原是同步 HTTP:面板进度行轮询内核还原状态,动作按钮全面灰掉,完成后回填面板
        const progressEl = panelRoot?.querySelector("#backupRunProgress") as HTMLElement;
        const restoreTip = text("backupRestoreStarted", "Restore started");
        setBackupBusy(true);
        if (progressEl) {
            progressEl.classList.remove("fn__none");
            progressEl.textContent = restoreTip;
        }
        const poll = window.setInterval(async () => {
            const response = await fetchSyncPost("/api/backup/getConfig", {});
            const restore = (response.data as BackupData)?.progress?.restore;
            if (restore) {
                setBackupBusy(true, restore.target);
                if (progressEl) {
                    const seconds = Math.max(1, Math.round(Date.now() / 1000) - restore.startedAt);
                    progressEl.textContent =
                        text("backupProgressRestore", "Restoring: ${done}/${total} · ${seconds}s")
                            .replace("${done}", String(restore.done))
                            .replace("${total}", String(restore.total))
                            .replace("${seconds}", String(seconds));
                }
            }
        }, 2000);
        try {
            const response = await fetchSyncPost("/api/backup/restore", {target: targetId, snapshot, passphrase, force,
                keepIdentity: (document.querySelector("#backupRestoreKeepIdentity") as HTMLInputElement)?.checked || false});
            const data = response.data as {files?: number; bytes?: number; removed?: number; reindexed?: boolean} | undefined;
            if (response.code !== 0) {
                showMessage(`${text("backupRestoreFailed", "Restore failed")}: ${response.msg}`, 7000, "error");
                return;
            }
            // 还原改写整库:内核已在返回前同步重建索引,此处经内核广播整页重载,
            // 与数据导入完成后同通道,避免界面仍展示还原前的文档树与页签
            await fetchSyncPost("/api/ui/reloadUI", {});
            let message = `${text("backupRestoreReindexed", "Restored and reindexed")}: ${data?.files || 0} files`;
            if ((data?.removed || 0) > 0) {
                message += ", " + text("backupRestorePruned", "${x} stale files removed").replace("${x}", String(data?.removed));
            }
            showMessage(message, 6000);
        } finally {
            window.clearInterval(poll);
            setBackupBusy(false);
            if (progressEl) {
                progressEl.classList.add("fn__none");
                progressEl.textContent = "";
            }
            await loadPanel();
        }
    });
};

export const refreshBackupPanel = () => {
    if (panelRoot) {
        void loadPanel();
    }
};

export const registerBackupTab = (tab: SettingTabBuilder) => {
    const group = tab.group("backup", window.siyuan.languages.backup || "Backup");
    group.slot({
        key: "backupPanel",
        keywords: [text("backupTargets", "Backup targets")],
        html: () => "<div id=\"backupPanel\"></div>",
        afterMount: async (root) => {
            panelRoot = root.querySelector("#backupPanel") as HTMLElement || root;
            await loadPanel();
        },
    });
};
