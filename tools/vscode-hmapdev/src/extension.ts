import * as cp from "child_process";
import * as path from "path";
import * as vscode from "vscode";

import { PlgConfig, Severity, statusBarText, validatePlg } from "./core";
import { Toolchain } from "./toolchain";

let out: vscode.OutputChannel;
let tc: Toolchain;
let status: vscode.StatusBarItem;
let diagnostics: vscode.DiagnosticCollection;
let tailChild: cp.ChildProcess | undefined;

/** 找到工作区里的 plg.json（多个时取第一个并提示）。 */
async function findPlg(): Promise<vscode.Uri | undefined> {
  const found = await vscode.workspace.findFiles("**/plg.json", "**/{node_modules,out,dist,build}/**", 5);
  if (found.length === 0) {
    return undefined;
  }
  return found[0];
}

async function readPlg(uri: vscode.Uri): Promise<PlgConfig | undefined> {
  try {
    const txt = Buffer.from(await vscode.workspace.fs.readFile(uri)).toString("utf8");
    return JSON.parse(txt) as PlgConfig;
  } catch (e) {
    out.appendLine(`error: 解析 ${uri.fsPath} 失败：${e instanceof Error ? e.message : String(e)}`);
    return undefined;
  }
}

function severityToVscode(s: Severity): vscode.DiagnosticSeverity {
  switch (s) {
    case Severity.Error:
      return vscode.DiagnosticSeverity.Error;
    case Severity.Warning:
      return vscode.DiagnosticSeverity.Warning;
    case Severity.Information:
      return vscode.DiagnosticSeverity.Information;
    default:
      return vscode.DiagnosticSeverity.Hint;
  }
}

/** 在 JSON 文档里定位字段（找不到就标整个文件，至少让人看见）。 */
function rangeForField(doc: vscode.TextDocument, field?: string): vscode.Range {
  if (field) {
    const idx = doc.getText().indexOf(`"${field}"`);
    if (idx >= 0) {
      const start = doc.positionAt(idx);
      const end = doc.positionAt(idx + field.length + 2);
      return new vscode.Range(start, end);
    }
  }
  return new vscode.Range(new vscode.Position(0, 0), new vscode.Position(0, 0));
}

async function refresh(): Promise<void> {
  const uri = await findPlg();
  diagnostics.clear();
  if (!uri) {
    status.text = statusBarText(undefined, await tc.version());
    status.tooltip = "工作区里没有找到 plg.json（本扩展只在插件工程里工作）";
    return;
  }
  const cfg = await readPlg(uri);
  if (!cfg) {
    return;
  }

  const tcVersion = await tc.version();
  const diagSetting = vscode.workspace.getConfiguration("hmapdev").get<boolean>("diagnoseSdk", true);
  const installed = diagSetting ? await tc.sdkList() : undefined;

  const doc = await vscode.workspace.openTextDocument(uri);
  const items = validatePlg(cfg, installed).map((d) => {
    const vd = new vscode.Diagnostic(rangeForField(doc, d.field), d.message, severityToVscode(d.severity));
    vd.source = "hmapdev";
    return vd;
  });
  diagnostics.set(uri, items);

  const errors = items.filter((d) => d.severity === vscode.DiagnosticSeverity.Error).length;
  status.text = `$(tools) ${statusBarText(cfg, tcVersion)}`;
  status.backgroundColor = tc.isMissing()
    ? new vscode.ThemeColor("statusBarItem.errorBackground")
    : errors > 0
      ? new vscode.ThemeColor("statusBarItem.warningBackground")
      : undefined;
  const installedText = installed ? installed.join(", ") || "（无）" : "（未探测）";
  status.tooltip = [
    `插件：${cfg.name ?? "?"}`,
    `声明 SDK：${cfg.sdk ?? "未声明"}`,
    `已安装 SDK：${installedText}`,
    `工具链：${tcVersion ? `hmapdev ${tcVersion}` : "未找到（检查 hmapdev.path / PATH）"}`,
    `plg.json：${uri.fsPath}`,
  ].join("\n");
  status.command = "hmapdev.openPlgJson";
  status.show();
}

async function pluginDir(): Promise<string | undefined> {
  const uri = await findPlg();
  return uri ? path.dirname(uri.fsPath) : undefined;
}

async function withDir(fn: (dir: string) => unknown | Promise<unknown>): Promise<void> {
  const dir = await pluginDir();
  if (!dir) {
    void vscode.window.showWarningMessage("当前工作区没有 plg.json，无法定位插件工程。");
    return;
  }
  await fn(dir);
}

export function activate(context: vscode.ExtensionContext): void {
  out = vscode.window.createOutputChannel("hmapdev");
  tc = new Toolchain(out);
  diagnostics = vscode.languages.createDiagnosticCollection("hmapdev");
  status = vscode.window.createStatusBarItem(vscode.StatusBarAlignment.Left, 100);

  context.subscriptions.push(out, diagnostics, status);

  const reg = (id: string, fn: () => unknown) =>
    context.subscriptions.push(vscode.commands.registerCommand(id, async () => {
      try {
        await fn();
      } catch (e) {
        const msg = e instanceof Error ? e.message : String(e);
        out.appendLine(`error: ${msg}`);
        void vscode.window.showErrorMessage(`hmapdev: ${msg}`);
      }
    }));

  reg("hmapdev.build", () => withDir((d) => tc.runInTerminal(["build"], d, "hmapdev build")));
  reg("hmapdev.buildAll", () => withDir((d) => tc.runInTerminal(["build", "--target", "all"], d, "hmapdev build all")));
  reg("hmapdev.clean", () => withDir((d) => tc.runInTerminal(["clean"], d, "hmapdev clean")));
  reg("hmapdev.run", () => withDir((d) => tc.runInTerminal(["debug", d], d, "hmapdev debug")));

  reg("hmapdev.showVersion", async () => {
    out.show(true);
    const v = await tc.version(true);
    if (!v) {
      void vscode.window.showErrorMessage("找不到 hmapdev：请把它放到 PATH，或设置 hmapdev.path。");
      return;
    }
    await tc.runAndLog(["version"], process.cwd());
  });

  reg("hmapdev.listSdk", async () => {
    out.show(true);
    await tc.runAndLog(["sdk", "list"], process.cwd());
    await refresh();
  });

  reg("hmapdev.installSdk", async () => {
    const v = await vscode.window.showInputBox({
      title: "安装 SDK 版本",
      prompt: '输入完整版本号（如 1.2.0）或 latest。注意：SDK 版本跟随内核中版本，patch 位恒为 .0。',
      placeHolder: "1.2.0",
    });
    if (!v) {
      return;
    }
    tc.runInTerminal(["sdk", "install", `v${v.replace(/^v/, "")}`], process.cwd(), "hmapdev sdk install");
    void vscode.window.showInformationMessage(`安装完成后执行「hmapdev: 刷新状态」以重新校验。`);
  });

  reg("hmapdev.useSdk", async () => {
    const list = await tc.sdkList(true);
    if (!list || list.length === 0) {
      void vscode.window.showWarningMessage("没有探测到已安装的 SDK 版本（先跑「hmapdev: 列出 SDK 版本」看看）。");
      return;
    }
    const pick = await vscode.window.showQuickPick(list, { title: "切换当前 SDK 版本（存储里的 current）" });
    if (!pick) {
      return;
    }
    tc.runInTerminal(["sdk", "use", `v${pick}`], process.cwd(), "hmapdev sdk use");
    setTimeout(() => void refresh(), 1500);
  });

  reg("hmapdev.tailKernelLog", async () => {
    const cfgDir = vscode.workspace.getConfiguration("hmapdev");
    let dataDir = cfgDir.get<string>("kernelDataDir", "");
    if (!dataDir) {
      const answer = await vscode.window.showInputBox({
        title: "内核数据目录",
        prompt: "homed -data 指向的目录（用于跟随内核日志）。填一次会记住到设置里。",
        placeHolder: "/home/newqqagent",
      });
      if (!answer) {
        return;
      }
      dataDir = answer;
      await cfgDir.update("kernelDataDir", dataDir, vscode.ConfigurationTarget.Workspace);
    }
    const uri = await findPlg();
    const filter = uri ? (await readPlg(uri))?.name ?? "" : "";
    tailChild?.kill();
    tailChild = tc.tailKernelLog(dataDir, filter);
    out.show(true);
  });

  reg("hmapdev.stopTailKernelLog", () => {
    if (tailChild) {
      tailChild.kill();
      tailChild = undefined;
      out.appendLine("已停止跟随内核日志");
    }
  });

  reg("hmapdev.openPlgJson", async () => {
    const uri = await findPlg();
    if (uri) {
      await vscode.window.showTextDocument(await vscode.workspace.openTextDocument(uri));
    } else {
      void vscode.window.showWarningMessage("工作区里没有 plg.json。");
    }
  });

  reg("hmapdev.refresh", async () => {
    await tc.sdkList(true);
    await tc.version(true);
    await refresh();
  });

  // 任务提供者：把 hmapdev 动作接进 VSCode 的任务体系（可绑定快捷键 / 串联依赖 / 复用问题匹配器）
  context.subscriptions.push(
    vscode.tasks.registerTaskProvider("hmapdev", {
      provideTasks: async () => {
        const dir = await pluginDir();
        if (!dir) {
          return [];
        }
        const mk = (action: string, label: string, args: string[]) => {
          const def: vscode.TaskDefinition = { type: "hmapdev", action };
          const exec = new vscode.ProcessExecution(
            vscode.workspace.getConfiguration("hmapdev").get<string>("path", "hmapdev") || "hmapdev",
            args,
            { cwd: dir }
          );
          return new vscode.Task(def, vscode.TaskScope.Workspace, label, "hmapdev", exec, ["$hmapdev-go"]);
        };
        return [
          mk("build", "hmapdev: build", ["build"]),
          mk("buildAll", "hmapdev: build (all targets)", ["build", "--target", "all"]),
          mk("clean", "hmapdev: clean", ["clean"]),
          mk("run", "hmapdev: run (interpreted)", ["debug", dir]),
        ];
      },
      resolveTask: (task) => task,
    })
  );

  // plg.json 变化 → 重算诊断（含保存与外部修改）
  const watcher = vscode.workspace.createFileSystemWatcher("**/plg.json");
  context.subscriptions.push(
    watcher,
    watcher.onDidChange(() => void refresh()),
    watcher.onDidCreate(() => void refresh()),
    watcher.onDidDelete(() => void refresh())
  );

  void refresh();
}

export function deactivate(): void {
  tailChild?.kill();
  tailChild = undefined;
}
