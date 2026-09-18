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

直连可用就保留默认值；需要代理或订阅，按[代理教程](proxies.md)编辑。检查并启动：

```sh
"$HOME/.local/bin/ccodex-sleep-state" check
"$HOME/.local/bin/ccodex-sleep-state" serve
```

启动前先退出 Codex，看到 `Listening` 后再打开。服务不读取现有代理软件配置、不切换策略组、不改系统代理；你的其他应用仍使用原来的网络设置。

另开终端查看状态：

```sh
"$HOME/.local/bin/ccodex-sleep-state" status
```

退出用 Ctrl+C。崩溃后恢复：

```sh
"$HOME/.local/bin/ccodex-sleep-state" restore
```

然后重启 Codex。服务不安装 LaunchAgent，终端关掉并不等于已经安全恢复配置。配置冲突的处理、状态字段和常见问题与 [Windows 教程](windows.md)相同。

## 在独立目录里试，不碰日常 Codex

最稳妥的是跑仓库自带测试。想手工检查启动行为，可以只关掉配置接管：

```sh
"$HOME/.local/bin/ccodex-sleep-state" init --data-dir "$HOME/ccodex-trial"
"$HOME/.local/bin/ccodex-sleep-state" serve --data-dir "$HOME/ccodex-trial" --no-config
```

这不会改 `~/.codex/config.toml`，也不会自动从你的常用 Codex 接到请求。只启动进程不会采集 state；订阅配置如存在，仍会在启动时下载。测试后按 Ctrl+C 停止。不要把“没有接管 Codex”误认为“禁止联网”。
