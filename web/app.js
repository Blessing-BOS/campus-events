"use strict";
const $ = (s) => document.querySelector(s);
const state = {
  user: null,
  token: sessionStorage.getItem("campus_token"),
  writable: false,
  page: "explore",
  filter: "all",
  events: [],
  next: null,
  hasMore: false,
  authMode: "login",
  editing: null,
  renderVersion: 0,
};
const phases = {
  not_open: "即将开放",
  open: "报名中",
  closed: "报名截止",
  ongoing: "进行中",
  ended: "已结束",
};
const date = (v) =>
  new Intl.DateTimeFormat("zh-CN", {
    month: "long",
    day: "numeric",
    hour: "2-digit",
    minute: "2-digit",
    hour12: false,
    timeZone: "Asia/Shanghai",
  }).format(new Date(v));
function el(tag, attrs = {}, ...children) {
  const n = document.createElement(tag);
  for (const [k, v] of Object.entries(attrs)) {
    if (k === "class") n.className = v;
    else if (k.startsWith("on"))
      n.addEventListener(k.slice(2).toLowerCase(), v);
    else if (v !== false && v != null) n.setAttribute(k, v === true ? "" : v);
  }
  for (const c of children.flat(Infinity)) {
    if (c !== null && c !== undefined)
      n.append(c instanceof Node ? c : document.createTextNode(String(c)));
  }
  return n;
}
function toast(message) {
  $("#toast").textContent = message;
  $("#toast").hidden = false;
  clearTimeout(toast.timer);
  toast.timer = setTimeout(() => ($("#toast").hidden = true), 4000);
}
async function api(path, options = {}) {
  const headers = { ...options.headers };
  if (state.token) headers.Authorization = "Bearer " + state.token;
  if (options.body) headers["Content-Type"] = "application/json";
  const controller = new AbortController();
  const timeout = setTimeout(() => controller.abort(), 12000);
  try {
    const r = await fetch("/api/v1" + path, {
      ...options,
      headers,
      signal: controller.signal,
    });
    const data = r.status === 204 ? null : await r.json();
    if (!r.ok) {
      if (r.status === 401 && state.user) {
        clearAccount();
        renderAccount();
      }
      const err = new Error(data?.error?.message || "操作失败，请稍后再试");
      err.status = r.status;
      throw err;
    }
    return data;
  } catch (e) {
    if (e.name === "AbortError") throw new Error("请求超时，请稍后重试");
    throw e;
  } finally {
    clearTimeout(timeout);
  }
}
function clearAccount() {
  state.user = null;
  state.token = null;
  sessionStorage.removeItem("campus_token");
}
function renderAccount() {
  $("#manage-link").hidden = state.user?.role !== "organizer";
  const area = $("#account-actions");
  area.replaceChildren();
  if (state.user) {
    area.append(
      el("span", { class: "account-name" }, state.user.username),
      el(
        "button",
        {
          class: "button small",
          onClick: async () => {
            try {
              await api("/auth/logout", { method: "POST" });
              clearAccount();
              renderAccount();
              if (location.hash !== "#explore") location.hash = "explore";
              else await render();
              toast("已退出登录");
            } catch (e) {
              toast(e.message);
            }
          },
        },
        "退出",
      ),
    );
  } else {
    area.append(
      el(
        "button",
        { class: "button primary small", onClick: () => openAuth() },
        "登录 / 注册",
      ),
    );
  }
}
function openAuth() {
  if (!state.writable) {
    toast("当前为只读预览，请使用完整服务体验报名功能");
    return;
  }
  state.authMode = "login";
  updateAuth();
  $("#auth-form").reset();
  $("#auth-dialog").showModal();
}
function updateAuth() {
  $("#auth-title").textContent =
    state.authMode === "login" ? "欢迎回来" : "加入校园里的新鲜事";
  $("#auth-submit").textContent =
    state.authMode === "login" ? "登录" : "创建学生账号";
  $("#auth-switch").textContent =
    state.authMode === "login"
      ? "还没有账号？注册学生账号"
      : "已有账号？返回登录";
  $("#auth-error").textContent = "";
  $("#auth-form [name=password]").autocomplete =
    state.authMode === "login" ? "current-password" : "new-password";
}
$("#auth-switch").addEventListener("click", () => {
  state.authMode = state.authMode === "login" ? "register" : "login";
  updateAuth();
});
$("#auth-form").addEventListener("submit", async (e) => {
  e.preventDefault();
  const submit = $("#auth-submit");
  submit.disabled = true;
  $("#auth-error").textContent = "";
  const f = new FormData(e.target);
  const payload = { username: f.get("username"), password: f.get("password") };
  try {
    if (state.authMode === "register")
      await api("/auth/register", {
        method: "POST",
        body: JSON.stringify(payload),
      });
    const login = await api("/auth/login", {
      method: "POST",
      body: JSON.stringify(payload),
    });
    state.user = login.user;
    state.token = login.access_token;
    sessionStorage.setItem("campus_token", state.token);
    $("#auth-dialog").close();
    renderAccount();
    await render();
    toast("登录成功，欢迎 " + state.user.username);
  } catch (err) {
    $("#auth-error").textContent = err.message;
  } finally {
    submit.disabled = false;
  }
});
document
  .querySelectorAll("[data-close]")
  .forEach((b) =>
    b.addEventListener("click", () =>
      document.getElementById(b.dataset.close).close(),
    ),
  );
const button = (text, fn, kind = "") =>
  el(
    "button",
    {
      type: "button",
      class: "button " + kind,
      onClick: async (e) => {
        const target = e.currentTarget;
        target.disabled = true;
        try {
          await fn();
        } catch (err) {
          toast(err.message);
        } finally {
          target.disabled = false;
        }
      },
    },
    text,
  );
function heading(title, subtitle, action) {
  return el(
    "section",
    { class: "page-heading" },
    el("div", { class: "eyebrow" }, "CAMPUS / " + state.page.toUpperCase()),
    el(
      "div",
      { class: "section-heading" },
      el("div", {}, el("h1", {}, title), el("p", {}, subtitle)),
      action || null,
    ),
  );
}
function empty(title, desc) {
  return el("div", { class: "empty" }, el("h3", {}, title), el("p", {}, desc));
}
function card(event, index) {
  const art = el(
    "div",
    { class: "card-art art-" + (index % 3) },
    el(
      "span",
      { class: "art-symbol", "aria-hidden": "true" },
      ["✳", "↗", "◎"][index % 3],
    ),
    el(
      "span",
      { class: "art-label" },
      ["LEARN SOMETHING NEW", "MEET YOUR NEXT IDEA", "MAKE A CONNECTION"][
        index % 3
      ],
    ),
    el(
      "span",
      { class: "art-number", "aria-hidden": "true" },
      String(index + 1).padStart(2, "0"),
    ),
  );
  return el(
    "article",
    { class: "event-card" },
    art,
    el(
      "div",
      { class: "card-body" },
      el(
        "div",
        { class: "card-top" },
        el("span", { class: "badge " + event.phase }, phases[event.phase]),
        el("span", { class: "card-category" }, "CAMPUS ACTIVITY"),
      ),
      el("h3", {}, event.title),
      el("p", { class: "card-meta" }, "◷  " + date(event.starts_at)),
      el("p", { class: "card-meta" }, "⌖  " + event.location),
      el("div", { class: "card-divider" }),
      el(
        "div",
        { class: "card-bottom" },
        el(
          "span",
          { class: "seats" },
          "剩余 ",
          el("strong", {}, event.remaining_capacity),
          " / " + event.capacity + " 席",
        ),
        button("了解活动 ↗", () => showDetail(event.id), "small"),
      ),
    ),
  );
}
async function explore(append = false) {
  const version = state.renderVersion;
  if (!append) {
    state.events = [];
    state.next = null;
  }
  const page = await api(
    "/events?limit=9" +
      (append && state.next ? "&before_id=" + state.next : ""),
  );
  if (version !== state.renderVersion) return;
  state.events.push(...page.items);
  state.next = page.next_before_id;
  state.hasMore = page.has_more;
  drawExplore();
}
function drawExplore() {
  const main = $("#main");
  main.replaceChildren();
  main.append(
    el(
      "section",
      { class: "hero" },
      el("div", { class: "eyebrow" }, "MAKE YOUR CAMPUS COUNT"),
      el("h1", {}, "让每一段课余，", el("br"), el("span", {}, "都有所期待。")),
      el(
        "p",
        {},
        "一场分享、一次相遇、一个新起点，",
        el("br"),
        "在这里发现校园灵感。",
      ),
    ),
  );
  if (!state.writable)
    main.append(
      el(
        "div",
        { class: "notice" },
        "你正在浏览只读预览。完整服务支持登录、活动发布、报名与签到。",
      ),
    );
  main.append(
    el(
      "div",
      { class: "section-heading" },
      el(
        "div",
        {},
        el("span", { class: "eyebrow" }, "FIND YOUR NEXT MOMENT"),
        el("h2", {}, "探索精彩校园"),
      ),
      el("p", {}, "把课堂交给日常，把惊喜留给这里。"),
    ),
  );
  const toolbar = el("div", { class: "toolbar" });
  for (const [value, label] of [
    ["all", "全部活动"],
    ["open", "正在报名"],
    ["ongoing", "进行中"],
    ["not_open", "即将开放"],
  ])
    toolbar.append(
      button(
        label,
        () => {
          state.filter = value;
          drawExplore();
        },
        "chip " + (state.filter === value ? "selected" : ""),
      ),
    );
  main.append(toolbar);
  const items = state.events.filter(
    (e) => state.filter === "all" || e.phase === state.filter,
  );
  main.append(
    items.length
      ? el(
          "div",
          { class: "event-grid" },
          items.map((e, i) => card(e, i)),
        )
      : empty("暂时没有符合条件的活动", "换一个筛选，或稍后再来看看。"),
  );
  if (state.filter !== "all")
    main.append(
      el(
        "p",
        { class: "section-note" },
        "筛选当前已加载的活动；加载更多可继续发现。",
      ),
    );
  if (state.hasMore)
    main.append(
      el(
        "div",
        { class: "pagination" },
        button("加载更多活动", () => explore(true)),
      ),
    );
}
async function showDetail(id) {
  const e = await api("/events/" + id);
  let registration = null;
  if (state.user?.role === "student") {
    try {
      registration = await api("/events/" + id + "/registrations/me");
    } catch (err) {
      if (err.status !== 404) throw err;
    }
  }
  const content = $("#detail-content");
  content.replaceChildren(
    el(
      "div",
      { class: "dialog-heading" },
      el(
        "div",
        {},
        el("span", { class: "badge " + e.phase }, phases[e.phase]),
        el("h2", {}, e.title),
      ),
      button("×", () => $("#detail-dialog").close(), "icon-button"),
    ),
  );
  const facts = [
    ["活动时间", date(e.starts_at) + " — " + date(e.ends_at)],
    ["活动地点", e.location],
    [
      "报名时间",
      date(e.registration_starts_at) + " — " + date(e.registration_ends_at),
    ],
    ["剩余名额", e.remaining_capacity + " / " + e.capacity + " 席"],
  ];
  content.append(
    el(
      "dl",
      { class: "detail-grid" },
      facts.map(([k, v]) => el("div", {}, el("dt", {}, k), el("dd", {}, v))),
    ),
    el(
      "p",
      { class: "detail-description" },
      e.description || "这场活动，等待你的加入。",
    ),
  );
  if (state.user?.role === "student" && e.phase === "open") {
    content.append(
      el(
        "div",
        { class: "detail-actions" },
        registration?.status === "active"
          ? button("取消我的报名", async () => {
              await api("/events/" + e.id + "/registrations/me", {
                method: "DELETE",
              });
              toast("报名已取消，名额已释放");
              $("#detail-dialog").close();
              await render();
            })
          : e.remaining_capacity > 0
            ? button(
                "报名参加",
                async () => {
                  await api("/events/" + e.id + "/registrations", {
                    method: "POST",
                  });
                  toast("报名成功，可在“我的报名”查看");
                  $("#detail-dialog").close();
                  await render();
                },
                "primary",
              )
            : el("span", { class: "hint" }, "报名名额已满"),
      ),
    );
  } else if (!state.user && e.phase === "open") {
    content.append(
      el(
        "div",
        { class: "detail-actions" },
        button(
          "登录后报名",
          () => {
            $("#detail-dialog").close();
            openAuth();
          },
          "primary",
        ),
      ),
    );
  }
  if (!$("#detail-dialog").open) $("#detail-dialog").showModal();
}
async function mine(append = false) {
  const version = state.renderVersion;
  if (!state.user) {
    $("#main").replaceChildren(
      heading("我的报名", "每一次参与，都值得期待。"),
      empty(
        "登录后查看你的校园日程",
        "报名记录、取消操作和签到状态都会出现在这里。",
      ),
      el(
        "div",
        { class: "pagination" },
        button("登录 / 注册", () => openAuth(), "primary"),
      ),
    );
    return;
  }
  if (state.user.role !== "student") {
    $("#main").replaceChildren(
      heading("我的报名", "组织者可以通过活动管理查看报名名单。"),
      empty("当前为组织者身份", "切换至学生账号体验活动报名。"),
    );
    return;
  }
  const prev = append ? mine.cursor : null;
  const data = await api(
    "/me/registrations?limit=20" + (prev ? "&before_id=" + prev : ""),
  );
  if (version !== state.renderVersion) return;
  mine.items = append ? [...mine.items, ...data.items] : data.items;
  mine.cursor = data.next_before_id;
  const records = mine.items.map((r) =>
    el(
      "article",
      { class: "record" },
      el(
        "div",
        {},
        el("h3", {}, r.event_title),
        el("p", {}, date(r.starts_at) + " · " + r.location),
        el(
          "p",
          {},
          r.checked_in_at
            ? "已于 " + date(r.checked_in_at) + " 签到"
            : r.status === "cancelled"
              ? "已取消报名"
              : "报名成功，期待你的到来",
        ),
      ),
      el(
        "div",
        { class: "record-actions" },
        el(
          "span",
          { class: "badge " + (r.status === "active" ? "open" : "") },
          r.checked_in_at
            ? "已签到"
            : r.status === "active"
              ? "已报名"
              : "已取消",
        ),
        button("活动详情", () => showDetail(r.event_id), "small"),
        r.status === "active" &&
          !r.checked_in_at &&
          new Date() < new Date(r.registration_ends_at)
          ? button(
              "取消报名",
              async () => {
                await api("/events/" + r.event_id + "/registrations/me", {
                  method: "DELETE",
                });
                toast("已取消报名");
                await mine();
              },
              "small danger",
            )
          : null,
      ),
    ),
  );
  $("#main").replaceChildren(
    heading("我的报名", "为有意义的相遇，留出一点时间。"),
    ...(records.length
      ? records
      : [empty("还没有报名记录", "去发现活动，给自己的日程加一点新鲜感。")]),
  );
  if (data.has_more)
    $("#main").append(
      el(
        "div",
        { class: "pagination" },
        button("加载更多记录", () => mine(true)),
      ),
    );
}
function localInputDate(d) {
  const pad = (n) => String(n).padStart(2, "0");
  return `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())}T${pad(d.getHours())}:${pad(d.getMinutes())}`;
}
function editEvent(e = null) {
  state.editing = e?.id || null;
  $("#event-form").reset();
  $("#event-error").textContent = "";
  $("#event-form-title").textContent = e ? "编辑活动草稿" : "创建校园活动";
  for (const name of ["title", "description", "location", "capacity"])
    if (e) $("#event-form").elements[name].value = e[name];
  const now = Date.now();
  const defaults = {
    registration_starts_at: now - 60000,
    registration_ends_at: now + 86400000,
    starts_at: now + 172800000,
    ends_at: now + 180000000,
  };
  for (const name of Object.keys(defaults))
    $("#event-form").elements[name].value = localInputDate(
      new Date(e ? e[name] : defaults[name]),
    );
  $("#event-dialog").showModal();
}
$("#event-form").addEventListener("submit", async (e) => {
  e.preventDefault();
  const submit = e.target.querySelector("[type=submit]");
  submit.disabled = true;
  $("#event-error").textContent = "";
  try {
    const f = new FormData(e.target);
    const input = {
      title: f.get("title"),
      description: f.get("description"),
      location: f.get("location"),
      capacity: Number(f.get("capacity")),
    };
    for (const name of [
      "registration_starts_at",
      "registration_ends_at",
      "starts_at",
      "ends_at",
    ])
      input[name] = new Date(f.get(name)).toISOString();
    await api("/events" + (state.editing ? "/" + state.editing : ""), {
      method: state.editing ? "PATCH" : "POST",
      body: JSON.stringify(input),
    });
    $("#event-dialog").close();
    toast("草稿已保存");
    await manage();
  } catch (err) {
    $("#event-error").textContent = err.message;
  } finally {
    submit.disabled = false;
  }
});
async function manage(append = false) {
  const version = state.renderVersion;
  if (state.user?.role !== "organizer") {
    $("#main").replaceChildren(
      heading("活动管理", "让每一次校园相聚，都井然有序。"),
      empty("此页面需要组织者身份", "请使用组织者账号登录。"),
    );
    return;
  }
  const prev = append ? manage.cursor : null;
  const data = await api(
    "/organizer/events?limit=20" + (prev ? "&before_id=" + prev : ""),
  );
  if (version !== state.renderVersion) return;
  manage.items = append ? [...manage.items, ...data.items] : data.items;
  manage.cursor = data.next_before_id;
  const records = manage.items.map((e) =>
    el(
      "article",
      { class: "record" },
      el(
        "div",
        {},
        el("h3", {}, e.title),
        el("p", {}, date(e.starts_at) + " · " + e.location),
        el("p", {}, e.registered_count + " 人报名 / " + e.capacity + " 个名额"),
      ),
      el(
        "div",
        { class: "record-actions" },
        el(
          "span",
          { class: "badge " + (e.status === "draft" ? "draft" : e.phase) },
          e.status === "draft" ? "草稿" : phases[e.phase],
        ),
        e.status === "draft"
          ? button("编辑", () => editEvent(e), "small")
          : null,
        e.status === "draft"
          ? button(
              "发布活动",
              async () => {
                await api("/events/" + e.id + "/publish", { method: "POST" });
                toast("活动已发布");
                await manage();
              },
              "small primary",
            )
          : button("报名名单", () => roster(e), "small"),
      ),
    ),
  );
  $("#main").replaceChildren(
    heading(
      "活动管理",
      "把一个好想法，变成一场好活动。",
      button("＋ 创建活动", () => editEvent(), "primary"),
    ),
    ...(records.length
      ? records
      : [
          empty(
            "从第一场活动开始",
            "创建草稿、确认时间与名额，然后发布给同学们。",
          ),
        ]),
  );
  if (data.has_more)
    $("#main").append(
      el(
        "div",
        { class: "pagination" },
        button("加载更多活动", () => manage(true)),
      ),
    );
}
async function roster(e, append = false) {
  const prev = append ? roster.cursor : null;
  const data = await api(
    "/events/" +
      e.id +
      "/registrations?limit=20" +
      (prev ? "&before_id=" + prev : ""),
  );
  roster.items = append ? [...roster.items, ...data.items] : data.items;
  roster.cursor = data.next_before_id;
  const content = $("#detail-content");
  content.replaceChildren(
    el(
      "div",
      { class: "dialog-heading" },
      el(
        "div",
        {},
        el("span", { class: "eyebrow" }, "ATTENDANCE"),
        el("h2", {}, e.title),
        el("p", { class: "hint" }, "报名名单 · " + date(e.starts_at)),
      ),
      button("×", () => $("#detail-dialog").close(), "icon-button"),
    ),
  );
  if (!roster.items.length)
    content.append(
      empty("暂时没有报名", "发布后，学生的报名记录会显示在这里。"),
    );
  else {
    const body = el("tbody");
    for (const r of roster.items)
      body.append(
        el(
          "tr",
          {},
          el("td", {}, r.username),
          el(
            "td",
            {},
            r.status === "cancelled"
              ? "已取消"
              : r.checked_in_at
                ? "已签到"
                : "待签到",
          ),
          el(
            "td",
            {},
            r.status === "active" && !r.checked_in_at
              ? button(
                  "核验签到",
                  async () => {
                    await api(
                      "/events/" +
                        e.id +
                        "/registrations/" +
                        r.id +
                        "/check-in",
                      { method: "POST" },
                    );
                    toast("签到成功");
                    await roster(e);
                  },
                  "small",
                )
              : r.checked_in_at
                ? date(r.checked_in_at)
                : "—",
          ),
        ),
      );
    content.append(
      el(
        "table",
        { class: "roster" },
        el(
          "thead",
          {},
          el(
            "tr",
            {},
            el("th", {}, "学生"),
            el("th", {}, "状态"),
            el("th", {}, "签到"),
          ),
        ),
        body,
      ),
    );
  }
  if (data.has_more)
    content.append(
      el(
        "div",
        { class: "pagination" },
        button("加载更多名单", () => roster(e, true)),
      ),
    );
  content.append(
    el(
      "p",
      { class: "hint" },
      "仅可在活动开始至结束期间签到。取消报名的同学无法签到。",
    ),
  );
  if (!$("#detail-dialog").open) $("#detail-dialog").showModal();
}
async function render() {
  const version = ++state.renderVersion;
  state.page = location.hash.slice(1) || "explore";
  document
    .querySelectorAll("[data-nav]")
    .forEach((a) => a.classList.toggle("active", a.dataset.nav === state.page));
  $("#main").replaceChildren(
    el("div", { class: "loading" }, "正在整理校园里的新鲜事…"),
  );
  try {
    if (state.page === "mine") await mine();
    else if (state.page === "manage") await manage();
    else await explore();
  } catch (err) {
    if (version !== state.renderVersion) return;
    $("#main").replaceChildren(
      empty("暂时无法加载", err.message),
      el("div", { class: "pagination" }, button("重新加载", render)),
    );
  }
}
window.addEventListener("hashchange", render);
(async () => {
  try {
    const meta = await api("/meta");
    state.writable = meta.writable;
    if (state.token) {
      try {
        state.user = await api("/me");
      } catch {
        clearAccount();
      }
    }
    renderAccount();
    await render();
  } catch (err) {
    $("#main").replaceChildren(empty("连接暂时不可用", err.message));
  }
})();
