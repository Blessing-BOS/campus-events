# 完整接口说明

默认前缀 `/api/v1`，JSON 请求须带 `Content-Type: application/json`。登录后使用 `Authorization: Bearer <access_token>`。请求体上限 1 MiB，未知 JSON 字段会被拒绝。服务返回的时间使用 UTC，页面以北京时间展示。

## 接口清单

| 方法 | 路径 | 权限 | 作用 |
| --- | --- | --- | --- |
| GET | /healthz | 公开 | 进程存活与数据模式 |
| GET | /readyz | 公开 | 完整业务表是否可查询 |
| GET | /api/v1/meta | 公开 | 应用信息与是否支持写入 |
| POST | /auth/register | 公开 | 注册学生 |
| POST | /auth/login | 公开 | 登录，返回令牌和用户 |
| POST | /auth/logout | 登录 | 撤销当前会话，成功返回 204 |
| GET | /me | 登录 | 当前用户 |
| GET | /events | 公开 | 已发布活动列表 |
| GET | /events/:id | 公开 | 已发布活动详情 |
| GET | /organizer/events | 组织者 | 自己的草稿与已发布活动 |
| POST | /events | 组织者 | 创建草稿 |
| PATCH | /events/:id | 所属组织者 | 完整替换草稿的可编辑字段 |
| POST | /events/:id/publish | 所属组织者 | 发布草稿；重复发布返回原活动 |
| POST | /events/:id/registrations | 学生 | 本人报名或重新报名 |
| GET | /events/:id/registrations/me | 学生 | 本人在此活动的报名状态 |
| DELETE | /events/:id/registrations/me | 学生 | 取消本人的报名 |
| GET | /me/registrations | 学生 | 本人报名分页列表 |
| GET | /events/:id/registrations | 所属组织者 | 本活动名单，包含已取消记录 |
| POST | /events/:id/registrations/:registration_id/check-in | 所属组织者 | 核验签到 |

前两条健康检查没有 `/api/v1` 前缀。其余业务路径的表格缩写均接在该前缀后，meta 行已写出完整路径。

## 账号

注册与登录请求：

```json
{"username":"student_01","password":"your-password-at-least-8-bytes"}
```

用户名归一化为小写，只接受 3～32 位 ASCII 字母、数字或下划线；密码 8～72 字节。注册不能传 role，也不能用 public API 创建组织者。返回用户只包含 id、username、role、created_at。

登录响应：

```json
{
  "access_token": "<JWT>",
  "expires_at": "2026-10-29T16:00:00Z",
  "user": {"id":1,"username":"student_01","role":"student","created_at":"2026-10-29T12:00:00Z"}
}
```

JWT 校验签名算法、发行者和有效期；会话必须仍在数据库中，角色从数据库读取。退出只撤销当前会话，其余登录设备的会话不受影响。没有刷新令牌、改密码和密码找回接口。

注册与登录共用一个按客户端 IP 计数的基础限流器：每分钟 30 次，超过返回 429 与 Retry-After。该限制在当前进程内生效；当前没有信任任意代理转发头。

## 活动创建和编辑

以下日期为结构示例，实际调用应使用未来的有效时间：

```json
{
  "title":"Go 项目交流会",
  "description":"带上你的项目，分享一次实现中的取舍。",
  "location":"图书馆研讨室",
  "capacity":30,
  "registration_starts_at":"2026-11-01T00:00:00Z",
  "registration_ends_at":"2026-11-02T00:00:00Z",
  "starts_at":"2026-11-02T02:00:00Z",
  "ends_at":"2026-11-02T04:00:00Z"
}
```

标题 2～120 字，介绍最多 5000 字，地点最多 200 字且不能为空，容量 1～10000。时间归一为 UTC 毫秒精度，再验证：报名开始 < 报名截止 <= 活动开始 < 活动结束，报名截止必须晚于当前时间。

创建时不接受 organizer_id，组织者身份来自登录会话。PATCH 接收完整表单，不是只传修改字段。发布后内容、时间和名额锁定。活动取消、删除和容量调整不在当前范围。

公开详情只显示已发布活动，草稿与不存在的 ID 都返回 404。阶段 phase 为 not_open、open、closed、ongoing 或 ended，按时间推导。remaining_capacity 为数据库容量减去当前有效报名数。

## 分页

活动、本人报名、组织者活动和名单支持 `limit` 与 `before_id`。默认每页 20，最多 100，ID 从大到小排列，只查小于 before_id 的记录。空值、重复参数、负数与越界数值返回 400。

```json
{"items":[],"has_more":false,"next_before_id":null}
```

有下一页时将 next_before_id 原样用于后续请求。新插入的大 ID 需刷新第一页才能看到；此分页不是固定快照。

## 报名、取消与签到

报名和取消不接收 user_id，始终操作当前学生。报名成功返回同一条 registrations 记录，包含 id、event_id、user_id、status、checked_in_at、created_at、updated_at。

报名窗口内重复报名返回原记录，不重复计数；取消只在 active 转 cancelled 时减人数，重复取消返回已取消记录。重新报名复用原记录，重新占用一个名额。报名与取消都受报名时间窗口约束，窗口外返回 REGISTRATION_CLOSED。

本人报名列表与组织者名单在报名记录上附加 username、event_title、location、starts_at、registration_ends_at。名单只允许所属组织者访问。本人状态查询未报名时返回 404。

签到要求活动已发布、当前时间在 `[starts_at, ends_at)`，且报名状态为 active。重复签到保持原签到时间。取消记录不能签到；签到不改变 registered_count。时间戳按数据库 DATETIME(3) 的毫秒精度写入。

## 错误结构

```json
{"error":{"code":"EVENT_FULL","message":"报名名额已满"},"request_id":"<本次请求ID>"}
```

| HTTP | 典型错误码 |
| --- | --- |
| 400 | INVALID_ARGUMENT、INVALID_TIME_RANGE |
| 401 | UNAUTHORIZED、INVALID_CREDENTIALS |
| 403 | FORBIDDEN |
| 404 | NOT_FOUND、EVENT_NOT_FOUND、ROUTE_NOT_FOUND |
| 409 | USERNAME_TAKEN、EVENT_FULL、REGISTRATION_CLOSED、EVENT_ALREADY_PUBLISHED、CHECK_IN_CLOSED、REGISTRATION_CANCELLED、ALREADY_CHECKED_IN |
| 429 | RATE_LIMITED |
| 503 | SERVICE_UNAVAILABLE、NOT_READY、RETRY_LATER、READ_ONLY_MODE |

请求 ID 同时出现在 X-Request-ID 响应头与请求日志中。数据库细节不会直接暴露在响应中。数据库锁等待或死锁时事务回滚，并返回可重试错误，不自动假定操作已成功。
