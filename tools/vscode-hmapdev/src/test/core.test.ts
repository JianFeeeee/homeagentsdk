import * as assert from "node:assert/strict";
import { test } from "node:test";

import {
  PlgConfig,
  Severity,
  compareVersions,
  isFullVersion,
  isMinorVersion,
  normalizeVersion,
  parseSdkList,
  parseToolchainVersion,
  statusBarText,
  validatePlg,
} from "../core";

const good: PlgConfig = { name: "memo", version: "0.1.0", entry: "plugin.bin", sdk: "1.2.0" };

test("isFullVersion / isMinorVersion 区分完整版本与区间写法", () => {
  assert.equal(isFullVersion("1.2.0"), true);
  assert.equal(isFullVersion("v1.2.0"), true);
  assert.equal(isFullVersion("1.2"), false);
  assert.equal(isFullVersion("1.2.3.4"), false);
  assert.equal(isMinorVersion("1.2"), true);
  assert.equal(isMinorVersion("1.2.0"), false);
  assert.equal(normalizeVersion("v1.2.0"), "1.2.0");
});

test("合法的 plg.json 不产生错误", () => {
  const d = validatePlg(good, ["1.2.0"]);
  assert.equal(d.filter((x) => x.severity === Severity.Error).length, 0, JSON.stringify(d));
});

test("缺必需字段要报错并指出字段", () => {
  const d = validatePlg({ sdk: "1.2.0" }, ["1.2.0"]);
  const fields = d.filter((x) => x.severity === Severity.Error).map((x) => x.field).sort();
  assert.deepEqual(fields, ["entry", "name", "version"]);
});

test("区间写法 1.2 必须被拒，并说明 patch 位恒为 .0", () => {
  const d = validatePlg({ ...good, sdk: "1.2" }, ["1.2.0"]);
  const err = d.find((x) => x.field === "sdk" && x.severity === Severity.Error);
  assert.ok(err, "区间写法应报错");
  assert.match(err!.message, /完整版本号/);
  assert.match(err!.message, /patch 位恒为 \.0/);
});

test("缺 sdk 字段只警告（向后兼容存量项目）", () => {
  const d = validatePlg({ name: "memo", version: "0.1.0", entry: "plugin.bin" }, ["1.2.0"]);
  const sdk = d.find((x) => x.field === "sdk");
  assert.ok(sdk);
  assert.equal(sdk!.severity, Severity.Warning);
});

test("声明的 SDK 未安装要报错并给出安装命令", () => {
  const d = validatePlg(good, ["0.8.0"]);
  const err = d.find((x) => x.field === "sdk" && x.severity === Severity.Error);
  assert.ok(err, "未安装应报错");
  assert.match(err!.message, /hmapdev sdk install v1\.2\.0/);
  assert.match(err!.message, /0\.8\.0/);
});

test("探测不到已装列表时不误报未安装", () => {
  const d = validatePlg(good, undefined);
  assert.equal(d.filter((x) => x.severity === Severity.Error).length, 0, JSON.stringify(d));
});

test("windows 目标给提示（协议 2 未移植）", () => {
  const d = validatePlg({ ...good, targets: "linux/amd64,windows/amd64" }, ["1.2.0"]);
  const info = d.find((x) => x.field === "targets");
  assert.ok(info);
  assert.match(info!.message, /WSL2/);
});

test("parseSdkList 吃掉 * 标记与空格，并按数值排序", () => {
  const text = ["Installed SDK versions:", "  * v1.2.0", "    v0.8.0", "  v1.2.10"].join("\n");
  assert.deepEqual(parseSdkList(text), ["0.8.0", "1.2.0", "1.2.10"]);
  assert.deepEqual(parseSdkList("No SDK versions installed."), []);
});

test("parseToolchainVersion 从 self-report 里取版本", () => {
  const text = ["hmapdev 1.2.0", "  SDK 模块: gitcode.com/JianFeeeee/homeagent-sdk", "  构建用 Go: go1.25.12"].join("\n");
  assert.equal(parseToolchainVersion(text), "1.2.0");
  assert.equal(parseToolchainVersion("Usage:\n  hmapdev init <name>"), "");
});

// 反向核对抓到的真缺陷：`hmapdev <非版本>` 形状的输出曾被当成版本号，
// 于是「工具链不在」会被显示成「工具链 <垃圾词>」，并让 SDK 诊断跟着失真。
test("parseToolchainVersion 不会把非版本 token 当成版本", () => {
  for (const bad of ["hmapdev 未找到", "hmapdev error", "hmapdev not found", "hmapdev -v", "hmapdev"]) {
    assert.equal(parseToolchainVersion(bad), "", `不应从 ${JSON.stringify(bad)} 解析出版本`);
  }
  assert.equal(parseToolchainVersion("hmapdev 1.3.0-dev"), "1.3.0-dev"); // 开发构建的后缀要带出来
  assert.equal(parseToolchainVersion("hmapdev v1.2.0"), "1.2.0");
});

test("compareVersions 是数值比较（1.2.10 > 1.2.9）", () => {
  assert.ok(compareVersions("1.2.10", "1.2.9") > 0);
  assert.ok(compareVersions("1.2.0", "1.2.0") === 0);
  assert.ok(compareVersions("0.8.0", "1.2.0") < 0);
});

test("状态栏文本包含插件、声明 SDK 与工具链版本", () => {
  assert.equal(statusBarText(good, "1.2.0"), "memo · SDK 1.2.0 · hmapdev 1.2.0");
  assert.equal(statusBarText({ ...good, sdk: undefined }, ""), "memo · SDK 未声明 · hmapdev 未找到");
});
