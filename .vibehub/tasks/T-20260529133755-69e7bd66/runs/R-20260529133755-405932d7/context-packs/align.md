# Context Pack: Align

Task: T-20260529133755-69e7bd66
Run: R-20260529133755-405932d7
Phase: Align
Generated at: 2026-06-01T02:19:06Z
Source commit: d835a9b

## Instructions

Use this context only for the current phase.
Do not mark state.yaml completed.
Report files read, commands run, decisions made, and unresolved risks.

## Capability Output Schema

```json
{
  "required_fields": [
    "intent",
    "scope",
    "success_criteria",
    "non_goals"
  ],
  "optional_fields": [
    "stakeholders",
    "references"
  ],
  "produces": [
    "alignment_summary"
  ],
  "consumes": [],
  "parallel_safe": false,
  "custom": false
}
```

## Prior Outputs Summary

```json
[
  {
    "capability": "align",
    "completed": [
      "`user_confirmed`: 2026-06-01 用户要求把预约抢票/抢座相关“竞态”条件和竞态判断重新全面梳理，说明各场景竞争过程，并修补可能诱发问题的缺陷。",
      "`hard_observed`: 已按 VibeHub 协议读取当前任务入口、上下文、交接、硬规则、协议文件、context pack 和 research pack。",
      "`hard_observed`: 已运行 `vibehub sync /Users/chenm0m/LocalRepo/LaLaLandShow`；同步状态为 `needs_attention`，报告路径为 `.vibehub/tasks/T-20260529133755-69e7bd66/runs/R-20260529133755-405932d7/sync/sync-20260531-160118.md`，changed_files_count=179。",
      "`hard_observed`: 已审计学生首次预约、代约批量预约、重选全部、换本人座位、取消预约、座位快照/SSE、管理员分配/释放/锁座、检票重复扫码、限流和压测工具相关竞态路径。",
      "`hard_observed`: 核心抢座写入仍由 PostgreSQL 事务和约束兜底：`seat_bookings.seat_id` 主键保证一座一人，`seat_bookings.student_id` 唯一保证一人一座，批量写入使用 `ON CONFLICT DO NOTHING` 并按成功插入数量回滚部分成功。",
      "`hard_observed`: 已修补一个高峰期可用性缺陷：`/api/seats/rebook` 和 `/api/seats/change` 现在与 `/api/seats/book` 一样使用预约高峰限流额度，避免重选/换座被普通接口限流过早挡住。",
      "`hard_observed`: 已新增限流回归测试，覆盖 `/api/seats/book`、`/api/seats/rebook`、`/api/seats/change` 都使用 `BookingMax`。",
      "`hard_observed`: 已运行 `gofmt` 和 `go test ./...`，后端测试通过。",
      "`agent_reported`: 本轮未发现会导致“同一座位被两个学生同时成功预约”或“同一学生同时成功占两个座位”的代码级竞态漏洞；剩余上线风险主要是真实 PostgreSQL/Redis 并发压测未运行、管理员锁座与已有预约叠加的产品语义需确认、VibeHub/Git 大量 dirty 文件未归属。",
      "`user_confirmed`: 2026-05-31 用户再次要求确认“代预约/本人登录”逻辑是否闭合，并继续完善巩固。",
      "`hard_observed`: 已加固学生预约释放语义：`POST /api/seats/release` 现在会在同一事务中释放登录学生自己的座位，以及由该登录学生创建的所有代约座位，并为每个释放座位发布 SSE `seat_changed/free`。",
      "`hard_observed`: 已加固重复预约拦截：新预约批次写入前会检查登录学生是否已有本人座位或由其管理的代约座位；存在任一活动记录时返回 `ErrBookingConflict`，避免先代约/后本人或多批次代约叠加。",
      "`hard_observed`: 已加固快照和前端展示：座位快照支持 `proxy` 状态并返回被代约学生 ID；选座页统计、图例、底部已预约列表和取消按钮都识别代约座位；SSE 事件后会延迟刷新个性化 snapshot，避免多标签页或代约状态停留在泛化 `others/free`。",
      "`hard_observed`: 已加固个人中心/票券前置数据：`/api/student/me` 的 `booking` 与 `proxy_bookings` 都包含实际 `student_id`，票券页可正确展示本人和代约对象。",
      "`hard_observed`: 已补充测试：服务层释放结果覆盖多座位返回；PostgreSQL 集成测试新增代理批次预约、重复批次拒绝、整组释放校验；快照测试覆盖 `proxy` 分类。",
      "`hard_observed`: 已更新 `docs/api.md` 和 `docs/db-schema.md`，记录 `proxy` 状态、代约学生必须完成注册、单登录人只能管理一个活动预约组、整组释放和代约票券 QR 权限边界。",
      "`hard_observed`: 已运行 `go test ./...`，后端测试通过。",
      "`hard_observed`: 已运行 `npm run build`，前端类型检查和生产构建通过。",
      "`user_confirmed`: 2026-05-31 用户要求进一步完善和巩固当前后台功能。",
      "`hard_observed`: 已再次运行 `vibehub sync /Users/chenm0m/LocalRepo/LaLaLandShow`；同步状态仍为 `needs_attention`，报告路径为 `.vibehub/tasks/T-20260529133755-69e7bd66/runs/R-20260529133755-405932d7/sync/sync-20260531-151049.md`。",
      "`hard_observed`: 已新增 `Config.Validate()` 并在后端启动时调用；生产环境会拒绝空密钥、JWT/HMAC 共用同一密钥、占位密钥、过短密钥，以及占位/过短的管理员或检票员密码。",
      "`hard_observed`: 已新增配置校验单元测试，覆盖生产占位密钥拒绝、JWT/HMAC 相同拒绝、本地开发默认允许、生产强密钥允许。",
      "`hard_observed`: 已在本地和生产 Compose/env 示例中显式加入 `APP_ENV`；开发默认保留可启动，生产默认 `APP_ENV=production`。",
      "`hard_observed`: 已将本地默认 `HMAC_SECRET` 改为 `change-me-hmac`，避免开发环境因为 JWT/HMAC 默认相同而无法启动。",
      "`hard_observed`: 已统一文档口径：代约时每个被预约学生都必须先完成注册资料；未注册或半注册学生会收到 `409`。",
      "`hard_observed`: 已运行 `gofmt` 和 `go test ./...`，后端测试通过。",
      "`user_confirmed`: 2026-05-31 用户要求检查当前系统后台功能是否全面、是否满足最开始需求，以及特殊情况处理能力是否充足。",
      "`hard_observed`: 已按 VibeHub 协议读取当前任务入口、上下文、交接、硬规则、协议文件、context pack 和 research pack。",
      "`hard_observed`: 已运行 `vibehub sync /Users/chenm0m/LocalRepo/LaLaLandShow`；同步状态为 `needs_attention`，报告路径为 `.vibehub/tasks/T-20260529133755-69e7bd66/runs/R-20260529133755-405932d7/sync/sync-20260531-150411.md`。",
      "`hard_observed`: 已核对 `docs/api.md`、`docs/db-schema.md`、`docs/operations.md`、`docs/load-test-report.md` 与 backend 代码，当前后台覆盖学生登录/注册/通知、座位快照/SSE/预约/取消/换座、票券/QR、检票、管理端座位/学生/审计/配置/导出/备份恢复、公共配置、限流、迁移、座位种子和压测工具。",
      "`hard_observed`: 已确认核心抢座正确性由 PostgreSQL 事务与约束兜底：`seat_bookings.seat_id` 主键、`student_id` 唯一、学生预约批量写入事务、`ON CONFLICT DO NOTHING` 后按插入数量整体回滚冲突。",
      "`hard_observed`: 已确认特殊情况已有覆盖：未开放/已锁定/演出后冻结、座位不存在、锁座、重复 seat/student、未包含本人、未注册资料、重复扫码、旧 QR 票、管理员强制改座通知、配置时间格式校验、限流和 2C2G 保守连接池。",
      "`agent_reported`: 本次审查未发现“最初 B0-B12 后台主链路”级别的大缺口；但发现仍有若干验收风险：文档与实现对“代约同伴是否自动半注册”描述不一致，真实 PostgreSQL 集成/并发测试未在本轮执行，生产默认密钥若未覆盖仍有安全风险，当前 169 个 dirty 文件尚未完成 VibeHub 归属确认。",
      "`user_confirmed`: 2026-05-31 用户要求再次深度广泛检查流程漏洞、后端处理漏洞和上千流量并发承载能力；随后补充生产服务器规格为 2 核 2G、带宽约 300M、常态可用内存不到 1G。",
      "`hard_observed`: 已再次运行 `vibehub sync /Users/chenm0m/LocalRepo/LaLaLandShow`；同步状态仍为 `needs_attention`，报告路径为 `.vibehub/tasks/T-20260529133755-69e7bd66/runs/R-20260529133755-405932d7/sync/sync-20260531-145233.md`。",
      "`hard_observed`: 已审查后端启动配置、数据库连接池、认证中间件、限流中间件、座位 SSE、订座事务、后台配置、loadtest 脚本、Nginx 代理和前端选座 store。",
      "`hard_observed`: 已将 PostgreSQL 连接池改为可配置并按 2C2G 小机器保守默认：`DB_MAX_CONNS=12`、`DB_MIN_CONNS=1`、`DB_MAX_CONN_LIFETIME=1h`、`DB_MAX_CONN_IDLE_TIME=5m`。",
      "`hard_observed`: 已在 Compose 和 env example 文件暴露数据库池参数，避免生产默认连接数过大占用内存。",
      "`hard_observed`: 已修复选座 SSE 鉴权不匹配：学生认证中间件现在仅对 `GET /api/seats/stream` 接受 `?token=`，其他接口仍必须使用 Authorization header。",
      "`hard_observed`: 已在前端 seat store 增加 `disconnectStream()`，离开选座页时关闭 EventSource 和重连定时器，避免页面切换后持续占用 SSE 连接。",
      "`hard_observed`: 已将 SSE heartbeat 从 15s 调整为 5s，并在 `deploy/nginx.conf` 为 `/api/seats/stream` 单独关闭 proxy buffering/cache、设置长 `proxy_read_timeout`。",
      "`hard_observed`: 已将限流 key 从 `IP + method + max` 改为 `IP + method + path + max`，避免普通页面接口互相消耗同一个 bucket；保留订座接口单独高上限。",
      "`hard_observed`: 已补后台配置时间值校验：`booking_open_at`、`booking_lock_at`、`show_start_at`、`countdown_target_at` 必须是 JSON string 且符合 RFC3339。",
      "`hard_observed`: 已更新 loadtest booking 模式，使压测学生先登录、完成注册资料，再执行订座，匹配“未注册学生不能订座”的真实生产规则。",
      "`hard_observed`: 已按 2C2G 服务器规格更新运维/压测文档，建议从 `600/40`、`900/60`、`1200/80` 梯度压测，不再直接建议 120 并发冲击小内存机器。",
      "`hard_observed`: 审查结果显示核心抢座正确性仍依赖 PostgreSQL 事务与唯一约束：每座一个 booking、每学生一个 booking；冲突会返回 409 并整体回滚，未发现需要改写的致命并发正确性漏洞。",
      "`user_confirmed`: 用户要求全面深度优化选座页，目标对标大麦、淘票票等购票软件的移动端、桌面端、平板端选座体验。",
      "`user_confirmed`: 用户进一步要求打开本地预约窗口，并继续细化为购票 App 式全览图：顶部显示状态/图例/分区数据，点击座位或空白区域自动居中放大，放大时显示半透明鸟瞰图，排号辅助更清晰。",
      "`hard_observed`: 已按项目协议读取 `.vibehub/agent-view/current.md`、`.vibehub/agent-view/current-context.md`、`.vibehub/rules/hard-rules.md`、当前 context pack 等 VibeHub 入口文件。",
      "`hard_observed`: 已运行 `vibehub sync /Users/chenm0m/LocalRepo/LaLaLandShow`；同步状态为 `needs_attention`，报告路径为 `.vibehub/tasks/T-20260529133755-69e7bd66/runs/R-20260529133755-405932d7/sync/sync-20260531-142116.md`。",
      "`agent_reported`: 调研和归纳票务 App 选座页常见模式：全屏座位地图、舞台/银幕方向、左中右分区、缩放/拖动、颜色图例、底部固定确认栏、已选座即时摘要、不可选座位灰化、点击聚焦和缩略鸟瞰图。",
      "`hard_observed`: 已通过本地 admin API 将 `booking_open_at` 设为 `2026-05-31T00:00:00Z`、`booking_lock_at` 设为 `2026-06-02T00:00:00Z`、`show_start_at` 设为 `2026-06-03T00:00:00Z`；`/api/public/window-state` 返回 `open`。",
      "`hard_observed`: 已继续重构 `frontend/src/views/SeatsView.vue`，保留现有 Vue/Pinia/API 数据流，未改后端接口。",
      "`hard_observed`: 选座页现在是全屏剧场地图，顶部为返回、标题、预约状态、可选/已选/我的/不可选统计；桌面端保留分区席数条，移动端隐藏分区条以减少拥挤。",
      "`hard_observed`: 底部悬浮图例已移至顶部，右侧只保留缩放/回正按钮，降低初始全览时遮挡座位的概率。",
      "`hard_observed`: 座位视觉为带椅背和坐垫层次的小座椅形态，区分可选、已选、我的、占用、锁定状态。",
      "`hard_observed`: 点击可选座位后会以该座位为中心自动放大；点击座位图空白处也会以点击点为中心放大。",
      "`hard_observed`: 放大后左上角显示半透明鸟瞰图，并用视窗框标识当前视野范围。",
      "`hard_observed`: 排号标签改为半透明条样式，在放大时更容易定位当前排。",
      "`hard_observed`: 底部抽屉显示当前座位、待选/换座清单、代约学号输入、统计和确认按钮；有已预约座位时保留取消和换座流程。",
      "`hard_observed`: `npm run build` 通过。",
      "`hard_observed`: 本地 Vite dev server 在 `http://127.0.0.1:5174/` 运行，供用户测试。",
      "`hard_observed`: 浏览器实测 `/seats` 开放状态可见，初始全览下底部抽屉未遮挡座位图主体；点击 `左區 3排 1號` 后页面自动放大并居中，左上角鸟瞰图出现，底部清单显示“待確認換座 / 左區 3排 1號”。",
      "`user_confirmed`: 用户反馈座椅按不动/难以选择，重复点击会持续放大并偏移；拖动时排号文字会进入类似复制/选中状态。",
      "`hard_observed`: 已修正座位点击事件边界：座位按钮使用 click stop，避免座位选择冒泡触发地图空白点击放大。",
      "`hard_observed`: 已在地图拖拽和聚焦后加入位移边界钳制，降低放大到上限后继续点击导致画面漂移的概率。",
      "`hard_observed`: 已对选座页和座位图区设置 `user-select: none`、`-webkit-user-select: none`、`-webkit-touch-callout: none`，并让排号标签不接收指针事件，避免拖动时文字被选中。",
      "`hard_observed`: 已移除底部无选座状态的重复提示文案。",
      "`hard_observed`: 浏览器复测：刷新后开放状态仍可见，`row-label` 的 computed `user-select` 为 `none`；点击 `左區 3排 2號` 后 `.seat.selected` 从 0 变为 1，底部清单显示该座位；再次点击已选座位后 `.seat.selected` 变回 0，且未继续触发地图空白放大。",
      "`user_confirmed`: 用户要求已预约界面清楚区分本人座位与代预约座位，并显示代预约分别对应哪个学号。",
      "`hard_observed`: 已扩展后端座位快照：对当前登录者本人的座位返回 `state: \"mine\"` 和 `student_id`；对当前登录者代预约给别人的座位返回 `state: \"proxy\"`、`student_id`、`booked_by_student_id`；其他人的预约仍只返回 `others`，不泄露学号。",
      "`hard_observed`: 已更新前端 Seat 类型，支持 `proxy` 状态和相关学号字段。",
      "`hard_observed`: 已更新选座页：顶部增加“代约”统计，地图上代约座位显示为蓝色“代”，底部已预约清单将本人/代约分行展示，并显示代约学生学号，例如 `2333233 代約`。",
      "`hard_observed`: 已重建 Docker 后端，使新的座位快照字段在本地 `8080` 生效。",
      "`hard_observed`: 浏览器验证 `/seats` 可见“代約 1”，地图上出现蓝色“代”标记，底部显示 `中區 6排 6號 / 2333233 代約`。",
      "`user_confirmed`: 用户补充要求代约人无论是否锁定都不应看到被代约学生的唯一标识码/二维码，只能看到被代约座位信息。",
      "`hard_observed`: 已核对后端票券服务：`/api/ticket/mine` 仅在票券持有人等于当前登录学生时生成 `qr_payload` 和 `qr_token`；代约记录仍返回 holder/seat 信息但不签发 QR。",
      "`hard_observed`: 已新增 `TestServiceListDoesNotExposeProxyTicketQR`，覆盖本人票券有 QR、代约票券无 `QRToken`/`QRPayload`、但仍保留代约学生与座位信息。",
      "`hard_observed`: 已核对前端票券页：只有 `ticket.owned_by_viewer && ticket.qr_token` 时渲染二维码；代约票券显示“QR 僅本人可見”和座位管理入口。",
      "`hard_observed`: 浏览器验证当前 `/ticket` 页面代约座位卡片显示 `2333233 / 中區 6排 6號`，页面中 `.qr-box` 数量为 0，未出现二维码或 token 文本。",
      "`user_confirmed`: 用户反馈换座、本人选座、代约座位管理交互不清晰，要求改成先点选可换座位，再明确点击换座确认。",
      "`hard_observed`: 已新增后端 `POST /api/seats/rebook`，用于“重选全部”时在一个 PostgreSQL 事务内释放当前本人+代约管理组并写入新选择组；若新座位冲突会整体回滚。",
      "`hard_observed`: 已更新选座页底部状态机：有当前预约时显示模式切换，默认“换本人座位”，另有“重选全部”；两种模式分别显示不同提示和确认按钮。",
      "`hard_observed`: “换本人座位”模式仅选择 1 个空座并调用 `/api/seats/change`，代约座位保持不变；“重选全部”模式可重新点选 1-3 个座位并调用 `/api/seats/rebook`，第 2/3 座填写代约学号。",
      "`hard_observed`: “重选全部”模式允许重新纳入当前本人座位或当前代约座位，便于保留本人座位只调整代约座位。",
      "`hard_observed`: 已更新 `frontend/src/api.ts`、`frontend/src/stores/seats.ts`、`frontend/src/views/SeatsView.vue`、`docs/api.md`、`docs/db-schema.md` 以记录和使用新的 rebook 流程。"
    ],
    "full_ref": ".vibehub/tasks/T-20260529133755-69e7bd66/runs/R-20260529133755-405932d7/outputs/output.md",
    "key_decisions": [
      "`agent_reported`: 将数据库而不是前端/SSE/Redis 作为抢座正确性边界；前端只做即时提示和失败后刷新，不能作为“座位仍可抢”的最终判断。",
      "`agent_reported`: 抢座类写接口应共用预约高峰限流额度，范围包含首次预约、重选全部和换座；释放预约仍保留普通限流。",
      "`agent_reported`: 本轮只修补已确认的限流可用性缺陷，不改动核心事务算法，因为现有约束和事务已覆盖主要竞态正确性。",
      "`agent_reported`: 将“登录人发起的一次本人+代约预约”视为一个活动预约组；取消按组释放，新增预约按组冲突拦截，这比只看本人座位更符合“最多一次预约 3 座”的用户预期。",
      "`agent_reported`: 保留票券 QR 的本人边界：代约人可以看到代约座位信息，但不能看到被代约学生的 QR；被代约学生本人登录后才能获取自己的 QR。",
      "`agent_reported`: SSE 保持泛化事件以避免服务端按用户定制广播，同时前端在事件后刷新个性化 snapshot 修正 `mine/proxy/others` 分类。",
      "`agent_reported`: 安全校验采用“开发环境允许占位值、生产环境强制拒绝”的策略，避免本地调试变麻烦，同时防止误带默认密钥上线。",
      "`agent_reported`: 代约规则按当前实现和运维文档收口为“被预约学生必须已完成资料”，不在本轮改变预约业务逻辑。",
      "`agent_reported`: 将本次结论定位为后台功能审查和风险清单，不直接改业务代码。",
      "`agent_reported`: 对“是否全部满足最初需求”的判断以 `docs/api.md`、`docs/db-schema.md`、research pack 和当前 backend 代码为硬证据；由于 Git 工作区仍有大量未归属变更，结论不等同于正式发布验收。",
      "`agent_reported`: 针对 2C2G 服务器，优先做“少连接、少常驻内存、可梯度压测”的保守调优，而不是提高并发参数制造表面吞吐。",
      "`agent_reported`: SSE query token 只对白名单路径 `/api/seats/stream` 开放，避免为了 EventSource 便利把 token query 变成全站认证入口。",
      "`agent_reported`: Nginx 对 SSE 使用独立 location，因为普通 `/api/` 代理配置适合短请求，不适合长连接实时流。",
      "`agent_reported`: 后台配置时间值在 service 层校验，避免坏配置进入数据库后让 public/status/booking 多处一起失败。",
      "`agent_reported`: 不引入新前端框架或新状态库，降低风险；本轮只重构选座页单文件。",
      "`agent_reported`: 采用“全屏地图 + 顶部状态/图例 + 底部选择抽屉 + 点击聚焦 + 放大鸟瞰图”的票务 App 模式，而不是继续使用普通页面卡片式布局。",
      "`agent_reported`: 移动端优先确保触摸目标、底部固定操作、顶部统计可见、座位图完整；桌面端保留更宽的地图视野和分区数据。",
      "`agent_reported`: 当前不改变选座业务规则：最多 3 座、代约学号校验、已有座位时确认换座、锁定后不可新增或换座。"
    ]
  }
]
```

## Neighbors

```json
[]
```

## File: .vibehub/tasks/T-20260529133755-69e7bd66/task.yaml

Reason: active task metadata and goal

```text
schema_version: 1
kind: vibehub_task
task_id: T-20260529133755-69e7bd66
title: lalaland-platform
mode: evidence_drive
phase: align
phase_status: completed
created_at: 2026-05-29T13:37:55Z
created_by: vibehub
```

## File: .vibehub/tasks/T-20260529133755-69e7bd66/runs/R-20260529133755-405932d7/run.yaml

Reason: active run metadata

```text
schema_version: 1
kind: vibehub_run
task_id: T-20260529133755-69e7bd66
run_id: R-20260529133755-405932d7
mode: evidence_drive
phase: align
phase_status: completed
created_at: 2026-05-29T13:37:55Z
created_by: vibehub
baseline_commit: null
```

## File: .vibehub/rules/hard-rules.md

Reason: protocol hard rules

```text
# VibeHub Hard Rules

- Agent output is reported state only.
- Only VibeHub code updates canonical state transitions.
- Do not mark state.yaml completed from agent output.
- Distinguish hard_observed, agent_reported, inferred, and user_confirmed evidence.
- P0/P1 observability is best-effort and must not claim full runtime observation.
- Agents should read agent-view files and the current context pack, not the whole .vibehub directory.
- Keep changes scoped to the active task.
```

## Known Missing Context

- None declared.

## Stop Condition

Write output.md and return to VibeHub for validation.
