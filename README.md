# ccodex-sleep-state

**一个尝试解决 Codex 降智和限流的本地工具，目前只做 Astra。**

做这个项目的起点很简单：Codex 用着用着，回答质量不对了，或者请求开始频繁受限。我们在本地从代理出口和 turn-state 入手做了一些调整，最近的使用感受有所改善，于是把这套做法整理出来，方便大家自己部署、一起验证。

它不是百分百有效的修复，也没有足够的对照数据证明改善一定来自哪一步。开源的是一套正在尝试的办法，不是“装上就永不降智”的保证。

程序用 Go 写成，只需要启动一个服务：导入你自己的订阅或代理，采集和维护 state，再把它注入 Codex 的请求。不用分别照看几份脚本，也不用让电脑上的其他软件跟着切代理。

[Windows 上手](docs/windows.md) · [macOS 上手](docs/macos.md) · [订阅和代理](docs/proxies.md) · [测试进展](docs/testing.md) · [群聊交流](#一起试一起反馈)

> **真实链路已经跑通，首版仍按公开测试版发布。** 保持默认 10 块规则，从真实 AnyTLS 出口采到 292 后，两次 Codex CLI 请求均完成注入并收到回复。Windows、macOS 的自动测试与实际使用验证分开记录，见[测试记录](docs/testing.md)。

## 适合谁用

你正在用 Codex 的 **Astra**，碰到了降智或频繁限流，手里有自己的订阅、HTTP 或 SOCKS5 代理，也愿意花几分钟配置和观察，那可以试试。

如果只是账号额度用完了，或者账号本来就没有 Astra 权限，这个程序帮不了你。它不会增加额度，也不会替你开通模型。

## 它实际做了什么

```text
Codex → 本地服务 → 你的代理出口 → Astra
           │
           └─ 采集 state，保留当前值和备用值，按需注入请求
```

这里的 state 指 `X-Codex-Turn-State`。程序从候选出口发出短请求，按封装、时间和密文块数筛选返回的 state，留一份使用、一份备用。后续 Codex 请求会带上当前 state，并走它对应的出口。临近估计过期时间或出现连续异常时，再尝试补充。

这些筛选规则来自本地尝试，**不是官方的质量指标**。拿到符合规则的 state，只说明程序筛选通过；到底有没有改善，要看你实际做事时的表现。

正常启动时，它会备份并修改 Codex 配置，让请求接到本地服务；正常退出时恢复。它不修改模型、不读取其他人的账号，也不自动切换系统代理。

## 先跑起来

先在 Codex 里完成登录。到 [Releases](https://github.com/gylive/ccodex-sleep-state/releases) 下载对应系统的包；如果还没有发布包，可以[从源码构建](docs/development.md)。

- **大多数 Windows 电脑：** `windows-amd64`
- **Windows ARM 电脑：** `windows-arm64`
- **Apple 芯片 Mac：** `darwin-arm64`
- **Intel Mac：** `darwin-amd64`

Windows 解压后，在文件夹地址栏输入 `powershell`，运行：

```powershell
.\ccodex-sleep-state.exe init
.\ccodex-sleep-state.exe paths
```

这一步只创建本工具的配置，还没接管 Codex。接着按[订阅和代理教程](docs/proxies.md)填好自己的出口。默认是直连；不能直连的话，别跳过这一步。

退出 Codex，然后运行：

```powershell
.\ccodex-sleep-state.exe check
.\ccodex-sleep-state.exe serve
```

看到 `Listening` 后重新打开 Codex，发一条 Astra 请求。第一次会先采集 state，可能要多等一会儿。**运行期间保留这个终端窗口。**

另开一个 PowerShell，可以查看服务状态：

```powershell
.\ccodex-sleep-state.exe status
```

用完回到服务窗口按 **Ctrl+C**，再重启 Codex。意外关掉终端或进程崩溃后，用 `restore` 恢复配置。[Windows 完整教程](docs/windows.md)写了每一步应该看到什么，以及失败后怎么处理。

Mac 用户看 [macOS 教程](docs/macos.md)，命令前缀换成 `./ccodex-sleep-state`，其余用法相同。

## 用之前知道这几件事

- **采集会用到模型额度。** 默认每轮最多试 6 个出口，首条请求拿到一份合格 state 就继续转发，备用值留到后台补；同一轮不会重复试同一个出口。单次探测最多 20 秒，两轮至少间隔 180 秒。
- **不会不停重发你的问题。** 正常生成请求不由本服务自动重放。遇到 401、403、429 会停止本轮探测；权限、额度和速率限制需要分别处理。
- **目前只走 HTTP / SSE。** 暂不支持 WebSocket，也不支持其他模型。
- **改了订阅要重启服务。** 现在只在启动或 `check` 时下载订阅，不做后台热更新。
- **先小范围试。** 真实代理、客户端版本和长会话的表现还需要反馈；已测过和没测过的内容都放在[测试记录](docs/testing.md)里。

## 能接哪些代理

订阅支持 Clash/Mihomo YAML、逐行 URI 和 Base64 URI 列表。已有代理软件的，可以直接填它的 HTTP、HTTPS 或 SOCKS5 地址，支持用户名密码。

订阅中的 AnyTLS、SS、SSR、VMess、VLESS、Trojan、Hysteria、Hysteria2、TUIC 由内嵌的 Mihomo 出站适配器处理，不需要另起代理核心。这里只读取节点，不导入订阅里的规则、DNS、TUN 或代理组。

协议实现支持与真实节点跑通是两回事。具体格式、例子和限制见[代理教程](docs/proxies.md)。

## 账号、订阅要交给谁

不需要交给项目作者。程序在你自己电脑上运行，使用你自己的 Codex 登录和出口。

仓库里不包含作者的账号、订阅、节点或个人配置。程序没有遥测，不上传日志；认证头和完整 state 只留在内存，常规日志不记录对话正文、订阅链接或密码。

它会把 Codex 的认证头和请求转发到你配置的上游，所以不要随手把 `upstream` 改成别人给的地址。排错时也别把整个数据目录打包发群里，里面可能有自己的代理配置和 Codex 备份。[隐私说明](SECURITY.md)

## 文件一般放在哪

| 内容 | Windows | macOS |
|---|---|---|
| 程序，建议位置 | `%LOCALAPPDATA%\Programs\ccodex-sleep-state` | `~/.local/bin` |
| 本工具的配置和日志 | `%LOCALAPPDATA%\ccodex-sleep-state` | `~/Library/Application Support/ccodex-sleep-state` |
| Codex 配置 | `%USERPROFILE%\.codex\config.toml` | `~/.codex/config.toml` |
| 恢复备份 | 与 Codex 的 `config.toml` 同目录 | 与 Codex 的 `config.toml` 同目录 |

如果设置过 `CODEX_HOME`，会跟随它。本工具也支持 `CCODEX_STATE_HOME` 和 `--data-dir`；不确定时运行 `paths`，以输出为准。

## 一起试，一起反馈

用起来有没有改善、哪个版本接不上、什么情况下又出问题，都欢迎来聊。报错和复现步骤也可以提 [Issue](https://github.com/gylive/ccodex-sleep-state/issues)，方便后面查找。

| 微信群 · 爱的交流 | QQ 群 · 此间大梦无边 |
|:---:|:---:|
| <img src="docs/assets/wechat-group-2026-09-18.jpg" alt="微信群“爱的交流”二维码，图中标注 9 月 25 日前有效" width="280"> | <img src="docs/assets/qq-group.jpg" alt="QQ 群“此间大梦无边”二维码，群号 754842541" width="280"> |
| 图中标注 **9 月 25 日前有效**；过期后请先用 QQ 群入口。 | 扫码，或搜索群号 **754842541**。 |

反馈时带上系统、Codex 版本、工具版本和错误提示就够了。**不要发账号凭据、完整订阅链接或未经检查的配置文件。**

## 想改代码

命令入口在 `cmd/ccodex-sleep-state`，其余按职责拆在 `internal`：

- `proxyroute`：订阅解析和出站连接。
- `turnstate`：state 筛选、当前值、备用值和切换规则。
- `gateway`：请求校验、采集调度、流式转发。
- `codexconfig`：配置修改、备份和恢复。
- `service`：服务启停、单实例和状态接口。
- `settings`、`logbook`：配置目录与日志。

[构建方法](docs/development.md) · [设计取舍](docs/architecture.md)

## 许可证

GPL-3.0，见 [LICENSE](LICENSE)。代理部分复用了 Mihomo，见 [依赖说明](THIRD_PARTY_NOTICES.md)。本项目与 OpenAI、Mihomo 没有隶属关系。
