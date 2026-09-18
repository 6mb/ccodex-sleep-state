# Windows：从解压到接上 Codex

准备好已经登录的 Codex，以及你自己的订阅或代理。下面按第一次使用来写，不需要装 Go，也不用管理员权限。

先说明当前进度：这是早期测试版，跑通本地测试不等于所有人的 Codex 都能接上。第一次先用一个简单问题验证，别直接拿正在赶工的长任务试。最新结果看[测试记录](testing.md)。

## 1. 放好程序

到 [Releases](https://github.com/gylive/ccodex-sleep-state/releases) 下载 Windows ZIP。大多数电脑选 `windows-amd64`；只有 ARM 设备选 `windows-arm64`。如果页面暂时没有发布包，可以等待发布，或按[构建教程](development.md)自己编译。

建议解压到：

```text
%LOCALAPPDATA%\Programs\ccodex-sleep-state
```

也可以放别处，但别把程序放进订阅同步盘或公共共享目录。进入解压目录，在资源管理器地址栏输入 `powershell` 打开终端。

```powershell
.\ccodex-sleep-state.exe version
.\ccodex-sleep-state.exe init
.\ccodex-sleep-state.exe paths
```

如果 `init` 说配置已存在，它不会覆盖，继续用原来的文件即可。

发布二进制未做商业代码签名。若 Windows 给出来源提示，先核对 GitHub 仓库、版本和 SHA256；不要为了运行它全局关闭 Defender 或 SmartScreen。

## 2. 选一种接法

默认配置是直连。按你的情况选一种就行：

- **有订阅链接：** 按[订阅导入](proxies.md#用订阅链接)填写。程序会自己解析节点。
- **已经开着代理软件：** 找到软件提供的 HTTP 或 SOCKS5 监听地址，按[已有代理](proxies.md#已有-http--socks5-代理)填写。不需要导出整份配置。
- **本机能直接访问上游：** 先保留默认值。

打开本工具的配置：

```powershell
notepad "$env:LOCALAPPDATA\ccodex-sleep-state\config.json"
```

改过 `--data-dir` 的话，打开 `paths` 显示的位置。`check` 会下载和解析订阅、构造代理，但不会登录账号，也不会给模型发请求：

```powershell
.\ccodex-sleep-state.exe check
```

如果这里报错，按提示检查对应的订阅或节点。通过后再启动服务。

## 3. 启动并重启 Codex

先退出 Codex，再运行：

```powershell
.\ccodex-sleep-state.exe serve
```

服务先占用本地端口，加载出口，再备份和修改 Codex 配置。默认地址是 `127.0.0.1:17841`，只有本机可连接。

看到 `Listening` 后重新打开 Codex。模型配置固定为 `gpt-6-astra`；已有任务或项目可能有自己的模型/provider 覆盖，请检查这些覆盖项，必要时新建一个普通 Astra 任务测试。这个程序不会删除项目规则或强行改已有任务。

第一次请求要先等程序采集 state，可能会慢一些。默认每轮最多尝试 6 个出口，每个出口最多等 20 秒；等这一轮结束再看结果。

这个窗口就是服务，使用期间保持打开。暂时没有开机自启，关机后下次再运行一次 `serve`。

## 4. 别只看窗口没报错，要确认请求进来了

另开一个 PowerShell，同样进入程序目录：

```powershell
.\ccodex-sleep-state.exe status
```

先看 `sessions`：发过 Astra 请求后，这里应该出现会话。再看 `usable`：为 `true` 表示已经采到了符合本地规则的 state。两者都正常、Codex 也收到了回复，才说明这次请求走通了；这仍不等于已经证明回答质量提高。

其他字段排错时再看：

- `routes`：可用配置中的出口数量，不代表每个出口已经联网成功。
- `sessions`：服务在内存中记住的凭据会话；刚启动时为空是正常的。
- `usable`：当前 state 是否符合配置的封装/时间基线，不是智力评分。
- `ready`：有没有仍在有效期内的备用值。
- `remaining_seconds`：按配置 TTL 算出的估计剩余时间，不是上游承诺。
- `strikes`：当前版本连续出现非基线形状的次数。

日志默认在 `%LOCALAPPDATA%\ccodex-sleep-state\logs`。每份约 2 MiB，最多保留当前文件加 3 份历史文件。

如果 `sessions` 一直是空的，说明服务还没收到 Codex 的请求。先重启 Codex，再检查项目配置或启动参数有没有指定别的 provider；也可以新建一个 Astra 任务试一下。不同客户端版本对自定义 provider 的支持可能有差别。

## 5. 停止与恢复

回到服务窗口按 **Ctrl+C**。正在进行的转发会被取消，请尽量等当前任务结束再停。没有人在运行期间改过配置时，原文件会按字节恢复，备份保留。

不要直接关窗口作为日常退出方式。断电、任务管理器强制结束或终端被关闭后，执行：

```powershell
.\ccodex-sleep-state.exe restore
```

然后重启 Codex。`restore` 发现服务还在运行会拒绝操作；发现配置被你或其他工具改过，也会拒绝覆盖。

遇到冲突，打开 `paths` 找到 Codex 配置和同目录的 `config.toml.sleep-state-*.bak`。**先另存当前文件**，比较新旧内容，再手工保留需要的改动。恢复记录在本程序数据目录的 `config-transaction.json`，里面指向对应备份；不要随手删除记录掩盖问题。若决定完全恢复备份，请确认当前文件与备份相同后再运行 `restore`，它会收尾。

## 常见情况

| 表现 | 先检查 |
|---|---|
| 端口被占用 | 是否已经开过一个服务；不要同时开两个终端重复 `serve` |
| `state_unavailable` | 先看日志里的 `result`、`state_blocks` 和 `expected_blocks`；请求成功但块数不符也会报这个错。见[实测问题](testing.md#遇到同样的-503-怎么看)，不要连续重发 |
| HTTP 401 / 403 | 在 Codex 中处理登录、账号权限或上游拒绝，不会继续换 IP 撞 |
| HTTP 429 | 等额度或速率限制恢复；程序不会重置额度 |
| `state_shape_changed` | 这是封装基线变化，不是确定的质量结论；请求不会自动重放，上游可能已经消耗额度 |
| WebSocket 426 | 确认使用生成的 provider 配置并重启 Codex；此版本只走 HTTP/SSE |
| `Encrypted content...` | 上游密文或会话链可能变了；先保留工作，再考虑新建任务，不会自动删除上下文链 |

需要重置的是这个工具时，先正常退出并恢复配置，再决定是否保留它的数据目录。不要删除 Codex 的 `auth.json` 或整个 `.codex` 文件夹。

还有问题，可以带着错误提示到 [Issues](https://github.com/gylive/ccodex-sleep-state/issues) 或[群里](../README.md#一起试一起反馈)交流。发之前先检查截图，别把订阅和登录信息一起带上。
