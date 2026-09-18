# ccodex-sleep-state

**一个尝试解决 Codex 降智和限流的本地工具，目前只做 Astra。**

做这个项目的起点很简单：Codex 用着用着，回答质量不对了，或者请求开始频繁受限。我们在本地从代理出口和 turn-state 入手做了一些调整，最近的使用感受有所改善，于是把这套做法整理出来，方便大家自己部署、一起验证。

它不是百分百有效的修复，也没有足够的对照数据证明改善一定来自哪一步。开源的是一套正在尝试的办法，不是“装上就永不降智”的保证。

程序用 Go 写成，只需要启动一个服务。现在可以在本地网页里导入订阅、填写代理、测试出口、选择路由，也能随时关闭注入。官方 ChatGPT 登录可以采集和维护 state；中转站与官方 API key 通道保留原来的认证方式，只做普通转发，不硬塞官方 state。不用分别照看几份脚本，也不用让电脑上的其他软件跟着切代理。

[管理面板怎么用](docs/web-panel.md) · [Windows 上手](docs/windows.md) · [macOS 上手](docs/macos.md) · [订阅和代理](docs/proxies.md) · [测试进展](docs/testing.md) · [群聊交流](#一起试一起反馈)

> **仍按公开测试版发布。** 旧版已跑通过默认 10 块规则的真实采集和注入；新增面板、CCS 配置适配与平台测试的具体结果，以[测试记录](docs/testing.md)为准。能编译 Windows 包，不等于所有 Windows 桌面版本都已实测。

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

先确认 Codex 原来就能用：官方账号完成登录，中转站填好自己的 API key；用 CCS 管理配置的，先在 CCS 中选好这次要用的那份。然后退出 Codex，暂时不要切换配置。

到 [Releases](https://github.com/gylive/ccodex-sleep-state/releases) 下载对应系统的包：

- **大多数 Windows 电脑：** `windows-amd64`
- **Windows ARM 电脑：** `windows-arm64`
- **Apple 芯片 Mac：** `darwin-arm64`
- **Intel Mac：** `darwin-amd64`

Windows 解压后，在文件夹地址栏输入 `powershell`，运行：

```powershell
.\ccodex-sleep-state.exe init
.\ccodex-sleep-state.exe serve
```

第一次 `init` 创建的是本工具的配置；已有配置不会被覆盖。`serve` 才会备份并接管 Codex。

终端会显示本地服务地址、**管理面板地址**和**管理口令**。默认在浏览器打开 [http://127.0.0.1:17841/admin/](http://127.0.0.1:17841/admin/)，粘贴口令进入。口令每次启动都会换，不是你的 Codex 密码，也别发到群里。

1. 到“订阅与代理”，选择直连、本地代理、订阅链接或本地订阅文件。
2. 有代理软件的，填它实际的 HTTP / SOCKS5 地址，例如 `socks5://127.0.0.1:7897`。**7897 只是例子，以你软件里的端口为准。**
3. 点测试，确认格式能解析；再点应用。测试通过不会自动保存。
4. 到“路由”，加载出口，单独测试连接。连接通和能采到 state 是两回事。
5. 回到概览，处理完配置提示，再打开 Codex，发一个简短的 Astra 问题。

如果是官方登录且开启注入，第一条请求会先采集 state，可能要等一会儿；如果是 API / 中转通道，或你已经关闭注入，就直接转发。**运行期间保留终端窗口。**

停止时回到终端按 **Ctrl+C**，等恢复完成，再重启 Codex。完整操作见 [Windows 教程](docs/windows.md)、[macOS 教程](docs/macos.md)；不想改 Codex 配置的试用方法也写在里面。

## 用 CCS、中转站或官方 API，怎么接

CCS 是配置管理工具，不是另一种模型协议。程序读取当前选中的 provider，判断请求原本要发给谁，再临时把入口改成本地服务；不会导入 CCS 的账号库。启动时可能只读所选 Codex 目录的 `auth.json` 判断认证类型，不返回密钥、不改文件、不刷新登录。

| 原来的接法 | 接入后怎么走 |
|---|---|
| 官方 ChatGPT 登录、Codex 官方后端 | 可以开启 state 采集与注入，也可以手动关闭 |
| 中转站、第三方 Responses API | 沿用原上游和 API key 认证方式；不采集、不注入官方 state |
| OpenAI 官方 API key | 按 API 通道转发，不当成 ChatGPT 订阅登录 |
| Codex profile | 在本工具设置 `codex_profile`，Codex 也要用同名 profile 启动 |

**中转通道能接进来，不代表 state 方案能改善它。** 这部分解决的是配置兼容、出口选择和排错，不会替中转站提供 Astra，也不会转换成别的模型或补齐不支持的接口。上游仍需支持 Astra 和 Responses HTTP/SSE。

不要在服务运行中继续点 CCS 切换配置。正确顺序是：停止服务并恢复 → 在 CCS 切换 → 重新启动服务 → 重启 Codex。检测到配置被外部改动时，程序会拒绝继续转发，避免凭据发错地方；不会强行覆盖你的新配置。[具体说明](docs/web-panel.md#ccs-和-profile-怎么配合)

## 用之前知道这几件事

- **采集会用到模型额度。** 默认每轮最多试 6 个出口，首条请求拿到一份合格 state 就继续转发，备用值留到后台补；同一轮不会重复试同一个出口。单次探测最多 20 秒，两轮至少间隔 180 秒。
- **不会不停重发你的问题。** 正常生成请求不由本服务自动重放。遇到 401、403、429 会停止本轮探测；权限、额度和速率限制需要分别处理。
- **目前只走 HTTP / SSE。** 暂不支持 WebSocket，也不支持其他模型。
- **从面板应用出口，不用重启。** 没有请求执行时切换，旧 state 一并清空。直接用编辑器改 JSON 则仍需重启；程序不会覆盖外部改动。收到上游认证或限流拒绝时，不能借换出口清掉暂停状态。
- **先小范围试。** 真实代理、客户端版本和长会话的表现还需要反馈；已测过和没测过的内容都放在[测试记录](docs/testing.md)里。

## 报 503，先别来回重试

503 不只有一种原因。看错误码，比只看数字有用：

| 错误 | 是什么意思 | 下一步 |
|---|---|---|
| `state_unavailable` | 本地没采到合格 state，尚未转发这次正式请求 | 看出口是否连通、探测结果与块数；等冷却，或自己决定关闭注入 |
| `state_shape_changed` | 上游回包形状不符合当前基线 | 这次请求可能已经消耗额度，服务不会自动再发一遍 |
| `service_not_ready` | 出口配置或 Codex 配置恢复还没处理好 | 打开面板看具体提示 |
| 上游返回的 503 | 上游服务暂时不可用 | 看上游状态；不要靠改块数把错误藏起来 |

本地与上游错误还会用 `X-Sleep-State-Error-Source` 响应头区分。292 是当前 10 块规则对应的一种长度，**不是官方“满血证明”**；没拿到也不能只凭这一点断定代理不行。[面板排错指南](docs/web-panel.md#出了问题先看哪里)

## 能接哪些代理

订阅支持 Clash/Mihomo YAML、逐行 URI 和 Base64 URI 列表。已有代理软件的，可以直接填它的 HTTP、HTTPS 或 SOCKS5 地址，支持用户名密码。

订阅中的 AnyTLS、SS、SSR、VMess、VLESS、Trojan、Hysteria、Hysteria2、TUIC 由内嵌的 Mihomo 出站适配器处理，不需要另起代理核心。这里只读取节点，不导入订阅里的规则、DNS、TUN 或代理组。

协议实现支持与真实节点跑通是两回事。具体格式、例子和限制见[代理教程](docs/proxies.md)。

## 账号、订阅要交给谁

不需要交给项目作者。程序在你自己电脑上运行，使用你自己的 Codex 登录和出口。

仓库里不包含作者的账号、订阅、节点或个人配置。程序没有遥测，不上传日志；认证头和完整 state 只留在内存，常规日志不记录对话正文、订阅链接或密码。

普通接管会跟随当前 Codex provider 的上游与认证方式；中转不会借用官方登录。使用 `--no-config` 自己指定上游时，你仍需要确认地址和 API key 的归属，不要照抄不明配置。排错时也别把整个数据目录打包发群里，里面可能有自己的代理配置和 Codex 备份。[隐私说明](SECURITY.md)

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
- `service`：服务启停、单实例、Web 管理面板和配置热切换。
- `settings`、`logbook`：配置目录与日志。

[构建方法](docs/development.md) · [设计取舍](docs/architecture.md)

## 许可证

GPL-3.0，见 [LICENSE](LICENSE)。代理部分复用了 Mihomo，见 [依赖说明](THIRD_PARTY_NOTICES.md)。本项目与 OpenAI、Mihomo 没有隶属关系。
