import * as cp from "child_process";
import * as fs from "fs";
import * as path from "path";
import * as vscode from "vscode";

import { parseSdkList, parseToolchainVersion } from "./core";

/** execFile 的 Promise 版（不引第三方依赖）。 */
function execFile(
  file: string,
  args: string[],
  cwd: string,
  timeoutMs = 120_000
): Promise<{ code: number; stdout: string; stderr: string }> {
  return new Promise((resolve) => {
    cp.execFile(file, args, { cwd, timeout: timeoutMs, maxBuffer: 8 * 1024 * 1024 }, (err, stdout, stderr) => {
      const code = err && typeof (err as { code?: number }).code === "number" ? (err as { code: number }).code : err ? 1 : 0;
      resolve({ code, stdout: stdout ?? "", stderr: stderr ?? "" });
    });
  });
}

/**
 * 工具链封装：定位 hmapdev、执行命令、缓存 version / sdk list。
 *
 * 为什么要缓存并显式 refresh：SDK 存储会在外部变化（`hmapdev sdk install` 之后），
 * 而诊断信息依赖它——不刷新就会一直报「未安装」。
 */
export class Toolchain {
  private versionCache?: string;
  private sdkCache?: string[];
  private missing = false;

  constructor(private readonly out: vscode.OutputChannel) {}

  private get exe(): string {
    return vscode.workspace.getConfiguration("hmapdev").get<string>("path", "hmapdev") || "hmapdev";
  }

  /** 记录一条消息到输出通道（加上工具链前缀，便于与构建输出区分）。 */
  log(line: string): void {
    this.out.appendLine(line);
  }

  async version(refresh = false): Promise<string> {
    if (this.versionCache !== undefined && !refresh) {
      return this.versionCache;
    }
    const r = await execFile(this.exe, ["version"], process.cwd(), 20_000);
    if (r.code !== 0 && !r.stdout) {
      this.missing = true;
      this.versionCache = "";
      return "";
    }
    this.missing = false;
    this.versionCache = parseToolchainVersion(r.stdout + r.stderr);
    return this.versionCache;
  }

  async sdkList(refresh = false): Promise<string[] | undefined> {
    if (this.sdkCache !== undefined && !refresh) {
      return this.sdkCache;
    }
    const r = await execFile(this.exe, ["sdk", "list"], process.cwd(), 20_000);
    if (r.code !== 0 && !r.stdout) {
      this.sdkCache = undefined; // 探测不到就不做「未安装」判断，避免误报
      return undefined;
    }
    this.sdkCache = parseSdkList(r.stdout + r.stderr);
    return this.sdkCache;
  }

  isMissing(): boolean {
    return this.missing;
  }

  /** 在集成终端里执行（构建/运行这类长命令：要能看进度、能 Ctrl-C）。 */
  runInTerminal(args: string[], cwd: string, name: string): vscode.Terminal {
    const term = vscode.window.createTerminal({ name, cwd });
    term.show(true);
    const cmd = [this.exe, ...args].map((a) => (/\s/.test(a) ? JSON.stringify(a) : a)).join(" ");
    this.log(`$ ${cmd}`);
    term.sendText(cmd, true);
    return term;
  }

  /** 一次性执行并把输出写进输出通道（查询类命令）。 */
  async runAndLog(args: string[], cwd: string): Promise<number> {
    this.log(`$ ${this.exe} ${args.join(" ")}`);
    const r = await execFile(this.exe, args, cwd, 60_000);
    if (r.stdout) {
      this.out.append(r.stdout);
    }
    if (r.stderr) {
      this.out.append(r.stderr);
    }
    return r.code;
  }

  /**
   * 跟随内核日志：定位 <dataDir>/log 下最新的 homed 日志并按插件名过滤。
   *
   * 为什么这是「调试插件」的正路：插件是子进程、跑在内核里，真正的问题几乎都
   * 表现为内核日志里的几行（握手失败/崩溃重启/工具报错），在 IDE 里跟住它比
   * 反复手动 tail 高效得多。
   */
  tailKernelLog(dataDir: string, filter: string): cp.ChildProcess | undefined {
    const logDir = path.join(dataDir, "log");
    if (!fs.existsSync(logDir)) {
      this.log(`error: 日志目录不存在：${logDir}（hmapdev.kernelDataDir 是否指对？）`);
      return undefined;
    }
    const newest = fs
      .readdirSync(logDir)
      .filter((f) => f.startsWith("homed_") && f.endsWith(".log"))
      .map((f) => ({ f, m: fs.statSync(path.join(logDir, f)).mtimeMs }))
      .sort((a, b) => b.m - a.m)[0];
    if (!newest) {
      this.log(`error: ${logDir} 下没有 homed_*.log`);
      return undefined;
    }
    const file = path.join(logDir, newest.f);
    this.log(`跟随 ${file}${filter ? `（过滤 ${filter}）` : ""}`);
    const child = cp.spawn("tail", ["-F", file], { stdio: ["ignore", "pipe", "pipe"] });
    const emit = (buf: Buffer) => {
      for (const line of buf.toString("utf8").split(/\r?\n/)) {
        if (!line) {
          continue;
        }
        if (!filter || line.includes(filter)) {
          this.out.appendLine(line);
        }
      }
    };
    child.stdout?.on("data", emit);
    child.stderr?.on("data", emit);
    return child;
  }
}
