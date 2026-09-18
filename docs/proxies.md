# 把自己的订阅或代理接进来

有订阅链接，直接导入订阅；已经有可用的 HTTP 或 SOCKS5 代理，也可以只填代理地址。它们都只给这个程序用，不会改变其他应用的网络设置。

Clash/Mihomo 订阅读取顶层 `proxies` 里的实际节点，规则、DNS、TUN 和代理组不一起导入。如果拿到的是只有 `proxy-providers` 的配置，需要换成包含节点的订阅。

## 先用面板，少敲几行命令

启动 `setup`（已有配置也可 `serve`）后，打开终端显示的管理面板，用同一终端的管理口令进入。到“订阅与代理”选来源，填地址或文件路径，测试通过后点应用。已经开着代理软件的，通常直接填本地 HTTP / SOCKS5 地址最省事。

这里有两个不同的测试：

- **来源测试：** 读取或下载、解析、构建出站。不会发送模型请求，也不证明节点能连通。
- **路由连接测试：** 通过指定出口给所选上游发不带登录信息的请求。收到 HTTP 响应只说明网络有回应，不证明能采到符合当前个人 / Team 规则的 state。

面板应用会替换整组来源，备份旧配置并清空 state；不需要重启。想同时配置多个来源，或不把凭据写进 JSON，就继续看下面的手动方法。[完整面板教程](web-panel.md)

## 本地订阅文件与本机订阅接口

面板选“本地订阅文件”，填完整路径；JSON 形式如下：

```json
{
  "direct": false,
  "subscriptions": [
    { "file": "C:\\Users\\你的用户名\\Downloads\\nodes.yaml" }
  ]
}
```

把示例路径换成自己的实际文件。macOS 用绝对路径。只读取普通文件，支持与远程订阅相同的 YAML / URI / Base64 格式；不会修改原文件，不允许与同条来源的 `url` / `url_env` 混用。

本机程序提供的 HTTP 订阅，在面板选“订阅链接”。地址用 `http://127.0.0.1:端口/路径`，IPv6 可以用 `[::1]`。HTTP 仅支持字面值回环地址，不接受 `localhost` 或局域网地址；远程订阅仍必须 HTTPS。

## 用订阅链接

订阅链接通常自带访问凭据，别贴到 Issue 或群里。下面用环境变量传给程序，不把链接写进配置。每次新开终端需要重新输入一次。

在 PowerShell 运行下面三行，再按提示粘贴订阅链接；输入内容不会显示，也不会写进命令历史：

```powershell
$secret = Read-Host "粘贴订阅链接" -AsSecureString
$env:CCODEX_SUBSCRIPTION = [System.Net.NetworkCredential]::new("", $secret).Password
Remove-Variable secret
```

编辑本程序 `config.json` 的这几项，其他项保留：

```json
{
  "direct": false,
  "proxy_urls": [],
  "proxy_envs": [],
  "subscriptions": [
    { "url_env": "CCODEX_SUBSCRIPTION" }
  ]
}
```

这是可以独立使用的最小配置；缺少的字段会使用默认值。接着在**同一个 PowerShell 窗口**执行 `check` 和 `serve`。环境变量不会自动传给之前打开的另一个终端，也不会跨重启保留。

macOS 默认 zsh 可以用：

```sh
read -rs 'CCODEX_SUBSCRIPTION?粘贴订阅链接：'
export CCODEX_SUBSCRIPTION
printf '\n'
```

也支持 `{"url":"https://HOST/PATH?token=TOKEN"}` 的本地配置形式，但明文文件更容易被同步、截图或误提交。不要同时填写 `url` 和 `url_env`。

远程订阅 URL 只接受 HTTPS；回环 HTTP 也可以接本机程序提供的订阅。最大 2 MiB，最多 16 个订阅，合并后最多 256 个出口。无效项会终止导入，并显示订阅/节点序号，不显示私有 URL 或节点名称。

**启动、`check` 以及面板里的来源/路由操作会按需读取订阅，不做后台定时更新。** 用面板应用新来源可热切换，旧 state 会一起清空；直接改 JSON 则重启生效。不要把订阅文件改了等同于服务已经重新载入。

## 下载到了配置，却没有节点？

有的订阅会按下载客户端的 `User-Agent` 返回不同内容。同一个链接，可能给一种客户端完整节点，给另一种客户端空的 `proxies: []`。程序现在会明确报“服务端没有提供节点”，不会把它误报为 Base64 格式错误。

如果你的链接在 Clash Verge 能导入，在这里却拿不到节点，可以给这一条订阅指定下载标识，并只选自己想用的协议：

```json
{
  "direct": false,
  "subscriptions": [
    {
      "url_env": "CCODEX_SUBSCRIPTION",
      "user_agent": "clash-verge/v2.4.2",
      "include_protocols": ["anytls"],
      "exclude_keywords": ["香港", "台湾", "臺灣", "台灣", "澳门", "澳門", "Hong Kong", "Taiwan", "Taipei", "Macau", "Macao", "🇭🇰", "🇹🇼", "🇲🇴"]
    }
  ]
}
```

这是按需使用的例子，不是所有订阅都要照抄。`include_protocols` 不填就不筛协议；`exclude_keywords` 对节点名称和服务器地址做不区分大小写的包含匹配。过滤发生在建立出站适配器之前，被排除的节点不会参与采集。

**名称筛选不是出口定位。** 节点叫“英国”并不能证明流量一定从英国出去。需要确认实际出口时，还要另做检查。

程序不会因为某个节点要求 `skip-cert-verify: true` 就关闭证书校验。可以通过协议筛选排除这类节点，或请服务商提供能正常校验证书的配置。

## 已有 HTTP / SOCKS5 代理

先在你的代理软件里找到实际监听地址和端口，确认代理软件还在运行。不要照抄别人的端口。

在 PowerShell 这样输入完整地址：

```powershell
$secret = Read-Host "输入 HTTP 或 SOCKS5 代理地址" -AsSecureString
$env:CCODEX_PROXY = [System.Net.NetworkCredential]::new("", $secret).Password
Remove-Variable secret
```

地址格式如下。`HOST`、`PORT`、`USER`、`TOKEN` 换成你自己的值：

```text
http://HOST:PORT
https://HOST:PORT
socks5://HOST:PORT
socks5h://HOST:PORT
socks5://USER:TOKEN@HOST:PORT
http://USER:TOKEN@HOST:PORT
```

端口需要明确填写。用户名密码里的 `@`、`:`、`#`、`%` 等字符要做 URL 编码。配置引用变量名即可：

```json
{
  "direct": false,
  "proxy_envs": ["CCODEX_PROXY"],
  "subscriptions": []
}
```

macOS 默认 zsh 输入方式：

```sh
read -rs 'CCODEX_PROXY?输入 HTTP 或 SOCKS5 代理地址：'
export CCODEX_PROXY
printf '\n'
```

然后在同一个终端运行 `check` 和 `serve`。

不含凭据的本地代理地址也可以直接放在 `proxy_urls`。多个 URI 按数组顺序加入出口池。启用 `direct` 会把直连作为第一个出口；只想走代理，请保持 `false`。

HTTP/HTTPS 代理使用 CONNECT 隧道。SOCKS5 由代理接收目标域名。程序不会把失败的代理悄悄降级成直连，也不自动继承 `HTTP_PROXY` / `HTTPS_PROXY`，以免你不知道实际流量走了哪里。

## 下载订阅本身也需要代理

节点还没导入时不能用节点池下载它自己。准备一个独立的 HTTP/HTTPS/SOCKS5 代理 URI 环境变量，然后设置：

```json
{
  "subscription_proxy_env": "CCODEX_SUBSCRIPTION_PROXY"
}
```

这个代理只负责订阅下载，不会自动成为模型请求出口。下载失败返回的是经过脱敏的错误；不会把带 token 的 URL 塞进日志。

## 订阅格式与协议边界

| 格式 | 处理方式 |
|---|---|
| Clash/Mihomo YAML | 读取顶层 `proxies`；JSON 形式也可由 YAML 解析器处理 |
| 逐行代理 URI | 每行解析；任一非空、非注释行无效则报错 |
| Base64 URI 列表 | 支持标准与 URL-safe 字母表，有无 padding 均可 |

支持 HTTP、SOCKS5、AnyTLS、SS、SSR、VMess、VLESS、Trojan、Hysteria、Hysteria2 和 TUIC。HTTPS URI 被映射为启用 TLS 的 HTTP 代理。URI 别名与具体可用参数取决于锁定版本的 Mihomo 转换器；不要把“支持协议”理解为支持任意订阅服务商自定义格式。

SSH、WireGuard、代理组、链式 `dialer-proxy`、绑定系统接口/路由标记、从订阅加载本地证书或私钥都不开放。`skip-cert-verify: true` 会拒绝，避免订阅顺手关闭 TLS 校验。需要这些配置的节点，请换成能在严格校验下连接的出口。

## 探测是怎么跑的

以下仅适用于官方 ChatGPT 通道且开启注入；中转 / API 通道和关闭注入的请求不执行这套采集。

1. Codex 的第一条合法 Astra 请求提供转发认证头。启动时对认证文件的只读分类，不会将其中的密钥交给探测器或面板。
2. 如果没有可用 state，依次从候选出口发一个很短的独立请求，不带原会话正文和旧 state。
3. 完成的响应里，封装、时间和块数符合配置基线的值才进入 active / ready；首条请求拿到可用值就继续，不必等备用值。
4. 同一请求拿到快照后，固定从该 state 对应的出口转发。
5. 空闲时按冷却间隔补备用值，到续补窗口再更新。探测或正式请求遇到 401/403，会暂停这份凭据；遇到 429，按 Retry-After 和本地冷却时间暂停请求及探测，不通过切出口继续请求。

这一步是在为后续 Codex 请求找可用的出口和 state。它会消耗模型额度，也会用到代理流量；遇到账号权限或额度问题，先处理对应问题，再继续。
