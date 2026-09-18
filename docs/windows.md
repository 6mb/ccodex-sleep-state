# Windows：从解压到接上 Codex

不需要装 Go，也不用管理员权限。先确认 Codex 原来的官方登录或中转配置能用；用 CCS 的，先选好这次要用的配置。

当前仍是公开测试版。Windows 自动测试通过与每一种 Windows 桌面客户端都实测过，不是一回事。第一次先问一个简单问题，别直接拿正在赶工的长任务试。[测试记录](testing.md)会写明验证范围。

## 放好程序

到 [Releases](https://github.com/gylive/ccodex-sleep-state/releases) 下载 ZIP。大多数电脑选 `windows-amd64`，ARM 设备选 `windows-arm64`。先核对来源和 `SHA256SUMS`，再解压。建议位置：

```text
%LOCALAPPDATA%\Programs\ccodex-sleep-state
```

也可以放在其他自己的目录；不要放进公开共享目录。进入解压目录，在资源管理器地址栏输入 `powershell`，运行：

```powershell
.\ccodex-sleep-state.exe version
.\ccodex-sleep-state.exe init
.\ccodex-sleep-state.exe paths
```

`init` 只创建本工具的配置。如果提示已存在，不会覆盖；升级时一般继续使用原配置即可。发布文件未做商业代码签名，不要为了运行它全局关闭 Defender 或 SmartScreen。

## 启动，再到网页填写出口

退出 Codex，暂停 CCS 的配置切换，然后运行：

```powershell
.\ccodex-sleep-state.exe serve
```

终端会显示管理面板地址和管理口令。默认在浏览器打开 [http://127.0.0.1:17841/admin/](http://127.0.0.1:17841/admin/)，复制口令进去。

在“订阅与代理”里选一种：

- **本地代理：** 代理软件保持运行，填它实际提供的地址。例如 `socks5://127.0.0.1:7897`；端口不要照抄，以软件设置为准。
- **订阅链接：** 粘贴完整订阅 URL。它通常带凭据，不要截图发群。
- **本地订阅文件：** 粘贴文件完整路径，程序只读该文件。例如你自己 Downloads 里的 YAML。
- **直连：** 确实能直接访问所选上游时再选。程序不会自动替你继承系统代理。

点测试，再点应用。**测试只确认能读取、解析和构建配置，不会问模型，也不会自动保存。** 要看网络是否能通，再去“路由”测试连接。[面板完整教程](web-panel.md)

概览没有待处理的配置错误后，重新打开 Codex，选择 Astra，发一个短问题。

- 官方 ChatGPT + 开启注入：首次可能要等待采集，默认每轮最多 6 个出口、单次最多 20 秒。
- 中转 / 官方 API key：普通转发，不等 292，不会向官方采 state。
- 关闭注入：也直接转发，但仍经过本服务和选定出口。

服务运行期间保留终端窗口。没有安装 Windows 服务或开机自启，关机后下次需要再运行 `serve`。

## 确认请求真的走进来了

面板概览里，发过请求后应出现会话。也可以另开 PowerShell，在程序目录运行：

```powershell
.\ccodex-sleep-state.exe status
```

| 字段 | 怎么理解 |
|---|---|
| `upstream_kind` | `official` 是 ChatGPT 通道，`relay` 是 API / 中转通道 |
| `injection_enabled` | 当前是否开启注入，不代表是否接管配置 |
| `sessions` | 本地收到请求后建立的凭据会话；不是账号列表 |
| `phase` | `ready` 已有 state；`passthrough` 普通转发；`waiting_for_state` 尚未采到；`auth_blocked` / `rate_limited` 被上游拒绝或要求等待 |
| `routes` | 当前出口配置数，不代表每条已经联网验证 |
| `usable` | state 是否符合本地时间与封装规则，不是模型质量评分 |
| `retry_after_seconds` | 限流后需要等待的大致秒数 |

`sessions` 一直为空，先重启 Codex，再看有没有项目、任务或 profile 覆盖 provider。用 profile 的请看[CCS 与 profile](web-panel.md#ccs-和-profile-怎么配合)。本服务不会擅自删除这些覆盖项。

## 停止与恢复

回到服务窗口按 **Ctrl+C**，尽量等当前任务完成后再停。正常退出会恢复接管前的 Codex 配置，再重启 Codex 即可。关闭浏览器不会停止服务，关闭注入也不会恢复配置。

意外关掉终端后，新版再次启动会检查旧事务：只有文件哈希证明仍是自己接管的版本，才恢复后重新接管。**文件被你或 CCS 改过，就不覆盖。** 这解决的是意外退出留下的旧事务，不是让多个配置工具同时抢写文件。

只想恢复而不继续运行，先确认旧服务已经退出，再执行：

```powershell
.\ccodex-sleep-state.exe restore
```

旧版的 `unfinished config transaction; run restore before starting again` 是配置事务未完成，不是代理端口错误。先恢复，或升级后按面板提示处理，别反复开几个 `serve`。

遇到恢复冲突：

1. 用 `paths` 找到 Codex 配置，先把当前文件另存一份。
2. 找到同目录 `config.toml.sleep-state-*.bak`，比较差异，保留 CCS 或你自己的新改动。
3. 本工具数据目录的 `config-transaction.json` 指向恢复备份。不要随手删除它掩盖问题。
4. 确认已经手工恢复到对应备份内容后，再运行 `restore` 收尾。

## 常见报错

| 表现 | 先做什么 |
|---|---|
| 本地端口被占用 | 检查是否已有一个服务；不要重复启动 |
| 面板提示口令错误 | 复制当前终端里的新口令 |
| `state_unavailable` / 本地 503 | 看出口连通性和探测结果，等冷却或明确关闭注入；别连续重发 |
| `state_shape_changed` | 请求可能已经消耗额度，服务不会重放；检查 state 规则与上游变化 |
| 上游 503 | 查看上游是否可用，区别于本地没采到 state |
| 401 / 403 | 处理登录、权限或上游访问拒绝，不靠换 IP 继续撞 |
| 429 | 等额度或速率恢复；程序不会重置额度 |
| 426 | 此版本只走 HTTP/SSE，确认使用本服务生成的 provider 配置 |
| `Encrypted content...` | 可能涉及上游密文或会话链；保留工作后排查，不自动删上下文 |

日志在 `%LOCALAPPDATA%\ccodex-sleep-state\logs`，约 2 MiB × 4 轮换。反馈时发版本、错误码和脱敏后的相关行即可，不要把整个数据目录打包。

## 想手动配置，或只试面板

默认 JSON：

```powershell
notepad "$env:LOCALAPPDATA\ccodex-sleep-state\config.json"
.\ccodex-sleep-state.exe check
```

`check` 只读配置、读取或下载订阅并构建出站，不发送模型请求。修改 JSON 后需要重启；面板里的应用来源则可以直接生效。

独立试用且不接管日常 Codex：

```powershell
.\ccodex-sleep-state.exe init --data-dir "$env:LOCALAPPDATA\ccodex-trial"
.\ccodex-sleep-state.exe serve --data-dir "$env:LOCALAPPDATA\ccodex-trial" --no-config
```

`--no-config` 不修改 Codex 配置，也不会自动接到日常 Codex 的请求。它**不是禁止联网**：配置的订阅仍可能下载，面板网络测试也会联网。独立配置可用 `--config` 指定；选项放在子命令之后，`status` / `restore` 也要使用相同 `--data-dir`。

需要卸载时，先停止并恢复配置，再处理本工具目录。不要删除 Codex 的 `auth.json` 或整个 `.codex`。
