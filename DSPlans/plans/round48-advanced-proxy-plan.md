# Round48：高级代理设置 + 归档节点联动（E 站大批量下载优化）

> 状态：**已施工完成（2026-10-28），待珱垣实机验收。**
> 依据（定稿方案）：`DSPlans/YeonPlans-dsedit/关于大批量下载优化方案-dsedit.md`（不入库）
> 调研证据：`DSPlans/Research/`（不入库，含日志定量分析脚本）
> 本文件**不含**敏感内容（仅工程落点与接口契约），正常入库。

---

## 〇、实施记录（2026-10-28）

### 代码落点

| 层 | 文件 | 内容 |
|---|---|---|
| 模型 | `backend/internal/models/network.go`（新） | `NetworkSetting`（单行表）+ `ArchiveBinding`；`ClashEnabled` / `GroupForSite` |
| 模型 | `backend/internal/models/download.go` | `DownloadTask` 新增 `NodeName` |
| 迁移 | `backend/internal/database/db.go` | AutoMigrate 追加两张表 |
| 服务 | `backend/internal/services/clash_client.go`（新） | mihomo 控制面客户端：`Version`/`Group`/`ListGroups`/`GroupDelay`/`SetNode`；独立无代理 Transport；401 / 不可达 / 组不存在 / GLOBAL 保护；延迟测试用 E 站 URL + `expected=200-399` |
| 服务 | `backend/internal/services/network_setting.go`（新） | 设置读写、静态校验、组联网校验、客户端单例、出口 IP 探测（可选） |
| 服务 | `backend/internal/services/archive_binding.go`（新） | 绑定 Upsert / Find / Consume / GC（20h TTL） |
| 服务 | `backend/internal/services/archive_circuit.go`（新） | 画廊级连续网络失败熔断（窗口 3） |
| 服务 | `backend/internal/services/download_archive.go` | 阶梯 0 原地重试（300/900ms）；阶梯 1 换节点（全组延迟 → 过滤假节点 → 前 5 → 切换 + 重置连接池 → 只 GET 报价探测）；阶梯 2/3 判错与熔断；扣费后写绑定；报价需扣费前切回绑定节点；节点名落库 |
| 服务 | `backend/internal/services/download.go` | `RetryTask` / `unlockTaskAfterQuotaCheck` 解除熔断 |
| 接口 | `backend/internal/handlers/network_handler.go` | `GET/POST /network/advanced`、`POST /network/clash/test`、`GET /network/clash/groups` |
| 路由 | `backend/internal/router/router.go` | admin 组注册 4 条（仅管理员） |
| 前端 API | `src/api/network.ts`（新） | 4 个接口 + 类型 |
| 前端类型 | `src/api/download.ts` | `DownloadTask` 加 `nodeName` |
| 前端页面 | `src/components/settings/NetworkSettings.vue` | 拆基础/高级两块；高级面板（总开关、3 组配置、控制面地址 + Secret、测试/读取候选组、保存）；关闭时置灰禁交互；权限提示 |
| 前端页面 | `src/views/DownloadsView.vue` | 任务卡新增节点名 chip |
| 测试 | `backend/internal/services/clash_client_test.go`（新） | 假控制面：版本 / 鉴权 / 组 / 延迟 / 切换 / 自动组拒绝 / GLOBAL 保护 / 延迟排序 / 网络错误分类 |
| 测试 | `backend/internal/services/archive_binding_test.go`（新） | 熔断窗口 / 成功打断 / 同 gid 去重 / Reset；绑定生命周期 / 覆盖 / GC；设置默认值与校验 |

### 实现口径补充（施工时定的细节）

- **阶梯 1 的可行性测试用独立短超时 client（12s）**：不沿用主 client（20s × 3 次重试），否则单节点最长 60s、5 个候选会拖到分钟级。整轮最多 5 个候选。
- **换节点后必须 `resetSharedTransports()`**，日志打 `[CLASH] 已切换节点 X 并重置连接池`；这是"切换生效"的前提。
- **出口 IP 探测是异步的**（12s 超时，失败静默）：只用于绑定的校验/审计，不阻塞归档流程。
- **保存时的组校验分级**：控制面不可达 → 仍然保存并返回 `warning`（用户可能稍后启动 Clash）；组不存在 / 组内无选项 / 选了 GLOBAL → 拒绝保存（明确错误）。
- **Secret 不回传明文**：GET 只返回 `hasSecret`；保存时 `keepSecret=true` 沿用原值。
- **权限**沿用现有网络设置 tab 的 `adminOnly`（菜单已限制），未新增中间件。

### 验证记录

- `go build ./...`：通过
- `go test ./...`：通过（含新增 12 个测试用例）
- `npm run type-check`：通过
- `npm run build`：通过
- 打包：`SakuManga.exe`（31.6 MB，构建标识 `202610282048-gef912f7`）

### 待实机验收（珱垣）

- 522 个出错画廊重试后「请求 archiver.php 失败」的下降幅度
- 切换节点后出口 IP 是否真的变化（可用一次 IP 回显验证）
- GP 消耗观察（换 IP 后重新扣费是否符合预期）

---

## 〇.1 施工分期

| 期 | 内容 | 产出 |
|---|---|---|
| **P0** | 后端基础设施：模型、DB 迁移、Clash 控制面客户端、配置读写、连通性测试/组枚举接口 | 可独立测试的服务层 |
| **P1** | 归档引擎改造：原地重试 → 换节点 → 画廊判错 → 组级熔断；报价记录节点、扣费后写绑定、重试先验报价 | 核心行为 |
| **P2** | 前端：网络设置拆基础/高级板块、高级面板（开关/组配置/连接配置/测试/遮罩）、下载条目显示节点名 | UI |
| **P3** | 测试与打包：Go 单测、`go test ./...`、`type-check`/`build`、打包 exe | 可交付 exe |

---

## 一、接口契约（前后端对齐，先定稿）

全部挂在 `admin` 路由组（`middleware.AdminOnly()`），与现有 `/network/proxy` 一致。

### 1.1 `GET /api/v1/network/advanced` — 读取高级代理配置

```json
{
  "enabled": false,
  "controller": "127.0.0.1:9090",
  "secret": "",
  "groupEGallery": "",
  "groupEGalleryEx": "",
  "groupHath": ""
}
```

### 1.2 `POST /api/v1/network/advanced` — 保存（服务端校验）

请求体同上。校验规则：

| 校验 | 失败返回 |
|---|---|
| `groupEGallery` / `groupEGalleryEx` 必填 | 400 `"请填写 e-hentai.org / exhentai.org 的代理组名称"` |
| `controller` 非空（enabled=true 时） | 400 `"请填写 Clash 控制面地址（如 127.0.0.1:9090）"` |
| 组名能在控制面反查到（enabled=true 且能连通时） | 400 `"代理组不存在：xxx"` |
| 组为 Selector 类型 | **不阻止**，200 但返回 `warning` 字段提示 |
| 组内选项为空 | 400 `"代理组 xxx 内没有可用节点"` |

响应：`200 {"message":"已保存","warning":"可选提示"}`

### 1.3 `POST /api/v1/network/clash/test` — 连通性测试

一次完成三件事：`GET /version` 可达 → 读 e-hentai 组 → 跑一次 delay。

```json
// 成功
{ "ok": true, "version": "v1.19.33", "group": "Ehentai", "now": "Hong Kong 14",
  "options": 63, "bestDelayMs": 272, "message": "连接正常" }
// 失败
{ "ok": false, "message": "401：密钥不正确" }
```

### 1.4 `GET /api/v1/network/clash/groups` — 枚举候选组（下拉用）

```json
{ "groups": [ { "name": "Ehentai", "type": "Selector", "now": "Hong Kong 14",
                "options": 63, "usable": 56 } ] }
```

只返回 `type ∈ {Selector, URLTest, Fallback, LoadBalance}` 的组。

### 1.5 下载任务列表新增字段

`GET /downloads/tasks` 的每条任务新增 `nodeName`（string，omitempty）：该任务归档解锁时使用的节点名。来源：`DownloadTask.NodeName`（持久化列，非 binding 关联查询，避免 N+1）。

---

## 二、数据模型

### 2.1 `NetworkSetting`（单行表，`settings` 风格）

```go
type NetworkSetting struct {
    ID                   uint   `gorm:"primaryKey;default:1" json:"id"`
    AdvancedProxyEnabled bool   `gorm:"default:false" json:"enabled"`
    ClashController      string `gorm:"default:'127.0.0.1:9090'" json:"controller"`
    ClashSecret          string `json:"-"`
    GroupEGallery        string `json:"groupEGallery"`
    GroupEGalleryEx      string `json:"groupEGalleryEx"`
    GroupHath            string `json:"groupHath"`
    UpdatedAt            time.Time `json:"updatedAt"`
}
```
> `ClashSecret` 用 `json:"-"`，单独走 `HasSecret bool` 返回，避免明文回传前端（前端编辑时留空=不修改）。

### 2.2 `ArchiveBinding`（归档 ↔ 节点绑定）

```go
type ArchiveBinding struct {
    ID        uint      `gorm:"primaryKey;autoIncrement" json:"id"`
    GID       string    `gorm:"index" json:"gid"`
    UserID    uint      `gorm:"index" json:"userId"`
    Group     string    `json:"group"`     // 解锁时所在代理组
    Node      string    `gorm:"index" json:"node"` // 节点名（切换主键）
    IP        string    `json:"ip,omitempty"`      // 可选：出口 IP（探测失败为空）
    GPCost    string    `json:"gpCost"`    // 报价文本（Free! / 1,952 GP）
    CreatedAt time.Time `json:"createdAt"` // 扣费时刻
    ExpiresAt time.Time `gorm:"index" json:"expiresAt"`
    Consumed  bool      `gorm:"default:false" json:"consumed"`
}
```
清理：`ExpiresAt < now` 或 `Consumed = true` 且创建超过 1h 的记录，由归档引擎每分钟惰性清理（无需独立定时器，挂在调度 tick 上）。

### 2.3 `DownloadTask` 新增列

```go
NodeName string `json:"nodeName,omitempty"` // 归档解锁所用节点名（展示用）
```

---

## 三、后端落点

| 文件 | 动作 | 内容 |
|---|---|---|
| `backend/internal/models/network.go` | 新增 | `NetworkSetting` / `ArchiveBinding` |
| `backend/internal/models/download.go` | 改 | `DownloadTask` 加 `NodeName` |
| `backend/internal/database/db.go` | 改 | AutoMigrate 追加两张表 |
| `backend/internal/services/clash_client.go` | 新增 | 控制面客户端：`Version/GetGroup/GroupDelay/SetNode/Proxies`；**独立无代理 Transport**；超时 5s |
| `backend/internal/services/network_setting.go` | 新增 | DB 读写 + 四项校验 + `ClashReady()` + 单例 |
| `backend/internal/services/archive_binding.go` | 新增 | `Upsert/Find/Consume/GC` |
| `backend/internal/services/archive_circuit.go` | 新增 | 画廊级连续网络失败计数（进程内）+ 组失效标志 |
| `backend/internal/services/download_archive.go` | 改 | 阶梯 0/1/2/3；报价记节点；扣费后写绑定；重试先验报价 |
| `backend/internal/services/download.go` | 改 | `QueryArchiveInfo` 同样接入独立重试（可选，非必须） |
| `backend/internal/handlers/network_handler.go` | 改 | 新增 4 个 handler |
| `backend/internal/router/router.go` | 改 | admin 组注册 4 条路由 |

### 3.1 归档引擎升降阶（P1 核心）

```
run()
 ├─ resolveArchiveDownloadURL()
 │    ├─ fetchArchiverFormsWithRetry()          ← 阶梯0：2 次，300/900ms，仅网络类
 │    ├─ 若 enabled：记录当前节点名 → task.NodeName（DB 更新）
 │    ├─ 若报价为 GP 且存在未过期绑定：
 │    │     └─ 尝试切回绑定节点 → 重置连接池 → 再验报价
 │    ├─ POST createArchive（扣 GP）
 │    └─ 成功后 UpsertArchiveBinding(node, ip, gpCost, now+24h)
 └─ 若失败且错误为网络类：
      ├─ 阶梯1：switchAndRetry()
      │    ├─ GET /group/{组}/delay（一次，5s）
      │    ├─ 过滤无延迟（假节点）
      │    ├─ 按延迟升序取前 5
      │    ├─ 逐个：PUT 切节点 → resetSharedTransports() → fetchArchiverForms
      │    └─ 找到可用 → 固定该节点 → 继续原流程
      ├─ 阶梯2：全失败 → 画廊判错 + circuit.RecordNetworkFail()
      └─ 阶梯3：circuit.Tripped() → 后续 run() 直接判错（不再尝试）
```

非网络类错误（403/404/结构异常/解压）：直接判错，**不计数、不换节点**。

---

## 四、前端落点

| 文件 | 动作 | 内容 |
|---|---|---|
| `src/components/settings/NetworkSettings.vue` | 重写 | 拆「基础代理设置」+「高级代理设置」两块，沿用现有 `.setting-item` 视觉 |
| `src/api/network.ts` | 新增 | `fetchAdvanced/saveAdvanced/testClash/listGroups` + 类型 |
| `src/views/DownloadsView.vue` | 改 | 任务卡片信息行追加节点名（`（下载质量）- 优先级 - 发起者 - 所用节点名`） |
| `src/types/download.ts` | 改 | 任务类型加 `nodeName?: string` |

### 4.1 高级面板交互口径

- 开关关闭：整块 `filter: grayscale + opacity` 遮罩，`pointer-events: none` + 输入项 `disabled`，暗色/亮色主题都要可读
- 开关打开：正常编辑；**保存按钮**统一提交（组配置 + 连接配置一起存）
- 必填校验前置：前两个组为空时点保存 → toast 提示 + 不提交
- 连通性测试按钮：独立于保存，点击后展示 `mihomo vX · 组：X · 当前：Y · 延迟 Zms` 或失败原因
- 组名输入框：失焦时若已配置 controller，自动调 `groups` 接口给候选下拉（可选增强，P2 内实现）
- 权限：网络设置 tab 已是 `adminOnly`，本区域天然仅管理员；只需加一行提示「该设置影响本实例上的所有用户与所有下载任务」

---

## 五、测试与验收

| 类型 | 落点 |
|---|---|
| 单测（新增） | `clash_client_test.go`（httptest 假控制面：401/404/延迟/切换）、`archive_binding_test.go`、`archive_circuit_test.go` |
| 单测（补充） | `archive_download_test.go`：原地重试命中、换节点成功、前 5 全败判错、熔断触发 |
| 命令 | `cd backend && go test ./...`；`npm run type-check`；`npm run build` |
| 打包 | `build-release.bat` 产出 exe（交付珱垣实机部署） |

方案里的实机验收项（522 个出错画廊、GP 开销观察、IP 变化验证）由珱垣执行。

---

## 六、风险与回滚

| 风险 | 对策 |
|---|---|
| 切换后连接池未重置导致“切换无效” | 切换后强制 `resetSharedTransports()`，并在日志打 `[CLASH] reset transport` |
| 换节点循环耗时过长（5 节点 × 3s） | 每节点总超时 5s；整轮最多 25s；日志逐节点记录 |
| 误判组失效（画廊自身问题） | 只对网络类错误计数；403/404 直接跳过熔断 |
| 绑定表膨胀 | 24h 过期 + Consumed 清理，惰性 GC |
| 高级开关关闭时行为必须与现状完全一致 | 所有新逻辑以 `enabled` 短路，默认 false |

---

## 七、交付物

1. 可运行的 exe（`build-release.bat`）
2. 本 plan 的「实施记录」章节追加（落点表 + 验证记录）
3. 不更新版本号（按珱垣要求）
