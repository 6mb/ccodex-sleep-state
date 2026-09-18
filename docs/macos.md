# macOS：安装、启动和恢复

先在 Codex 里登录，再准备自己的订阅或代理。这个程序没有安装器，把可执行文件放好就能用。当前还是早期测试版，先用一个简单问题验证链路；[测试记录](testing.md)会说明实际跑到了哪一步。

Apple 芯片选 `darwin-arm64`，Intel 芯片选 `darwin-amd64`。到 [Releases](https://github.com/gylive/ccodex-sleep-state/releases) 下载压缩包（没有发布包时可[从源码构建](development.md)），核对来源和 SHA256，然后解压。示例假定你已经在解压目录：

```sh
mkdir -p "$HOME/.local/bin"
install -m 755 ./ccodex-sleep-state "$HOME/.local/bin/ccodex-sleep-state"
"$HOME/.local/bin/ccodex-sleep-state" init
"$HOME/.local/bin/ccodex-sleep-state" paths
```

二进制没有 Apple Developer 公证。如果系统拦截，请先核验来源，再使用系统提供的单应用放行流程；不需要关闭整个系统的 Gatekeeper。

配置默认在：

```text
~/Library/Application Support/ccodex-sleep-state/config.json
```

先退出 Codex；用 CCS 的，先选好配置并暂停切换。启动：

```sh
"$HOME/.local/bin/ccodex-sleep-state" serve
```

从终端复制管理面板地址和管理口令，默认打开 `http://127.0.0.1:17841/admin/`。在“订阅与代理”里填本地代理、订阅链接或本地订阅文件，测试后点应用；到“路由”单独测试连接。处理完概览里的配置提示，再打开 Codex。

[面板教程](web-panel.md)写了每个按钮的用途。关闭注入仍经过本服务，API / 中转通道始终不采官方 state。使用 profile 的，按同名 profile 启动 Codex；运行期间不要在 CCS 切换配置。服务不读取现有代理软件配置、不切换策略组、不改系统代理；你的其他应用仍使用原来的网络设置。

另开终端查看状态：

```sh
"$HOME/.local/bin/ccodex-sleep-state" status
```

退出用 Ctrl+C。崩溃后重新启动会先检查旧事务，只在哈希匹配时自动恢复；被其他工具改过则保留现场。只想恢复而不启动服务：

```sh
"$HOME/.local/bin/ccodex-sleep-state" restore
```

然后重启 Codex。服务不安装 LaunchAgent，终端关掉并不等于已经安全恢复配置。配置冲突的处理、状态字段和常见问题与 [Windows 教程](windows.md)相同。面板应用来源可热切换；直接编辑 JSON 后仍需重启。

## 在独立目录里试，不碰日常 Codex

最稳妥的是跑仓库自带测试。想手工检查启动行为，可以只关掉配置接管：

```sh
"$HOME/.local/bin/ccodex-sleep-state" init --data-dir "$HOME/ccodex-trial"
"$HOME/.local/bin/ccodex-sleep-state" serve --data-dir "$HOME/ccodex-trial" --no-config
```

这不会改 `~/.codex/config.toml`，也不会自动从你的常用 Codex 接到请求。只启动进程不会采集 state；订阅配置如存在，仍会在启动时下载。测试后按 Ctrl+C 停止。不要把“没有接管 Codex”误认为“禁止联网”。

## 使用独立配置文件

`--config` 指定本工具的 JSON，不是 Codex 的 TOML。选项放在子命令之后：

```sh
"$HOME/.local/bin/ccodex-sleep-state" init --data-dir "$HOME/ccodex-trial" --config "$HOME/ccodex-trial/local.json"
"$HOME/.local/bin/ccodex-sleep-state" serve --data-dir "$HOME/ccodex-trial" --config "$HOME/ccodex-trial/local.json" --no-config
```

初始化目标已存在会拒绝覆盖。查询状态或恢复时使用同一个 `--data-dir`。正常接管跟随所选 Codex provider；`--no-config` 模式则按 JSON 中的 `upstream` / `upstream_kind` 工作，务必自己确认上游与凭据匹配。
