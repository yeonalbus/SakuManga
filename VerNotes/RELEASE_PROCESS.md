# SakuHentai 发布流程（Release Process）

> 每次发版必须按此清单执行。本文件为**固定流程**，记录从「敲定版本号」到「发布」的全部步骤，避免遗漏。
>
> 最近一次执行：v1.3.0（2026-08）

---

## 0. 流程总览

```
敲定版本号 → 更新版本号 → 整理改动清单 → 维护 PROJECT_TREE → 维护 README
    → 生成 Release Notes → 验证（测试/构建） → 打包冒烟 → git 提交 + tag → 发布
```

---

## 1. 敲定版本号

按[语义化版本（SemVer）](https://semver.org/lang/zh-CN/)选择：

| 类型 | 示例 | 适用场景 |
| ---- | ---- | -------- |
| MAJOR | `2.0.0` | 破坏性变更 / 重大架构重构 / 里程碑式发布 |
| MINOR | `1.3.0` | 向后兼容的新功能 + 修复 + 优化（最常用） |
| PATCH | `1.2.1` | 仅纯 bug 修复，无新功能 |

**决策依据**：本次改动是否引入破坏性变更（数据库无法平滑迁移、配置格式不兼容、API 不向后兼容）。若仅新增功能 + 修复且数据可 AutoMigrate 平滑迁移，选 MINOR。

---

## 2. 更新版本号

版本号唯一来源是 [`package.json`](../package.json)，修改后**必须同步**以下文件：

| 文件 | 位置 | 说明 |
| ---- | ---- | ---- |
| `package.json` | 顶部 `version` 字段 | **唯一版本来源**，About 页 / 打包脚本自动读取 |
| `package-lock.json` | 顶部 `version` 与 `packages[""].version` 两处 | 历史遗留可能为 `0.0.0`，发版时一并对齐 |

无需手动修改：
- 前端「关于」页 [`AboutSettings.vue`](../src/components/settings/AboutSettings.vue) 自动 `import { version } from '../../../package.json'`
- 打包脚本 [`build-release.bat`](../build-release.bat) 自动 `node -p "require('./package.json').version"`

---

## 3. 整理改动清单

基于 git 历史生成 Release Notes 素材：

```bat
git tag -l                                   :: 查看已有 tag（命名规范：SakuHentai-X.Y.Z）
git log <上一tag>..HEAD --oneline --no-decorate   :: 列出本次发布涉及的全部提交
```

按提交 type 归类：`feat` → 新增功能、`fix` → Bug 修复、`refactor/perf` → 优化改进。

---

## 4. 维护 PROJECT_TREE.md

更新 [`PROJECT_TREE.md`](../PROJECT_TREE.md)：

- [ ] 文件头「版本：vX.Y.Z · 最近更新：YYYY-MM」
- [ ] 「目录总览」：新增/删除的顶层目录（如 `VerNotes/`）
- [ ] 前端 `src/`：新增的 `components / composables / stores / utils / views` 文件
- [ ] 后端 `backend/`：新增的 `handlers / services / models` 文件与测试文件
- [ ] 设置面板细分标题中的版本号
- [ ] 「功能 → 文件快速定位索引」：补充新增功能对应的定位文件
- [ ] 「发布注意」：更新版本号与流程引用

---

## 5. 维护 README.md

更新 [`README.md`](../README.md)：

- [ ] 「当前状态」：版本号 + 一句本期总结 + 功能亮点列表
- [ ] 「后续计划」：更新下一阶段方向
- [ ] 「设置说明」表格：新增设置项（如偏好里的「默认收藏夹」「本地优先加载」）
- [ ] 「项目结构图」：新增目录 / 文件
- [ ] （可选）「预览图」：UI 有较大变化时更新截图

---

## 6. 生成 Release Notes

在 [`VerNotes/`](../VerNotes/) 下新建 `RELEASE_NOTES_vX.Y.Z.md`，按附录 A 模板撰写：

- 引言：自上一版本以来做了什么、遗留问题解决情况
- `✨ 新增功能`：用户可感知的新能力
- `🛠️ 优化改进`：体验 / 性能 / 稳定性优化
- `🐛 Bug 修复`：关键修复（面向用户描述）
- `⚠️ 已知问题`：诚实列出遗留限制
- `📖 使用说明`：下载 / 运行 / 默认账号 / headless / 升级说明

---

## 7. 验证

发布前必须通过：

```bat
:: 后端：全部单元测试
cd backend && go test ./...

:: 前端：类型检查（构建含 type-check）
npm run type-check
:: 或完整构建
npm run build
```

> 失败则不发布：先修复，再重新验证。

---

## 8. 打包与冒烟测试

```bat
build-release.bat
```

- [ ] 根目录生成 `SakuHentai.exe`（内嵌前端 + 后端 + 托盘 + 自定义图标）
- [ ] 脚本标题显示版本号为本次目标版本
- [ ] 双击运行 → 系统托盘出现图标 → 打开界面正常
- [ ] 「关于」页版本号与本次目标一致
- [ ] （可选）`--headless` 模式可正常启动

---

## 9. git 提交 + 打 tag

```bat
git status && git diff --stat     :: 确认改动范围
git add -A
git commit -m "chore(release): v1.3.0 发布准备（版本号/项目树/README/Release Notes）"
git tag SakuHentai-1.3.0          :: tag 命名规范：SakuHentai-X.Y.Z
```

tag 命名必须与历史一致（`SakuHentai-1.0.0`、`SakuHentai-1.1.0`、`SakuHentai-1.2.0`）。

---

## 10. 发布（可选）

若通过 GitHub Release 分发：

- [ ] 推送 tag：`git push origin SakuHentai-1.3.0`
- [ ] 创建 Release：标题 `v1.3.0`，正文粘贴 Release Notes
- [ ] 附件：上传 `SakuHentai.exe`（如仓库允许）

---

## 附录 A：Release Notes 模板

```markdown
# SakuHentai vX.Y.Z

> 自 vA.B.C 以来，……（一句本期总结，含遗留问题解决情况）

## ✨ 新增功能

- （功能 1）
- （功能 2）

## 🛠️ 优化改进

- （优化 1）

## 🐛 Bug 修复

- （修复 1）

## ⚠️ 已知问题

- （遗留限制）

## 📖 使用说明

- 下载 `SakuHentai.exe` 后双击即可运行……
- 默认监听端口 `8081`……
- 首次启动自动创建管理员账号 `admin` / `admin123`……
- NAS / 无界面环境请使用 `SakuHentai.exe --headless`……
- 升级安装：直接替换 `SakuHentai.exe` 即可，`manga.db` / `data/` / `config.json` 跟随 exe 目录自动保留；数据库结构由启动时自动迁移……
```

---

## 附录 B：发布前自查清单

- [ ] `git status` 仅含预期改动，无临时脚本 / 调试产物
- [ ] 无敏感信息随发布（如 `backend/config.json` 代理地址，PROJECT_TREE 发布注意已提示）
- [ ] Release Notes「已知问题」如实填写
- [ ] 版本号三处一致：`package.json` / `package-lock.json` / Release Notes 标题
- [ ] README 预览图是否需要更新（UI 大改时）
- [ ] `cmd_debug/` 调试工具是否需从发布产物排除（不影响主程序，可选）
