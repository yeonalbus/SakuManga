# SakuManga 发布流程（Release Process）

> 每次发版必须按此清单执行。本文件是**AI 侧执行清单**——珱垣侧职责见 [`YeonPlans/协作方案/职责分配.md`](../../YeonPlans/协作方案/职责分配.md)，两边不重复。
>
> 最近一次执行：v2.3.0（2026-10）

---

## 0. 流程总览

```
敲定版本号 → 更新版本号 → 整理改动清单 → 组装版本简报 → 同步前端产物 → 维护 PROJECT_TREE
    → 维护 CHANGELOG → 生成 Release Notes（详实版） → 验证（测试/构建） → 打包冒烟（exe）
    → git 提交 + 打 tag + 推送 → 通知珱垣「可以发版了」（附简报）
    → 珱垣写 Release 正文并发布（GitHub 网页） → Docker 由 tag 自动构建 → 发布后收尾
```

---

## 1. 敲定版本号

按[语义化版本（SemVer）](https://semver.org/lang/zh-CN/)选择：

| 类型 | 示例 | 适用场景 |
| ---- | ---- | -------- |
| MAJOR | `3.0.0` | 破坏性变更 / 重大架构重构 / 里程碑式发布 |
| MINOR | `2.3.0` | 向后兼容的新功能 + 修复 + 优化（最常用） |
| PATCH | `2.2.1` | 仅修 Bug，无新功能 |

**决策依据**：是否引入破坏性变更（数据库无法平滑迁移、配置格式不兼容、API 不向后兼容）。若仅新增功能 + 修复且数据可 AutoMigrate 平滑迁移，选 MINOR。

**版本号由珱垣拍板**，AI 给建议。

> ⚠️ **未发布版本号吸收规则**：已被写进 `package.json` / `CHANGELOG` 但**未打 tag 发版**的版本号（如曾被误标的 `2.2.1`），其内容一律并入下一个正式版本，不单独补发；收尾时删掉或改写那个"待发布"区块。

---

## 2. 更新版本号

版本号唯一来源是 [`package.json`](../../package.json)，修改后**必须同步**以下文件：

| 文件 | 位置 | 说明 |
| ---- | ---- | ---- |
| `package.json` | 顶部 `version` 字段 | **唯一版本来源**，About 页 / 打包脚本自动读取 |
| `package-lock.json` | 顶部 `version` 与 `packages[""].version` 两处 | 发版时一并对齐 |
| `backend/internal/version/version.go` | `AppVersion` 常量 | 后端 `/api/v1/system/version` 返回值 |
| `DSPlans/VerNotes/RELEASE_NOTES_vX.Y.Z.md` | 标题 | 与本次版本号一致 |
| `DSPlans/YeonBriefs/BRIEF_vX.Y.Z.md` | 文件名与「版本信息」 | 与本次版本号一致 |
| `CHANGELOG.md` | 顶部版本区块 | 见第 6 节 |

> ⚠️ 版本号变更后**必须重新构建前端产物**（`npm run build` 并同步 `backend/webui/dist/`，或直接跑一次 `build-release.bat`）——「关于」页版本号是构建时嵌进前端的，不重建会残留旧版本号（Docker 镜像同理）。

无需手动修改：
- 前端「关于」页 [`AboutSettings.vue`](../../src/components/settings/AboutSettings.vue) 自动 `import { version } from '../../../package.json'`
- 打包脚本 [`build-release.bat`](../../build-release.bat) 自动 `node -p "require('./package.json').version"`

---

## 3. 整理改动清单

基于 git 历史核对本轮内容：

```bat
git tag -l                                       :: 查看已有 tag（命名规范：SakuManga-X.Y.Z）
git log <上一tag>..HEAD --oneline --no-decorate  :: 列出本次发布涉及的全部提交
git diff --stat <上一tag>..HEAD                  :: 改动规模
```

按提交 type 归类：`feat` → 新增功能、`fix` → Bug 修复、`refactor/perf` → 优化改进。
**重点核对版本窗口内是否有"未写进简报"的 Round**——简报靠日常增量，缺口从这里补。

---

## 4. 组装版本简报（给珱垣）

在 [`DSPlans/YeonBriefs/`](../YeonBriefs/) 下维护 `BRIEF_vX.Y.Z.md`（结构见附录 B）：

- [ ] 日常各 Round 的增量节是否齐全（对应第 3 节的提交范围）
- [ ] 组装定稿：版本信息 → 主题（**珱垣已确认**）→ 亮点分组 → 升级注意 → 已知问题 → README 待改点 → 机械清单
- [ ] 「已知问题」如实写，不因不利于宣传而省略
- [ ] 「README 待改点」只指位置与要点，**不代写正文**
- [ ] 完成后**主动通知珱垣**：「可以发版了」，并给出简报路径

> 口径：简报只陈述事实，不写宣传话术。主题句与卖点措辞由珱垣决定。

---

## 5. 维护 PROJECT_TREE.md

更新 [`PROJECT_TREE.md`](../../PROJECT_TREE.md)：

- [ ] 文件头「版本：vX.Y.Z · 最近更新：YYYY-MM」
- [ ] 「目录总览」：新增/删除的顶层目录（如 `DSPlans/YeonBriefs/`）
- [ ] 前端 `src/`：新增的 `components / composables / stores / utils / views` 文件
- [ ] 后端 `backend/`：新增的 `handlers / services / models` 文件与测试文件
- [ ] 设置面板细分标题中的版本号
- [ ] 「功能 → 文件快速定位索引」：补充新增功能对应的定位文件
- [ ] 「发布注意」：更新版本号与流程引用

---

## 6. 维护 CHANGELOG.md

[`CHANGELOG.md`](../../CHANGELOG.md) 是**版本日志文档**，作为「项目更新状态」的唯一归档处：README 只写当前版本（归珱垣），历史版本一律记在这里。

- [ ] 顶部追加本次版本区块（倒序）：版本号 · 日期（`YYYY-MM`）+ 本期总结 + 分点亮点（新增功能 / 优化改进 / Bug 修复）
- [ ] 若存在未发布的"待发布区块"（如误标的 2.2.1），按第 1 节规则**并入本版**后删除旧区块
- [ ] 已发布版本的既有文字不改写（勘误除外）
- [ ] 与 Release Notes 的分工：CHANGELOG 是**跨版本一览**（每版几行），`RELEASE_NOTES_vX.Y.Z.md` 是**单版本详述**
- [ ] 覆盖范围自 `v2.0.0`（项目更名 SakuManga）起；更早的 1.x 在文末保留一行指引

---

## 7. 生成 Release Notes（详实技术版）

在 [`DSPlans/VerNotes/`](.) 下新建 `RELEASE_NOTES_vX.Y.Z.md`，按附录 A 模板撰写，面向**开发者与愿意看细节的读者**：

- 引言：自上一版本以来做了什么、遗留问题解决情况、数据迁移情况
- `✨ 新增功能` / `🛠️ 优化改进` / `🐛 Bug 修复`：按技术口径分组，可含实现说明
- `⚠️ 已知问题`：如实列出
- `📖 使用说明`：部署 / 运行 / 默认账号 / `--headless` / 升级说明（**技术性内容**；面向用户的卖点与宣传话术不写在这里）

---

## 8. 验证

发布前必须通过：

```bat
:: 后端：全部单元测试
cd backend && go test ./...

:: 前端：类型检查（npm run build 会连带 type-check）
npm run type-check
```

> 失败则不发布：先修复，再重新验证。已知的偶发失败（顺序性单测）需在 Release Notes「已知问题」标注。

---

## 9. 构建产物同步 + 打包冒烟

```bat
build-release.bat
```

该脚本会：构建前端 → 拷贝到 `backend/webui/dist` → `go build` 生成根目录 `SakuManga.exe`（注入构建标识）。

- [ ] `backend/webui/dist` 已包含**本轮全部前端改动**（提交前用 `git status` 确认产物文件有更新）
- [ ] 根目录生成 `SakuManga.exe`，脚本标题版本号正确
- [ ] 双击运行 → 系统托盘出现图标 → 打开界面正常
- [ ] 「关于」页版本号与本次目标一致
- [ ] （可选）`--headless` 模式可正常启动

---

## 10. git 提交 + 打 tag

```bat
git status && git diff --stat     :: 确认改动范围
git add <本次预期文件>            :: 逐项暂存，避免夹带无关文件
git commit -m "chore(release): vX.Y.Z 发布准备（版本号/产物/CHANGELOG/VerNotes/简报）"
git tag SakuManga-X.Y.Z
```

tag 命名必须与历史一致（`SakuManga-2.2.0`、`SakuManga-2.3.0`）。

> ⚠️ **不夹带珱垣的目录**：`YeonPlans/`（只读区）与根目录零散文件（截图等）不进入发版提交；无关改动留给珱垣自行处理。

---

## 11. 发布（GitHub Release + Docker 镜像）

### ① GitHub Release（由珱垣发布）

- [ ] AI 通知「可以发版了」，附简报路径与本轮机械清单
- [ ] 珱垣撰写 Release 正文（基于简报自行组织）
- [ ] 珱垣在网页创建 Release：标题 `vX.Y.Z`，tag 选 `SakuManga-X.Y.Z`
- [ ] 附件上传根目录 `SakuManga.exe`
- [ ] 点发布

### ② Docker 镜像（v2.1.1 起由 tag 自动构建）

工作流见 [`.github/workflows/docker-publish.yml`](../../.github/workflows/docker-publish.yml)，推 tag 即触发，无需本机 Docker：

- [ ] 确认 `package.json` 版本号与 tag 一致（镜像标签取自 tag 名）
- [ ] 推送 tag 后到 **Actions** 页面确认 `Docker Image (GHCR)` 成功
- [ ] 产出：`ghcr.io/yeonalbus/sakumanga:X.Y.Z` · `X.Y` · `latest`（`linux/amd64` + `linux/arm64`）
- [ ] 需**补发镜像而不打 tag**：Actions → `Docker Image (GHCR)` → Run workflow（版本号取自 `package.json`）
- [ ] 本机装有 Docker 时可用 `build-docker.bat`（本地测试）/ `build-docker.bat push`（多架构推 GHCR）

---

## 12. 发布后收尾

- [ ] 确认 tag 已推送、Release 已发布、镜像工作流成功（三项状态回传给珱垣）
- [ ] 将 `BRIEF_vX.Y.Z.md` 定格（不再追加）；新版本另建新简报文件
- [ ] 宣传 / 贴文 / 预览图 / wiki 条目：**由珱垣决定与执行**，AI 只按需补写 wiki 内容
- [ ] 若有遗留的临时脚本 / 调试产物 / 截图，清理或归入 `_local/`

---

## 附录 A：Release Notes 模板（详实技术版）

```markdown
# SakuManga vX.Y.Z

> 自 vA.B.C 以来，……（本期总结，含遗留问题解决情况与数据迁移说明）

## ✨ 新增功能

- （功能 1，可含实现要点）

## 🛠️ 优化改进

- （优化 1）

## 🐛 Bug 修复

- （修复 1）

## ⚠️ 已知问题

- （遗留限制）

## 📖 使用说明

- 下载 `SakuManga.exe` 后双击即可运行……
- 默认监听端口 `8081`；首次启动自动创建管理员 `admin` / `admin123`……
- NAS / 无界面环境请使用 `SakuManga.exe --headless`……
- 升级安装：直接替换 `SakuManga.exe` 即可，`manga.db` / `data/` / `config.json` 跟随 exe 目录自动保留；
  数据库结构由启动时自动迁移（本次另有一次性字段回填，幂等）……
```

---

## 附录 B：版本简报结构模板

```markdown
# vX.Y.Z 版本简报

> 读者：珱垣。用于撰写 README「当前状态」与 GitHub Release 正文。
> 口径：只陈述事实，不含宣传话术。

## 一、版本信息        ← 版本号 / 上一版本 / 时间窗 / 规模 / 数据库 / 升级方式
## 二、主题（已确认）   ← 珱垣拍定的主题句
## 三、用户可感知亮点   ← 按主题分组的 A/B/C…，每条一句话
## 四、升级注意         ← 是否迁移、新增设置项、镜像标签
## 五、已知问题         ← 如实，供珱垣判断是否对外提及
## 六、README 待改点    ← 只指位置与要点，不代写
## 七、机械清单         ← tag / 产物 / 测试 / 镜像，珱垣不必关心
## Rolling log          ← 各 Round 增量记录（提交范围 + 要点）
```

---

## 附录 C：发布前自查清单

- [ ] `git status` 仅含预期改动，无临时脚本 / 调试产物 / 珱垣的目录
- [ ] 无敏感信息随发布（代理地址、账号凭据、测试账号文档）
- [ ] 版本号一致：`package.json` / `package-lock.json` / `version.go` / Release Notes 标题 / 简报文件名 / `CHANGELOG.md` 顶部区块
- [ ] 未发布的旧版本号区块已按规则并入本版
- [ ] `CHANGELOG.md` 已追加本次版本区块（倒序），已发布版本文字未被改写
- [ ] `VerNotes` 详实版与 `YeonBriefs` 简报都已就位，简报已通知珱垣
- [ ] **前端产物已同步** `backend/webui/dist`（版本号或前端改动后必做）
- [ ] Release Notes「已知问题」如实填写
- [ ] Docker 工作流是否成功（打 tag 后到 Actions 页面确认）
- [ ] 无 `PROMOTION_NOTES_*` 类宣传稿由 AI 代写（宣传归珱垣）

---

## 附录 D：文档职责边界

| 文档 / 区域 | 维护者 | 说明 |
| --- | --- | --- |
| `README.md` | **珱垣** | AI 不写、不改；只在简报里列「待改点」 |
| `YeonPlans/` | **珱垣** | **AI 只读**，未经显式点名授权不得修改 |
| `CHANGELOG.md` | AI | 跨版本一览 |
| `DSPlans/VerNotes/RELEASE_NOTES_vX.Y.Z.md` | AI | 单版本详实技术版 |
| `DSPlans/YeonBriefs/BRIEF_vX.Y.Z.md` | AI | 给珱垣的精炼素材 |
| `PROJECT_TREE.md` / `DSPlans/VerNotes/wiki.md` | AI | 技术文档 |
| GitHub Release 正文 / 贴文宣传 / 预览图 | **珱垣** | 产品与展示口径 |
