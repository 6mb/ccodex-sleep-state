"use strict";
const $ = (id) => document.getElementById(id);
let token = sessionStorage.getItem("sleep-state-control") || "";
let state = null;
let busy = false;
let noticeTimer;
function notice(text) {
  $("notice").textContent = text;
  $("notice").hidden = false;
  clearTimeout(noticeTimer);
  noticeTimer = setTimeout(() => {
    $("notice").hidden = true;
  }, 12000);
}
async function api(path, body) {
  const response = await fetch("/admin/api/" + path, {
    method: body === undefined ? "GET" : "POST",
    headers: {
      Authorization: "Bearer " + token,
      ...(body === undefined ? {} : { "Content-Type": "application/json" }),
    },
    cache: "no-store",
    redirect: "error",
    ...(body === undefined ? {} : { body: JSON.stringify(body) }),
  });
  const value = await response.json();
  if (!response.ok)
    throw new Error(value.error || "操作未完成，请检查服务终端。");
  return value;
}
async function action(fn) {
  if (busy) return;
  busy = true;
  document.querySelectorAll("button").forEach((button) => {
    button.disabled = true;
  });
  try {
    await fn();
  } catch (error) {
    notice(error.message);
  } finally {
    busy = false;
    document.querySelectorAll("button").forEach((button) => {
      button.disabled = false;
    });
    if (state && state.upstream_kind === "relay") $("toggle").disabled = true;
  }
}
const phases = {
  ready: "已有符合规则的 state",
  collecting: "正在采集，请稍等",
  waiting_for_state: "尚未采到合格 state",
  auth_blocked: "上游拒绝登录或权限",
  rate_limited: "上游要求等待",
  passthrough: "普通转发，不注入",
};
function textNode(tag, text, className) {
  const node = document.createElement(tag);
  node.textContent = text;
  if (className) node.className = className;
  return node;
}
async function refresh() {
  state = await api("status");
  $("headline").textContent =
    state.config_error || state.route_error
      ? "有一项配置需要处理。"
      : state.injection_enabled
        ? "服务在运行，注入已开启。"
        : "服务在运行，当前不注入。";
  $("mode").textContent =
    state.upstream_kind === "relay" ? "中转 / API 转发" : "官方 ChatGPT";
  $("route-count").textContent = state.routes;
  $("managed").textContent = state.configured_codex ? "已接管" : "未接管";
  $("toggle").textContent = state.injection_enabled ? "关闭注入" : "开启注入";
  $("toggle").disabled = state.upstream_kind === "relay";
  if (state.injection_reason)
    $("injection-help").textContent = state.injection_reason;
  $("warnings").replaceChildren();
  for (const message of [state.config_error, state.route_error])
    if (message) $("warnings").append(textNode("div", message, "warning"));
  $("sessions").replaceChildren();
  if (!state.sessions?.length)
    $("sessions").append(
      textNode(
        "p",
        "还没收到 Codex 请求。接管配置后重启 Codex，再发一条短消息。",
        "hint",
      ),
    );
  for (const [i, session] of (state.sessions || []).entries()) {
    let description =
      "会话 " + (i + 1) + " · " + (phases[session.phase] || session.phase);
    if (session.retry_after_seconds)
      description += " · 还需等待约 " + session.retry_after_seconds + " 秒";
    $("sessions").append(textNode("div", description, "session"));
  }
}
async function enter() {
  await refresh();
  sessionStorage.setItem("sleep-state-control", token);
  $("token").value = "";
  $("login").hidden = true;
  $("workspace").hidden = false;
  $("logout").hidden = false;
}
$("login-form").addEventListener("submit", (event) => {
  event.preventDefault();
  action(async () => {
    token = $("token").value.trim();
    await enter();
  });
});
$("logout").addEventListener("click", () => {
  sessionStorage.removeItem("sleep-state-control");
  token = "";
  state = null;
  $("login").hidden = false;
  $("workspace").hidden = true;
  $("logout").hidden = true;
});
document.querySelectorAll("[data-page]").forEach((button) =>
  button.addEventListener("click", () => {
    document
      .querySelectorAll("[data-page]")
      .forEach((item) => item.classList.toggle("active", item === button));
    document.querySelectorAll("[data-view]").forEach((view) => {
      view.hidden = view.dataset.view !== button.dataset.page;
    });
  }),
);
$("toggle").addEventListener("click", () =>
  action(async () => {
    const result = await api("injection", {
      enabled: !state.injection_enabled,
    });
    notice(result.message);
    await refresh();
  }),
);
$("recover").addEventListener("click", () =>
  action(async () => {
    const result = await api("recover", {});
    notice(result.message);
    await refresh();
  }),
);
function sourceMode() {
  const mode = $("source-mode").value;
  const subscription = mode === "subscription" || mode === "file";
  $("subscription-options").hidden = !subscription;
  $("source-value-wrap").hidden = mode === "direct";
  $("source-label").textContent =
    {
      proxy: "代理地址",
      subscription: "订阅链接",
      file: "本机订阅文件的完整路径",
    }[mode] || "";
  $("source-value").placeholder =
    {
      proxy: "socks5://127.0.0.1:7897",
      subscription: "https://… 或 http://127.0.0.1:端口/…",
      file: "C:\\Users\\你的用户名\\Downloads\\subscription.yaml",
    }[mode] || "";
  $("source-hint").textContent =
    mode === "file"
      ? "在这台电脑上读取你指定的普通文件，支持 YAML、URI 列表和 Base64 订阅。不会修改原文件。"
      : mode === "subscription"
        ? "链接可能含订阅密码，请勿截图分享。HTTP 只接受 127.0.0.1 等本机地址；远程链接必须使用 HTTPS。"
        : "端口以你的代理软件为准。支持 http://、https://、socks5://。这里填地址，不是 PowerShell 命令。";
}
$("source-mode").addEventListener("change", sourceMode);
function sourceBody() {
  const split = (id) =>
    $(id)
      .value.split(/[,，]/)
      .map((value) => value.trim())
      .filter(Boolean);
  return {
    mode: $("source-mode").value,
    value: $("source-value").value.trim(),
    user_agent: $("user-agent").value.trim(),
    exclude_keywords: split("exclude"),
    include_protocols: split("protocols"),
  };
}
function resultAt(id, value) {
  $(id).hidden = false;
  $(id).textContent =
    value.message +
    (value.routes ? "\n共 " + value.routes.length + " 个出口配置。" : "") +
    (value.status
      ? "\nHTTP " + value.status + " · " + value.duration_ms + " ms"
      : "");
}
$("test-source").addEventListener("click", () =>
  action(async () => {
    const value = await api("sources/test", sourceBody());
    resultAt("source-result", value);
  }),
);
$("source-form").addEventListener("submit", (event) => {
  event.preventDefault();
  action(async () => {
    if (
      !confirm(
        "用这份设置替换现有出口来源？旧配置会备份，已经采集的 state 会清空。",
      )
    )
      return;
    const value = await api("sources/apply", sourceBody());
    resultAt("source-result", value);
    $("source-value").value = "";
    await refresh();
  });
});
$("load-routes").addEventListener("click", () =>
  action(async () => {
    const value = await api("routes", {});
    $("routes-list").replaceChildren();
    for (const route of value.routes) {
      const row = textNode("div", "", "route");
      const label = textNode("div", "");
      label.append(
        textNode(
          "strong",
          (route.label || route.id) +
            (value.pinned_route === route.id ? " · 当前固定出口" : ""),
        ),
      );
      label.append(textNode("div", route.id, "hint"));
      row.append(label);
      const actions = textNode("div", "", "actions");
      const test = textNode("button", "测试连接", "secondary");
      test.addEventListener("click", () =>
        action(async () =>
          resultAt("route-result", await api("routes/test", { id: route.id })),
        ),
      );
      const pin = textNode("button", "固定此出口", "secondary");
      pin.addEventListener("click", () =>
        action(async () => {
          resultAt("route-result", await api("routes/pin", { id: route.id }));
          await refresh();
        }),
      );
      actions.append(test, pin);
      row.append(actions);
      $("routes-list").append(row);
    }
  }),
);
$("auto-route").addEventListener("click", () =>
  action(async () => {
    resultAt("route-result", await api("routes/pin", { id: "" }));
    await refresh();
  }),
);
if (token) action(enter);
setInterval(() => {
  if (token && !busy && !document.hidden)
    refresh().catch((error) => notice(error.message));
}, 10000);
