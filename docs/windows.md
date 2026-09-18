# Windows 使用教程

先把你自己的订阅或代理接进来，再启动服务，让 Codex 通过它发请求。整个过程不需要装 Go，也不用开管理员终端。Codex 继续使用你原来的登录。

## 1. 放好程序

在 Releases 选适合电脑架构的 Windows ZIP。解压到下面这个位置比较省心：

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

## 2. 填自己的出口

默认配置走直连。用订阅的读[订阅导入](proxies.md)，已有本地代理软件的只需填它提供的 HTTP 或 SOCKS5 地址，不需要导出整份代理配置。

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

## 4. 看它是否接上了

另开一个 PowerShell，同样进入程序目录：

```powershell
.\ccodex-sleep-state.exe status
```

几个字段够用了：

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
| `state_unavailable` | 查看探测状态码、订阅有效性和代理连接；等冷却期结束，不要连续重发 |
| HTTP 401 / 403 | 在 Codex 中处理登录、账号权限或上游拒绝，不会继续换 IP 撞 |
| HTTP 429 | 等额度或速率限制恢复；程序不会重置额度 |
| `state_shape_changed` | 这是封装基线变化，不是确定的质量结论；请求不会自动重放，上游可能已经消耗额度 |
| WebSocket 426 | 确认使用生成的 provider 配置并重启 Codex；此版本只走 HTTP/SSE |
| `Encrypted content...` | 上游密文或会话链可能变了；先保留工作，再考虑新建任务，不会自动删除上下文链 |

需要重置的是这个工具时，先正常退出并恢复配置，再决定是否保留它的数据目录。不要删除 Codex 的 `auth.json` 或整个 `.codex` 文件夹。
