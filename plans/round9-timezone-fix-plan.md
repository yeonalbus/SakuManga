# Round9 时区统一计划

## 背景与问题

用户反馈：更新扫描、Tag 维护中的「东八区小时（0-23）」设置与计算机实际时区不一致，希望调度时刻与系统时间统一。

### 现状

后端调度器**硬编码东八区**作为基准时区：

| 文件 | 现状 | 后果 |
|------|------|------|
| [`update_scheduler.go`](backend/internal/services/update_scheduler.go:24) | `var updateScanLoc = time.FixedZone("Asia/Shanghai", 8*60*60)` | 扫描「6:00」按东八区解释，若系统非东八区则时刻错位 |
| [`tag_maintain.go`](backend/internal/services/tag_maintain.go:35) | `var tagMaintainLoc = loadTagMaintainLocation()`（Asia/Shanghai，失败回退 FixedZone +8） | 每日刷新/每周写回同样按东八区 |
| [`models.go`](backend/internal/models/models.go:88) | 字段注释「东八区」 | 注释误导 |

前端却用**浏览器本地时区**计算「下次预计执行」：

- [`UpdateScanSettings.vue`](src/components/settings/UpdateScanSettings.vue:144) `nextRunLabel`：`new Date()` + `setHours` → 本地时区
- [`TagMaintainSettings.vue`](src/components/settings/TagMaintainSettings.vue) 文案写「东八区」

**前后端基准时区不一致**：前端显示「下次 6:00」是本地 6:00，后端实际按东八区 6:00 触发。

### 目标

将调度基准时区从「硬编码东八区」改为 **`time.Local`（系统本地时区）**，使：
- 设置的「小时」按系统本地时间解释，与用户计算机时间一致
- 前后端「下次预计执行」显示一致

---

## 改动点

### 1. 后端 [`update_scheduler.go`](backend/internal/services/update_scheduler.go:23)

- `var updateScanLoc = time.FixedZone("Asia/Shanghai", 8*60*60)` → `var updateScanLoc = time.Local`
- 注释同步改为「系统本地时区」
- 日志文案「东八区」→「本地」

### 2. 后端 [`tag_maintain.go`](backend/internal/services/tag_maintain.go:34)

- 移除 `loadTagMaintainLocation()` 函数与 `time.LoadLocation("Asia/Shanghai")` 回退逻辑
- `var tagMaintainLoc = loadTagMaintainLocation()` → `var tagMaintainLoc = time.Local`
- 注释同步改为「系统本地时区」

### 3. 后端 [`tag_scheduler.go`](backend/internal/services/tag_scheduler.go:43)

- 日志文案「东八区」→「本地」（2 处：每日刷新、每周写回）

### 4. 后端 [`models.go`](backend/internal/models/models.go:88)

- `RefreshHour`/`WritebackHour`/`ScanHour` 字段注释「东八区」→「系统本地」

### 5. 前端 [`UpdateScanSettings.vue`](src/components/settings/UpdateScanSettings.vue:53)

- 副标题「东八区小时（0-23），默认 6 点」→「系统本地小时（0-23），默认 6 点」
- `nextRunLabel` 注释同步（逻辑本身已用本地时区，无需改动）

### 6. 前端 [`TagMaintainSettings.vue`](src/components/settings/TagMaintainSettings.vue)

- 3 处「东八区」文案改为「系统本地时间」：
  - 每日 Tag 刷新说明（114）
  - 每日刷新时刻副标题（126）
  - 每周反向写回说明（143）+ 写回时刻副标题（166）

---

## 关键说明

- `time.Local` 是 Go 运行时读取的操作系统本地时区，随系统时区/DST 自动变化，无需 tzdata 回退；`time.Now()` 默认即 `time.Local`，`now.In(loc)` 用法保持不变
- 本应用为本地 Web 客户端，浏览器与后端通常同机，`time.Local` 与前端 `new Date()` 的本地时区一致；即使跨机也各自正确
- 无需数据库迁移：`ScanHour`/`RefreshHour`/`WritebackHour` 仍是 0-23 整数，仅解释语义从「东八区」变为「系统本地」，用户已有配置值含义不变（小时数字不变）

## 验证

1. `cd backend && go test ./...`
2. `npm run type-check`
3. 重启后端观察日志「下次 X 触发」时刻与系统时间吻合
4. git 提交（按模块拆分：后端一个 commit、前端一个 commit）
