/**
 * 纯逻辑层：不 import vscode，便于用 node --test 直接单测。
 *
 * 这里的规矩必须与工具链一致（tools/hmapdev/sdk_resolve.go）：
 *  - SDK 版本跟随内核中版本、**patch 位恒为 .0** → 一条内核线只有一个 SDK 版本；
 *  - 因此 plg.json 的 `sdk` 必须是**完整版本号**（x.y.z），区间写法（"1.2"）要报错，
 *    否则项目会以为「同一条线里还能挑不同 SDK」。
 */

/** plg.json 的字段（未知字段保留，不做拒绝）。 */
export interface PlgConfig {
  name?: string;
  name_zh?: string;
  name_en?: string;
  version?: string;
  description?: string;
  author?: string;
  entry?: string;
  sdk?: string;
  tags?: string[];
  targets?: string;
  sdk_path?: string;
  outdir?: string;
  bundle?: boolean;
  replaces?: Record<string, string>;
  source_dirs?: string[];
}

/** 诊断级别（与 vscode.DiagnosticSeverity 数值对齐，避免耦合）。 */
export enum Severity {
  Error = 0,
  Warning = 1,
  Information = 2,
  Hint = 3,
}

export interface PlgDiagnostic {
  severity: Severity;
  message: string;
  /** plg.json 里的字段名（用于在 JSON 文档里定位）。 */
  field?: string;
}

/** 完整版本号（x.y.z，允许 v 前缀）。 */
export function isFullVersion(v: string): boolean {
  return /^v?\d+\.\d+\.\d+$/.test((v ?? "").trim());
}

/** 中版本（x.y）。 */
export function isMinorVersion(v: string): boolean {
  return /^v?\d+\.\d+$/.test((v ?? "").trim());
}

export function normalizeVersion(v: string): string {
  return (v ?? "").trim().replace(/^v/, "");
}

/**
 * 校验 plg.json。
 *
 * `installedSdks` 为本地 SDK 存储里已安装的版本（不带 v 前缀）；传 undefined 表示
 * 没探测（例如工具链不可用），此时只校验格式、不报「未安装」。
 */
export function validatePlg(cfg: PlgConfig, installedSdks?: string[]): PlgDiagnostic[] {
  const out: PlgDiagnostic[] = [];
  const req = (field: keyof PlgConfig, hint: string) => {
    const v = cfg[field];
    if (v === undefined || v === null || String(v).trim() === "") {
      out.push({ severity: Severity.Error, message: `${field} 不能为空（${hint}）`, field: field as string });
    }
  };
  req("name", "插件名，与目录名一致最省事");
  req("entry", "入口产物，子进程模式通常是 plugin.bin");
  req("version", "插件自身版本号，如 0.1.0");

  // SDK 声明：这是「工具链自动选 SDK 版本」的依据，缺了就只能退回 current
  if (cfg.sdk === undefined || cfg.sdk === null || String(cfg.sdk).trim() === "") {
    out.push({
      severity: Severity.Warning,
      message: "缺少 sdk 字段：工具链无法据此选择 SDK 版本，会退回存储里的 current（换机器/换人后容易编出与预期不符的产物）",
      field: "sdk",
    });
  } else if (isMinorVersion(cfg.sdk)) {
    out.push({
      severity: Severity.Error,
      message:
        `sdk 必须是完整版本号（如 "1.2.0"）：${cfg.sdk} 这种区间写法会让人以为同一条内核线里还能挑不同 SDK。` +
        `SDK 版本跟随内核中版本、patch 位恒为 .0，一条内核线只有一个 SDK 版本。`,
      field: "sdk",
    });
  } else if (!isFullVersion(cfg.sdk)) {
    out.push({ severity: Severity.Error, message: `sdk 不是合法版本号（写法："1.2.0"）`, field: "sdk" });
  } else if (installedSdks && !installedSdks.includes(normalizeVersion(cfg.sdk))) {
    const have = installedSdks.length ? installedSdks.join(", ") : "（存储里还没有任何 SDK）";
    out.push({
      severity: Severity.Error,
      message: `声明的 SDK ${normalizeVersion(cfg.sdk)} 未安装。已安装：${have}。安装：hmapdev sdk install v${normalizeVersion(cfg.sdk)}`,
      field: "sdk",
    });
  }

  // 目标平台：windows 目前不支持（协议 2 的统一共享内存区未移植）
  const targets = (cfg.targets ?? "").toLowerCase();
  if (targets.includes("windows")) {
    out.push({
      severity: Severity.Information,
      message: "windows 目标暂不支持插件产物：协议 2 的统一共享内存区未移植 Windows（内核改走 WSL2）。构建会在该目标上明确报错。",
      field: "targets",
    });
  }
  if (!cfg.targets) {
    out.push({ severity: Severity.Information, message: "未声明 targets，构建时按默认目标处理", field: "targets" });
  }
  return out;
}

/**
 * 解析 `hmapdev sdk list` 的输出，返回已安装版本（去 v 前缀、升序）。
 *
 * 输出形如：
 *   Installed SDK versions:
 *     * v1.2.0
 *       v0.8.0
 * 每行可能带 `*` 标记（当前版本）或前导空格。
 */
export function parseSdkList(text: string): string[] {
  const out: string[] = [];
  for (const raw of (text ?? "").split(/\r?\n/)) {
    const line = raw.trim().replace(/^\*\s*/, "").trim();
    const m = /^v?(\d+\.\d+\.\d+)$/.exec(line);
    if (m) {
      out.push(m[1]);
    }
  }
  return out.sort(compareVersions);
}

/**
 * 解析 `hmapdev version` 的自述（首行形如 `hmapdev 1.2.0`）。
 *
 * 只认**以数字开头**的版本 token：否则 `hmapdev 未找到` / `hmapdev error`
 * 这类输出会被当成版本号，把「工具链不在」误报成「工具链 1.x」
 * （状态栏与「是否已装 SDK」的判断都基于它，假版本会让诊断全面失真）。
 */
export function parseToolchainVersion(text: string): string {
  for (const raw of (text ?? "").split(/\r?\n/)) {
    const m = /^hmapdev\s+v?(\d+(?:\.\d+)*(?:[-+.][0-9A-Za-z.-]+)?)\s*$/.exec(raw.trim());
    if (m) {
      return m[1];
    }
  }
  return "";
}

/** 数值比较 x.y.z（字典序会把 1.2.9 排在 1.2.10 之后）。 */
export function compareVersions(a: string, b: string): number {
  const pa = normalizeVersion(a).split(".").map((n) => parseInt(n, 10) || 0);
  const pb = normalizeVersion(b).split(".").map((n) => parseInt(n, 10) || 0);
  for (let i = 0; i < 3; i++) {
    const d = (pa[i] ?? 0) - (pb[i] ?? 0);
    if (d !== 0) {
      return d;
    }
  }
  return 0;
}

/** 状态栏文本：插件 + 声明 SDK + 工具链版本（缺项用 "?"）。 */
export function statusBarText(cfg: PlgConfig | undefined, toolchainVersion: string): string {
  const name = cfg?.name?.trim() || "(未识别插件)";
  const sdk = cfg?.sdk ? normalizeVersion(cfg.sdk) : "未声明";
  const tc = toolchainVersion ? `hmapdev ${toolchainVersion}` : "hmapdev 未找到";
  return `${name} · SDK ${sdk} · ${tc}`;
}
