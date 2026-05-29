# 02 · LaLaLand 毕业话剧平台 — 落地实施方案

> Status: **v2 — 与 [01-requirements.md](file:///Users/chenm0m/LocalRepo/LaLaLandShow/.vibehub/notes/01-requirements.md) v2 配套，GAP 全部并入**
> 这是**实施/落地文档**，聚焦"怎么做"。需求请看 01。
> 前后端拆开成独立 track，可并行或先后端后前端。

---

## A. 技术栈定型

| 层 | 选型 | 理由 |
|----|------|------|
| 后端语言 | **Go 1.22+** | 高并发友好、单二进制部署、2C2G 友好、AI 易写 |
| Web 框架 | **Fiber v3**（或 Echo v4 备选） | 类 Express 简洁，性能强 |
| 数据库 | **PostgreSQL 16** | 唯一约束抢座、JSONB 灵活、备份成熟 |
| 缓存 | **Redis 7** | 座位 bitmap + 限流 + 会话 |
| 鉴权 | JWT（学生短 token）+ bcrypt（管理员/检票员密码） | |
| 实时通信 | **SSE**（Server-Sent Events） | 单向推送选座变化，比 WebSocket 简单稳定 |
| 容器 | Docker Compose | 用户自取 |
| 静态资源 | 阿里云 OSS + CDN（剧照/视频） | 削带宽峰值（虽峰值 200 Mbps 但共用） |
| HTTPS / 反代 | 用户自理（Nginx / Caddy） | |
| 前端框架 | **由 MiMo 选型**（建议 Next.js / Nuxt / SvelteKit / Vite + Vue 任一） | MiMo 输出后人工微调 |
| 前端 UI | TailwindCSS + 自定义主题（紫/深蓝/夕阳黄） | MiMo 友好 |
| 二维码生成 | go-qrcode（后端）+ qrcode.vue / qr-scanner（前端） | |
| 二维码扫描 | **html5-qrcode** 或 zxing-js | PWA 兼容 |
| 日志/监控 | Zap + Promtail/Loki（可选） | |
| 备份 | pg_dump cron + 挂载卷 | |

---

## B. 总体架构

```diagram
                            ╭───────────────────╮
                            │  阿里云 OSS + CDN │  ← 剧照/视频
                            ╰─────────┬─────────╯
                                      │
   ╭─────────╮     ╭──────────╮     ╭─▼──────────────╮
   │ 学生端  │────▶│ Nginx /  │────▶│ Go Fiber       │
   │ 检票端  │     │ Caddy    │     │ App (单进程)   │
   │ 管理端  │     │ (HTTPS)  │     │   ├ REST API   │
   ╰─────────╯     ╰──────────╯     │   ├ SSE 推送   │
                                    │   └ 静态前端   │
                                    ╰────┬───────┬───╯
                                         │       │
                                  ╭──────▼─╮  ╭──▼────╮
                                  │Postgres│  │ Redis │
                                  ╰────────╯  ╰───────╯
                                         │
                                  ╭──────▼──────╮
                                  │ pg_dump cron│
                                  │ → 挂载卷    │
                                  ╰─────────────╯
```

---

## C. 仓库结构（推荐）

```
LaLaLandShow/
├── backend/                  # Go 后端
│   ├── cmd/server/main.go
│   ├── internal/
│   │   ├── api/              # HTTP handlers
│   │   ├── service/          # 业务逻辑
│   │   ├── repo/             # DB 访问
│   │   ├── domain/           # 实体模型
│   │   ├── auth/             # JWT / 密码 / 中间件
│   │   ├── seat/             # 座位逻辑（并发核心）
│   │   ├── ticket/           # 票根 / 二维码
│   │   ├── admin/            # 后台
│   │   └── checkin/          # 检票
│   ├── migrations/           # 数据库 schema 演进
│   ├── go.mod
│   └── Dockerfile
├── frontend/                 # MiMo 生成 + 微调
│   ├── (Next.js / SvelteKit 等)
│   ├── Dockerfile
│   └── ...
├── deploy/
│   ├── docker-compose.yml
│   ├── nginx.conf.example    # 给用户参考的反代配置
│   ├── backup.sh             # pg_dump 脚本
│   └── .env.example
└── docs/
    ├── api.md                # OpenAPI / 接口文档
    ├── db-schema.md
    └── operations.md
```

---

## D. 数据库 Schema 草案

> 在 research/plan 阶段会进一步打磨。这里给出 MVP 版本。

```sql
-- 学生（含被代约的未注册）
CREATE TABLE students (
  student_id     VARCHAR(7) PRIMARY KEY,                  -- 7 位学号
  class_code     VARCHAR(8),                              -- 班级（903 / 1103），半注册时为 NULL
  seat_no_in_class INT,                                   -- 班级座号，半注册时为 NULL
  display_name   VARCHAR(64),                             -- 可选真名（未来扩展）
  is_registered  BOOLEAN GENERATED ALWAYS AS (class_code IS NOT NULL) STORED,
  pending_notice JSONB,                                   -- 管理员强占后给用户的下次登录通知，本人确认后清空
  created_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at     TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- 座位静态定义（启动时种子初始化 385 座）
CREATE TABLE seats (
  seat_id        SERIAL PRIMARY KEY,
  zone           VARCHAR(8) NOT NULL,   -- left / middle / right
  row_no         INT NOT NULL,
  col_no         INT NOT NULL,
  is_blocked     BOOLEAN NOT NULL DEFAULT false,   -- 永久不可选
  block_reason   VARCHAR(64),
  UNIQUE(zone, row_no, col_no)
);

-- 座位绑定（核心抢座表）
CREATE TABLE seat_bookings (
  seat_id        INT PRIMARY KEY REFERENCES seats(seat_id),
  student_id     VARCHAR(7) UNIQUE REFERENCES students(student_id), -- ★ 每学号至多一座
  custom_label   VARCHAR(64),                                       -- 管理员自由占用标识（与 student_id 互斥，CHECK 约束）
  booked_by_student_id VARCHAR(7) REFERENCES students(student_id),  -- 代约人
  booked_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
  ticket_version INT NOT NULL DEFAULT 1,                            -- 换座则递增，旧二维码失效
  CHECK ((student_id IS NULL) OR (custom_label IS NULL))            -- 二选一
);

-- 操作日志
CREATE TABLE audit_logs (
  id             BIGSERIAL PRIMARY KEY,
  actor_type     VARCHAR(16) NOT NULL,   -- student / admin / checker / system
  actor_id       VARCHAR(64),
  action         VARCHAR(32) NOT NULL,   -- book / release / change_seat / admin_force / ...
  target_seat_id INT,
  target_student VARCHAR(7),
  before_json    JSONB,
  after_json     JSONB,
  ip_addr        INET,
  created_at     TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- 入场扫描事件
CREATE TABLE checkin_events (
  id             BIGSERIAL PRIMARY KEY,
  student_id     VARCHAR(7),                          -- 可能 NULL（未预约强批且无学号）
  seat_id        INT,                                 -- 可能 NULL（未预约强批）
  ticket_version INT,
  method         VARCHAR(16) NOT NULL,                -- qr / manual / force_no_booking
  result         VARCHAR(16) NOT NULL,                -- approved / rejected / signature_invalid / not_found
  is_first_scan  BOOLEAN NOT NULL DEFAULT false,
  scanner_account VARCHAR(64),
  ip_addr        INET,
  scanned_at     TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_checkin_student ON checkin_events(student_id, scanned_at);

-- 管理员 / 检票员账号（从环境变量启动时种子，不做改密功能）
-- 仍存 DB 以便会话校验 + 日志关联
CREATE TABLE staff_accounts (
  id             SERIAL PRIMARY KEY,
  username       VARCHAR(32) UNIQUE NOT NULL,
  password_hash  TEXT NOT NULL,
  role           VARCHAR(16) NOT NULL,   -- admin / checker
  created_at     TIMESTAMPTZ NOT NULL DEFAULT now()
);
-- 启动时从 ENV 读取：ADMIN_USERS / ADMIN_PASSWORDS / CHECKER_USER / CHECKER_PASSWORD
-- 启动时 upsert 到 staff_accounts，密码用 bcrypt 哈希

-- 系统配置（KV）
CREATE TABLE system_config (
  key            VARCHAR(64) PRIMARY KEY,
  value_json     JSONB NOT NULL,
  updated_by     VARCHAR(64),
  updated_at     TIMESTAMPTZ NOT NULL DEFAULT now()
);
-- 例：booking_open_at / booking_lock_at / show_start_at / countdown_target_at
--    play_name / venue / stage_desc / story_background / story_summary
--    cast_list (JSON) / gallery_urls (JSON array)
```

### 关键约束
- `seat_bookings.student_id UNIQUE` → DB 兜底"每学号最多 1 座"
- `seat_bookings.seat_id PRIMARY KEY` → DB 兜底"每座最多 1 人"
- `INSERT ... ON CONFLICT DO NOTHING` → 抢座原子化

---

## E. 关键 API 草案（后端先实现）

> 所有时间戳 UTC+8。所有写入接口幂等 token 可选。

| Method | Path | 用途 | 角色 |
|--------|------|------|------|
| POST | `/api/student/login` | 学号登录（首次返回需补全标志） | public |
| POST | `/api/student/register` | 补全班级 + 座号 | student |
| GET | `/api/student/me` | 我的座位 / 代约 | student |
| GET | `/api/seats/snapshot` | 整个选座图状态 | student |
| GET | `/api/seats/stream` | SSE 推送 | student |
| POST | `/api/seats/book` | 原子提交 1~3 座（带 companion 学号） | student |
| POST | `/api/seats/release` | 释放单座 | student |
| POST | `/api/seats/change` | 换座（仅锁定前） | student |
| GET | `/api/ticket/me` | 票根数据 + 二维码 | student（锁定后） |
| GET | `/api/ticket/me/image` | 票根 PNG 下载 | student |
| POST | `/api/checker/login` | 检票员/管理员登录 | public |
| POST | `/api/checker/verify-qr` | 扫码：验签 + 查座位（仅返回信息，不计入场） | checker, admin |
| POST | `/api/checker/lookup` | 手输学号查座位 | checker, admin |
| POST | `/api/checker/checkin` | **核准入场**（method = qr/manual/force_no_booking） | checker, admin |
| GET | `/api/checker/stats` | 实时已入场计数 | checker, admin |
| GET | `/api/checker/export-roster.csv` | 下载完整入场名单 CSV（宕机兜底） | checker, admin |
| POST | `/api/admin/login` | 管理员登录 | public |
| GET | `/api/admin/seats` | 全座位含绑定详情（学号/班级/座号/代约人/custom_label） | admin |
| POST | `/api/admin/seats/:id/assign` | 改绑学号 / 强占 / 释放（带 `force_release_existing` flag 触发二次确认逻辑） | admin |
| POST | `/api/admin/seats/:id/lock` | 永久锁定 / 解锁 | admin |
| GET | `/api/admin/students` | 注册名单（分页/搜索/按班筛选） | admin |
| GET | `/api/admin/audit-logs` | 审计日志（分页 + 筛选） | admin |
| GET | `/api/admin/checkin-events` | 入场事件日志 | admin |
| GET | `/api/admin/stats` | Dashboard 数据 | admin |
| GET/PUT | `/api/admin/config/:key` | 系统配置（时间/内容/演员表/剧照） | admin |
| POST | `/api/admin/backup` | 手动备份 | admin |
| POST | `/api/admin/restore` | 恢复（带 `confirm_token` 二次确认） | admin |
| GET | `/api/admin/export/:type.csv` | 各种 CSV 导出（students / bookings / audit / checkin） | admin |
| GET | `/api/public/config` | 介绍页所需公开配置（剧名/倒计时/演员表/剧照/时间窗口状态） | public |
| GET | `/api/public/window-state` | 当前时间窗口状态（before_open / open / locked / show_started） | public |

---

## F. 后端实施阶段（Track B — Backend）

可独立于前端推进。每阶段产出可测试的 API。

### B0 · 项目骨架（0.5 天）
- `go mod init`，Fiber + pgx + redis + zap + jwt 依赖
- Dockerfile + docker-compose（含 PG + Redis）
- 健康检查 `/healthz`
- 环境变量加载（.env）
- **验收**：`docker compose up` 起来，`curl /healthz` 通

### B1 · DB 迁移 + 座位种子（0.5 天）
- 用 `golang-migrate` 落 schema
- 启动时检测并种子 385 个 `seats`（含默认锁定 24 座）
- **验收**：`SELECT count(*) FROM seats` = 385，`is_blocked = true` 的 = 24

### B2 · 学号登录 + 注册补全（1 天）
- `POST /api/student/login` → 返回 JWT + need_register flag
- `POST /api/student/register` → 写 class_code / seat_no_in_class
- JWT 中间件
- 学号格式校验（7 位纯数字）
- **验收**：单元测试覆盖；首次登录、二次登录、半注册补全三场景

### B3 · 选座快照 + SSE 推送（1 天）
- `GET /api/seats/snapshot` 含每座的 free / locked / mine / others 状态
- SSE 频道：座位变更时广播 `{seat_id, new_state}`
- Redis pub/sub 桥接
- **验收**：开两个连接，一边改座位，另一边收到事件

### B4 · 抢座核心（2 天）★
- `POST /api/seats/book`：接收 `[{seat_id, student_id}]` 数组（1~3 条）
- 单事务内 `INSERT ... ON CONFLICT DO NOTHING`，部分失败则回滚全部
- 校验：
  - 学号格式
  - 自己必占 1 座
  - 同一批不重复
  - 每个学号未占座（半注册不算占）
  - 座位未锁定 / 未被占
- 时间窗口校验
- 操作日志
- 触发 SSE 广播
- **验收**：并发抢同座只 1 成功；其他 4xx；学号唯一约束生效

### B5 · 释放 / 换座（0.5 天）
- 时间窗口区分 release 与 change 权限
- 操作日志
- **验收**：锁定后只能 release，不能 change

### B6 · 票根 + 二维码生成（1 天）
- 锁定时间到达 → 后台 cron / 手动触发批量生成 `ticket_version` 不变
- 二维码内容：`base64({student_id, seat_id, version, issued_at}) | hmac_sha256(secret)`
- `GET /api/ticket/me` 返回票根 JSON
- `GET /api/ticket/me/image` 用 Go imaging 库合成 PNG
- **验收**：每个已选座学生能下载 PNG

### B7 · 检票核验（1.5 天）
- 检票员 + 管理员双角色登录（管理员可访问检票端）
- `POST /api/checker/verify-qr`：验签 + 校验 ticket_version + 查座位（仅返回信息）
- `POST /api/checker/lookup`：手输学号 → 查座位
- `POST /api/checker/checkin`：核准入场，写 checkin_events，判 first_scan，记录 method
  - method = `qr` / `manual` / `force_no_booking`（未预约强批）
- `GET /api/checker/stats` 返回 `{checked_in: N, total: M, last_minute_rate: X}`
- `GET /api/checker/export-roster.csv` 下载完整名单（宕机兜底）
- **验收**：
  - 复用扫描标记 first_scan = false
  - 篡改 QR 验签失败 → 拒绝
  - 换座后旧 QR (version 不匹配) → 拒绝
  - 未预约强批写入 method = force_no_booking
  - 管理员账号可调用所有 checker 接口

### B8 · 后台管理 CRUD（2 天）
- 管理员登录（密码从 ENV 启动时种子到 staff_accounts）
- 座位 assign：支持改绑学号 / 释放 / 强占（custom_label）
  - 若目标座位已被占且未传 `force_release_existing=true` → 返回 409
  - 传了 flag → 释放原绑定 + 写入新绑定 + 落日志，并在 students 表设置 `pending_notice` 标记
- 学生登录时检查 `pending_notice` → 返回时附带通知 payload
- 座位永久锁定 / 解锁（is_blocked 字段）
- 系统配置 KV CRUD（时间窗口 / 内容 / 演员表 / 剧照 URL 数组）
- 名单导出 CSV（students / bookings / audit / checkin 4 种）
- 审计日志 / 入场事件查询（分页 + 筛选）
- **验收**：
  - 管理员强占已选座位需 confirm flag
  - 原用户登录可见通知
  - 全日志可查 / 可导出

### B9 · Dashboard 统计 API（1 天）
- 按班分组 SQL
- 占用率计算
- 核验进度按时间桶聚合
- 加 Redis 缓存（30s TTL）
- **验收**：响应 <100ms（含 1200 学生数据）

### B10 · 备份 / 恢复（0.5 天）
- `backup.sh`：pg_dump → 挂载卷 + 滚动保留 14 份
- cron 容器（go-co-op/gocron 在主进程内更简单）
- 手动备份 / 恢复 API
- **验收**：人工触发备份，文件出现；恢复路径需 confirm_token

### B11 · 限流 + 排队 UI 后端支持（0.5 天）
- 单 IP 5 req/s（基于 Redis）
- 「开放预约前」接口集中返回 503 + 倒计时
- **验收**：超频返回 429

### B12 · 压测 + 优化（1 天）
- vegeta / k6 模拟 1200 并发抢 385
- 调 PG `max_connections` / `shared_buffers`
- 加索引
- **验收**：压测报告归档

**Backend 总计：~12 个工作日**（含整理 / 复测，可压缩到 8 天）

---

## G. 前端实施阶段（Track F — Frontend）

可与后端并行。**MiMo 输出后人工微调**。

### F0 · 前端框架定型（0.5 天）
- 让 MiMo 用 Next.js / SvelteKit / Vite+Vue 之一
- TailwindCSS + 主题色变量
- 设计 token：紫 `#6D28D9` / 深蓝 `#0F172A` / 夕阳黄 `#F59E0B`
- 路由：`/`（介绍）/ `/book`（登录/选座）/ `/me`（我的票）/ `/check`（检票）/ `/admin/*`

### F1 · 介绍页（2~3 天，MiMo 主导）
- Hero + CTA
- 倒计时组件（从 `/api/public/config` 拉）
- 故事背景 / 概要 sections
- 演员表卡片
- 剧照画廊（OSS URL）
- 全响应式
- **验收**：Lighthouse 移动端 ≥ 90 分

### F2 · 学号登录 + 注册补全（1 天）
- 7 位学号 input + 校验
- **登录页底部展示数据隐私简短声明**（G20）
- 首次登录弹出班级 + 座号补全
- 半注册场景：补全表单顶部显示「您已被 *{代约人班级}{座号}* 代约第 X 排第 Y 号」，**无接受/释放按钮**
- 登录成功后若 `pending_notice` 非空 → 弹窗显示管理员调整通知，确认后清空
- JWT 存 localStorage，7 天滚动刷新
- **验收**：4 场景流畅：首次登录、二次登录、半注册补全、管理员调整通知

### F3 · 选座图（3~4 天）★
- **进入选座前展示「预约规则横幅」**（G19）：最多 3 座 / 代约即生效 / 锁定后只能释放 / 操作可追溯 → 「我已阅读」按钮（不强制勾选）
- SVG / Canvas 渲染 385 座
- 三区清晰布局
- 锁定 / 已占 / 自己 / 可选 4 态颜色
- 悬停 / 触摸提示
- 移动端缩放 + 拖动
- 选中 chip（左下 1/3、2/3、3/3）
- 代约学号填写表单
- SSE 订阅实时更新
- 提交失败弹窗高亮冲突
- **验收**：移动端、PC 都流畅

### F4 · 我的座位 / 票根（1 天）
- 座位卡片显示「第 X 排第 Y 号 + 代约人 *{班级 + 座号}*」（若是被代约的）
- 票根页（背景剧照 + 信息 + 二维码 + 「本票本人有效，转让无效」小字）
- 下载 PNG 按钮
- 锁定前显示「修改 / 释放」
- 锁定后显示「释放」（取消不来）
- 演出后只读

### F5 · 检票端（1.5 天）
- 调起摄像头扫码（html5-qrcode）
- **手动输入学号输入框**（与扫码并列）
- 大字号结果卡片，根据 result 区分颜色：
  - 绿色 ✅ approved + 首次入场
  - 黄色 ⚠️ approved + 重复扫描（显示首次时间）
  - 红色 ❌ rejected（验签失败 / 未预约）
- 未预约时卡片下方显示「该学号未预约，是否仍核准入场？」按钮（force_no_booking）
- 顶部实时计数（已入场 / 总票数），30s 自动刷新
- 顶部「下载兜底名单 CSV」按钮（演出当天宕机预案）
- **PWA 配置**（演出当天稳定）
- **管理员账号同样可登录**
- **验收**：手机浏览器扫码顺畅 + 手输 + 强批 全流程通

### F6 · 后台管理（3~4 天）
- 管理员登录
- **首页提供「进入检票界面」入口**（管理员双角色）
- 选座图（复用 F3 组件，加管理员操作）
- 座位操作弹窗：改绑学号 / 释放 / **强占** / 永久锁定
- **强占已被预约座位时**：弹窗显示原用户信息 + 二次确认 + 自动落 pending_notice
- 时间 / 内容配置表单（开放预约、锁定、演出开始、倒计时目标）
- 内容管理：剧名、故事文本、演员表（增删改+头像上传到 OSS）、剧照画廊（上传/排序/删除）
- 名单导出按钮（4 种 CSV）
- 审计日志表格 + 筛选（按操作人/时间/动作）
- 入场事件日志表格 + 筛选
- 备份列表 / 手动备份 / 恢复确认弹窗（带 confirm_token）

### F7 · Dashboard 图表（1~2 天）
- 卡片 + ECharts / Chart.js 柱图、饼图、时间曲线
- 30s 自动刷新

### F8 · 「未开放 / 已锁定 / 已结束」状态页（0.5 天）
- 全站时间窗口闸门
- 倒计时跳转

### F9 · 联调 + 微调（持续）
- 后端定型后人工调 MiMo 输出
- 移动端实机测试
- 浏览器兼容矩阵

**Frontend 总计：~13 个工作日**（含 MiMo 等待 + 人工微调）

---

## H. Track 协作策略

```diagram
Week 1 │ B0 B1 B2 B3 ─────────────────────────── 后端打地基
       │             F0 F1 ──────────────────── 前端介绍页 + MiMo 试跑
Week 2 │ B4 B5 ──────────────────────────────── 抢座核心
       │       F2 F3 ──────────────────────────  登录 + 选座图
Week 3 │ B6 B7 B8 ───────────────────────────── 票根/检票/后台 API
       │             F4 F5 ──────────────────── 票根 + 检票端
Week 4 │ B9 B10 B11 B12 ─────────────────────── Dashboard + 备份 + 压测
       │                F6 F7 F8 F9 ─────────── 后台 + Dashboard + 联调
```

> 后端先行 1 周即可让前端有 mock 可对接；二者总并行约 4 周。
> 单人开发的话，建议**先把 B0~B7 完成（约 6~7 天）再启动前端**。

---

## I. 部署 / 上线 Checklist

### 部署
- [ ] 生产环境 `.env`：
  - `JWT_SECRET` / `HMAC_SECRET`（票根二维码签名）
  - `ADMIN_USERS=admin1,admin2` / `ADMIN_PASSWORDS=复杂密码1,复杂密码2`
  - `CHECKER_USER=checker` / `CHECKER_PASSWORD=复杂密码`
  - `POSTGRES_*` / `REDIS_*`
  - `APP_PORT` / `OSS_*`（介绍页静态资源 base URL）
- [ ] OSS bucket 建好 + CORS 配置（前端域名允许）
- [ ] CDN 加速 OSS（剧照、头像走 CDN）
- [ ] 域名解析到 2C2G 服务器
- [ ] Caddy/Nginx 反代 + HTTPS（你自理）
- [ ] docker-compose up -d
- [ ] 检查 staff_accounts 表已种子（从 ENV 读入）
- [ ] pg_dump cron 测试，备份文件出现在挂载卷

### 演出前 1 周
- [ ] 后台配置：开放预约时间 / 锁定时间 / 演出开始时间 / 倒计时目标
- [ ] 后台录入：剧名 / 故事背景 / 故事概要 / 演员表 / 剧照
- [ ] 后台预先占位：演员 / 工作人员 / 家长 / 老师 / 学校领导座位（用 custom_label）
- [ ] 全链路彩排：注册、抢座、代约、修改、释放、票根下载

### 演出当天
- [ ] 检票员账号登录至少 2 台设备
- [ ] **下载兜底 CSV 名单到 2~3 台手机**（系统宕机预案）
- [ ] 准备纸质名单备份（最保险）
- [ ] 管理员实时监控 Dashboard 入场进度
- [ ] 异常时管理员可登录检票端帮忙

### 演出后
- [ ] 导出全部 CSV 存档
- [ ] 触发一次最终备份
- [ ] （可选）按隐私声明销毁个人数据

---

## J. 风险与对策

| 风险 | 对策 |
|------|------|
| MiMo 生成代码质量参差 | 人工 review + 用 GPT-4 / Claude 微调 |
| 抢座 SQL 写错导致超卖 | 单元测试 + 集成测试 + 数据库唯一约束兜底 |
| 共用带宽被打满 | 静态资源 OSS+CDN + 选座图增量传输 |
| 演出当天意外宕机 | 备份 CSV + 纸质名单兜底 + 监控告警 |
| 学号被恶意代约 | 操作日志可追溯 + 本人可释放 + 管理员强制释放 |
| 二维码被截图转让 | 已确认不防御，仅声明 |

---

## K. 下一步建议（VibeHub 流程）

需求侧 100% 对齐（[01-requirements.md §11](file:///Users/chenm0m/LocalRepo/LaLaLandShow/.vibehub/notes/01-requirements.md) 20 个 GAP 已全部收敛）。建议：

1. VibeHub 落库 task `lalaland-platform`、mode = `evidence_drive`
2. `align` phase 完成 → `research` phase 验证 Go+Fiber+PG+Redis 抢座并发实测
3. `plan` phase 按本文档 §F / §G 切分 epic 工单
4. `implement` phase **优先做 backend B0~B7**（最小可演示链路）
5. 然后并行启动 frontend MiMo 生成 → 人工微调
