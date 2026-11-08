import {fetchSyncPost} from "../../util/fetch";
import {confirmDialog} from "../../dialog/confirmDialog";
import {showMessage} from "../../dialog/message";
import {escapeHtmlTextAndAttr} from "../../util/escape";
import type {SettingTabBuilder} from "../setting/builder";

type BackupType = "pg" | "s3" | "webdav";

type Retention = {keepLast?: number; maxAgeDays?: number};
type PGConfig = {host: string; port?: number; user?: string; password?: string; database: string; sslMode?: string; schema?: string};
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
    restore?: {startedAt: number; done: number; total: number};
}};

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
        <button class="b3-button b3-button--outline" id="backupRun">${escapeHtmlTextAndAttr(text("backupRunNow", "Back up now"))}</button>
        <button class="b3-button b3-button--outline" id="backupRestore">${escapeHtmlTextAndAttr(text("backupRestoreTitle", "Restore"))}</button>
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
            <div id="backupTypeFields"></div>
            <div class="fn__flex" style="gap:8px;margin-top:8px">
                <button class="b3-button b3-button--outline" id="backupSave">${escapeHtmlTextAndAttr(window.siyuan.languages.save)}</button>
                <button class="b3-button b3-button--outline" id="backupTest">${escapeHtmlTextAndAttr(text("backupTestTarget", "Test connection"))}</button>
                <button class="b3-button b3-button--outline" id="backupCancel">${escapeHtmlTextAndAttr(window.siyuan.languages.cancel)}</button>
            </div>
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
    panelRoot.querySelector("#backupRestore")?.addEventListener("click", () => void openRestoreDialog());
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
    panelRoot.querySelector("#backupTest")?.addEventListener("click", () => void testTarget(readForm()));
    panelRoot.querySelector("#backupCancel")?.addEventListener("click", () => closeForm());
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
    const fields = panelRoot?.querySelector("#backupTypeFields");
    if (fields) {
        fields.innerHTML = typeFields(target);
    }
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
                    .replace("${target}", run.target)
                    .replace("${done}", String(run.done))
                    .replace("${total}", String(run.total))
                    .replace("${seconds}", String(seconds));
        }
        progressEl.classList.remove("fn__none");
    }
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
    currentConfig.enabled = true;
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

const runBackup = async () => {
    if (currentConfig.targets.length > 0 && !currentConfig.enabled) {
        currentConfig.enabled = true;
        if (!await saveConfig()) {
            return;
        }
    }
    // 备份支持批量:仅备份勾选的目标;还原不受此勾选影响
    const selected = Array.from(panelRoot?.querySelectorAll(".backupTargetSelect:checked") || [])
        .map((el: HTMLInputElement) => el.dataset.id).filter(Boolean) as string[];
    if (!selected.length) {
        showMessage(text("backupSelectTargets", "Select at least one backup target"), 6000, "error");
        return;
    }
    // 备份是同步 HTTP,大工作空间要跑数分钟:点击即进 loading 态并轮询运行态,
    // 防止重复点击与无反馈
    const button = panelRoot?.querySelector("#backupRun") as HTMLButtonElement;
    if (button?.hasAttribute("disabled")) {
        return;
    }
    button?.setAttribute("disabled", "disabled");
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
        if (run && progressEl) {
            const seconds = Math.max(1, Math.round(Date.now() / 1000) - run.startedAt);
            progressEl.textContent =
                text("backupProgressBackup", "Backing up ${target}: ${done}/${total} · ${seconds}s")
                    .replace("${target}", run.target)
                    .replace("${done}", String(run.done))
                    .replace("${total}", String(run.total))
                    .replace("${seconds}", String(seconds));
        }
    }, 2000);
    try {
        const response = await fetchSyncPost("/api/backup/run", {targets: selected});
        if (response.code !== 0) {
            showMessage(`${text("backupRunFailed", "Backup failed")}: ${response.msg}`, 7000, "error");
            return;
        }
        const result = response.data as {snapshot?: string; targets?: Array<{ok: boolean; error?: string}>};
        const failed = (result.targets || []).filter(target => !target.ok);
        if (failed.length) {
            showMessage(`${text("backupRunFailed", "Backup failed")}: ${failed.map(target => target.error).join("; ")}`, 7000, "error");
        } else {
            showMessage(`${text("backupRunOK", "Backup completed")}: ${result.snapshot || ""}`, 4000);
        }
    } finally {
        window.clearInterval(poll);
        button?.removeAttribute("disabled");
        if (progressEl) {
            progressEl.classList.add("fn__none");
            progressEl.textContent = "";
        }
        await loadPanel();
    }
};

const openRestoreDialog = async () => {
    const enabledTargets = (currentConfig.targets || []).filter(target => target.enabled);
    if (!enabledTargets.length) {
        showMessage(text("backupRestoreNoTargets", "No enabled backup target to restore from"), 6000, "error");
        return;
    }
    // 还原是单目标操作:对话框内选目标,切换后按需拉取该目标的快照列表
    const snapshotOptionsHTML = (snapshots: Array<{id: string; createdAt: string; files: number; encrypted: boolean}>) =>
        snapshots.map(snapshot =>
            `<option value="${escapeHtmlTextAndAttr(snapshot.id)}">${escapeHtmlTextAndAttr(snapshot.createdAt)} · ${snapshot.files} files${snapshot.encrypted ? " · 🔒" : ""}</option>`).join("");
    const loadSnapshots = async (targetId: string): Promise<string> => {
        const listResponse = await fetchSyncPost("/api/backup/listSnapshots", {target: targetId});
        if (listResponse.code !== 0) {
            return `<option value="">${escapeHtmlTextAndAttr(`${text("backupRestoreListFailed", "Could not list snapshots")}: ${listResponse.msg}`)}</option>`;
        }
        const snapshots = (listResponse.data as {snapshots?: Array<{id: string; createdAt: string; files: number; encrypted: boolean}>}).snapshots || [];
        if (!snapshots.length) {
            return `<option value="">${escapeHtmlTextAndAttr(text("backupRestoreEmpty", "No snapshots on this target"))}</option>`;
        }
        return snapshotOptionsHTML(snapshots);
    };
    const targetOptions = enabledTargets.map((target, index) =>
        `<option value="${escapeHtmlTextAndAttr(target.id)}"${index === 0 ? " selected" : ""}>${escapeHtmlTextAndAttr(targetLabel(target))}</option>`).join("");
    const snapshotOptions = await loadSnapshots(enabledTargets[0].id);
    const dialogHTML = `<div class="b3-dialog__content" style="padding:12px">
    <div class="fn__flex" style="align-items:center;gap:8px;margin-bottom:8px">
        <span class="fn__flex-center ft__on-surface">${escapeHtmlTextAndAttr(text("backupRestoreTargetLabel", "Target"))}</span>
        <select id="backupRestoreTarget" class="b3-select" style="width:100%">${targetOptions}</select>
    </div>
    <select id="backupRestoreSnapshot" class="b3-select" style="width:100%">${snapshotOptions}</select>
    <input id="backupRestorePassphrase" type="password" class="b3-text-field fn__block" style="margin-top:8px"
        placeholder="${escapeHtmlTextAndAttr(text("backupRestorePassphrase", "Passphrase (encrypted backups only)"))}">
    <label class="fn__flex" style="align-items:center;gap:6px;margin-top:8px;cursor:pointer">
        <input id="backupRestoreForce" type="checkbox" checked>
        <span>${escapeHtmlTextAndAttr(text("backupRestoreForceLabel", "Converge workspace to the snapshot"))}</span>
    </label>
    <div class="ft__on-surface" style="margin-top:8px;font-size:12px">${escapeHtmlTextAndAttr(text("backupRestoreWarn", "Restoring overwrites data/ and conf/ in the current workspace. With converge enabled the workspace becomes an exact copy of the snapshot: documents and assets not in the snapshot are deleted; excluded files such as backup credentials are kept"))}</div>
</div>`;
    confirmDialog(text("backupRestoreTitle", "Restore from backup"), dialogHTML, async () => {
        const targetId = (document.querySelector("#backupRestoreTarget") as HTMLSelectElement)?.value || "";
        const target = enabledTargets.find(item => item.id === targetId);
        if (!target) {
            showMessage(text("backupRestoreNoTargets", "No enabled backup target to restore from"), 6000, "error");
            return;
        }
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
        // 还原是同步 HTTP:面板进度行轮询内核还原状态,完成后回填面板
        const progressEl = panelRoot?.querySelector("#backupRunProgress") as HTMLElement;
        const restoreTip = text("backupRestoreStarted", "Restore started");
        if (progressEl) {
            progressEl.classList.remove("fn__none");
            progressEl.textContent = restoreTip;
        }
        const poll = window.setInterval(async () => {
            const response = await fetchSyncPost("/api/backup/getConfig", {});
            const restore = (response.data as BackupData)?.progress?.restore;
            if (restore && progressEl) {
                const seconds = Math.max(1, Math.round(Date.now() / 1000) - restore.startedAt);
                progressEl.textContent =
                    text("backupProgressRestore", "Restoring: ${done}/${total} · ${seconds}s")
                        .replace("${done}", String(restore.done))
                        .replace("${total}", String(restore.total))
                        .replace("${seconds}", String(seconds));
            }
        }, 2000);
        try {
            const response = await fetchSyncPost("/api/backup/restore", {target: targetId, snapshot, passphrase, force});
            const data = response.data as {files?: number; bytes?: number; removed?: number} | undefined;
            if (response.code !== 0) {
                showMessage(`${text("backupRestoreFailed", "Restore failed")}: ${response.msg}`, 7000, "error");
                return;
            }
            let message = `${text("backupRestoreOK", "Restored")}: ${data?.files || 0} files`;
            if ((data?.removed || 0) > 0) {
                message += ", " + text("backupRestorePruned", "${x} stale files removed").replace("${x}", String(data?.removed));
            }
            showMessage(message, 6000);
        } finally {
            window.clearInterval(poll);
            if (progressEl) {
                progressEl.classList.add("fn__none");
                progressEl.textContent = "";
            }
            await loadPanel();
        }
    });
    // 目标切换后重拉快照列表(Dialog 同步注入 DOM,绑定安全)
    const targetSelect = document.querySelector("#backupRestoreTarget") as HTMLSelectElement;
    const snapshotSelect = document.querySelector("#backupRestoreSnapshot") as HTMLSelectElement;
    targetSelect?.addEventListener("change", async () => {
        if (!snapshotSelect) {
            return;
        }
        snapshotSelect.innerHTML = "<option value=\"\">...</option>";
        snapshotSelect.innerHTML = await loadSnapshots(targetSelect.value);
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
