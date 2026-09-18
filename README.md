# ccodex-sleep-state

**给 Codex 的降智和限流问题提供一种本地解决思路。** 目前只做 `gpt-6-astra`，优先支持 Windows，也能在 macOS 上用。

如果你用 Codex 时遇到回答质量突然变差、请求频繁受限，可以试试从代理出口和会话状态这两处入手。这个项目把订阅解析、出口探测、state 采集和注入做成了一个 Go 程序：填好自己的订阅或代理，启动一个服务，后面交给它处理。

不保证百分百有效。不同账号、出口和上游策略的表现可能不一样，把它当成一个可以自己部署、观察和验证的方案就好。

## 它怎么处理

程序会从你提供的出口里采集 `X-Codex-Turn-State`，按本地经验基线筛选，留一份正在用的，再准备一份备用。Codex 发请求时，服务注入当前 state，并使用它对应的出口；临近过期或连续出现异常信号时，再补下一份。

过去需要分别挂着的采集、转发和状态维护，现在放进同一个进程。服务启动时会备份并配置 Codex，正常退出时恢复。你的其他应用不用跟着切代理。

这里尝试改善的是可能与出口、会话状态有关的降智和请求受限。账号额度已经用完，或者没有模型权限，仍要等恢复或处理账号问题；程序收到 401、403、429 会停下本轮探测。state 的形状也只是筛选依据，最终有没有改善，还是看实际使用。

## 先跑起来

从 [Releases](https://github.com/gylive/ccodex-sleep-state/releases) 下载对应系统的压缩包。Windows 常见电脑选 `windows-amd64`，Windows ARM 选 `windows-arm64`；Mac 按芯片选 `darwin-arm64` 或 `darwin-amd64`。如果还没有 release，可以[从源码构建](docs/development.md)。

Windows PowerShell，在解压目录运行：

```powershell
.\ccodex-sleep-state.exe init
.\ccodex-sleep-state.exe paths
```

`init` 只创建本程序的配置，不动 Codex。默认配置走直连；需要订阅或代理，先按[代理教程](docs/proxies.md)改好，再执行：

```powershell
.\ccodex-sleep-state.exe check
.\ccodex-sleep-state.exe serve
```

看到 `Listening` 后，**重启 Codex** 让它加载本地 provider。登录仍由 Codex 自己处理。第一条 Astra 请求到来后才会开始采集，因此第一轮可能比平时慢。

另开一个 PowerShell 看状态：

```powershell
.\ccodex-sleep-state.exe status
```

Mac 上把命令前缀换成 `./ccodex-sleep-state`。完整步骤见 [Windows](docs/windows.md) / [macOS](docs/macos.md)。

采集本身也会消耗一点模型额度。默认每轮最多尝试 6 个出口，找到两份合格候选就停；单次最多等 20 秒，同一凭据两轮之间至少间隔 180 秒。探测逐个进行。正常生成请求不会被本服务自动重放，但网络失败前上游可能已经处理过它。

## 支持什么

- **订阅：** Clash/Mihomo YAML、逐行 URI、Base64 URI 列表。
- **直填代理：** HTTP、HTTPS、SOCKS5，支持用户名密码；`socks5h` 作为 SOCKS5 的别名。
- **订阅内协议：** Shadowsocks、ShadowsocksR、VMess、VLESS、Trojan、Hysteria、Hysteria2、TUIC，使用内嵌 Mihomo 出站实现。
- **模型：** `gpt-6-astra`，包括生成和 compact 请求。
- **传输：** HTTP / SSE，暂不支持 WebSocket。
- **部署：** Windows 优先，兼容 macOS。不需要 Python、Docker、额外代理核心进程或管理员权限。

并不是所有代理协议都做过真实节点联调。HTTP CONNECT 和带认证的 SOCKS5 有本地端到端测试；其余协议依赖上游核心，并有部分格式/构造测试。[测试边界](docs/testing.md)里有具体说明。

## 账号和订阅留在你自己电脑上

仓库里只有代码和方法，没有作者的 Codex 配置、订阅、代理节点或账号数据。部署时用你自己的配置，不需要把这些东西交给别人。

认证头和完整 state 只留在内存，日志不记订阅链接、密码、对话正文和完整 token。程序没有遥测，也不上传日志。Codex 配置会在接管前备份；运行期间你又改过它，恢复时就停下来，不覆盖你的新修改。

排错时别直接把整个运行目录发出来，里面可能有你自己的配置和备份。具体的数据处理和权限边界见 [隐私说明](SECURITY.md)。

## 文件放在哪

| 内容 | Windows | macOS |
|---|---|---|
| 程序，建议位置 | `%LOCALAPPDATA%\Programs\ccodex-sleep-state` | `~/.local/bin` |
| 本程序配置与日志 | `%LOCALAPPDATA%\ccodex-sleep-state` | `~/Library/Application Support/ccodex-sleep-state` |
| Codex 配置 | `%USERPROFILE%\.codex\config.toml` | `~/.codex/config.toml` |
| Codex 恢复备份 | 与 `config.toml` 同目录 | 与 `config.toml` 同目录 |

设置过 `CODEX_HOME` 时跟随它；本程序也支持 `CCODEX_STATE_HOME` 或 `--data-dir`。以 `paths` 命令显示的路径为准。

## 想看代码，从这里读

```text
cmd/ccodex-sleep-state/  命令入口，不放业务逻辑
internal/settings/     配置与目录
internal/proxyroute/   订阅解析、协议适配、独立出站
internal/turnstate/    封装解析、active/ready、快照和晋升规则
internal/gateway/      请求校验、采集调度、流式转发
internal/codexconfig/  保留原排版的 TOML 修改、备份和恢复
internal/service/      单实例、进程生命周期、状态接口
internal/logbook/      定长轮换日志
```

[设计取舍](docs/architecture.md)解释了为什么回包不能直接更新 active、为什么不重试生成请求，以及为什么不自动切系统代理。

## 许可证

GPL-3.0，见 [LICENSE](LICENSE)。代理协议复用了 Mihomo，来源和依赖说明见 [THIRD_PARTY_NOTICES.md](THIRD_PARTY_NOTICES.md)。这是独立项目，不隶属于 OpenAI 或 Mihomo。
