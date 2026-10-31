# AGENTS.md — SakuManga 项目协作约定

本文件是所有 AI 编码助手（Claude Code / Roo Code / Cline / DSH 等）的项目级工作约定，开始任何任务前先阅读。**核心要求：每次任务收尾必须「提交 + 推送」到远程，禁止只提交不推送。**

## 项目速览

- SakuManga：前端 Vue 3 + Vite，后端 Go，详见 `README.md` 与 `PROJECT_TREE.md`
- 远程仓库：`https://github.com/yeonalbus/SakuManga.git`（默认分支 `main`）
- 工作目录：`G:\SakuProjects\SakuManga`

## Git 工作流（每次任务收尾必做）

1. **验证**：改完先验证——后端 `cd backend && go test ./...`；前端 `npm run type-check`（或 `npm run build`）。文档类改动无需代码验证，但需检查改动范围。
2. **检查改动**：`git status` + `git diff --stat` 确认范围与是否混入无关文件。
3. **安全检查**：
   - 确认没有应被忽略的运行时数据/构建产物/测试素材被暂存（见下方「不可入库」清单，禁止 `git add -f` 强加）。
   - 改动不得包含账号凭据/密钥/敏感值：对照 `.dsh/redact.txt` 脱敏字段（EH 账号 ipb_pass_hash / igneous / sk），测试账号类文档禁止入库。
   - 发现敏感文件已被跟踪：`git rm --cached <文件>` 移出 + 追加 `.gitignore` 后继续。
4. **提交**：`git add -A` → `git commit`。Commit Message 遵循 Conventional Commits 中文语义化：`<type>(<scope>): <中文描述>`，type ∈ feat/fix/refactor/style/docs/test/chore。一条 commit 一件事，不夹带无关文件；改动大时按模块拆分。**`YeonPlans/` 是珱垣独占目录，不进入 AI 的提交**（未经显式授权的改动留给珱垣自行处理）。
5. **推送**：`git push`；远程有新提交则先 `git pull --rebase` 再推。
6. **失败处理**：push 失败（网络/认证/冲突）→ **保留本地提交**，向用户明确报告原因与可重试命令，不静默、不删提交。

## 不可入库清单（.gitignore 已覆盖）

- 运行时数据：`manga.db*`、`/data/`、`backend/data/*`、`backend/cover_cache/`
- 本地资产：`/_local/`（测试素材 `MangaExamlpe`、参考源码 `JHentai`、本机实例 `Test`/`Server`、图标备份 `icon`）
- 测试账号：`SakuManga测试账号,网站与Ehentai账户.md`
- 构建产物：`/dist`（根目录）、`/SakuManga*.exe`、`*.tsbuildinfo`、`logs/`
- 工具目录：`.dsh/`（含 skill 与脱敏清单，仅本机使用，不入库）
- 调试输出：`backend/ziprangecheck_out/`

## 文档职责边界（v2.3.0 起）

- **`README.md` 由珱垣本人维护**：AI 只读、不写、不改。发版时只在 `DSPlans/YeonBriefs/BRIEF_vX.Y.Z.md` 的「README 待改点」里列出需要改的位置与要点，不代写正文。
- **`YeonPlans/` 是珱垣独占目录**：AI **只读**。仅当珱垣在对话中显式点名授权某个文件时，AI 才可修改；未授权一律不碰。
- **AI 维护的技术文档**：`CHANGELOG.md`、`DSPlans/VerNotes/`（详实 Release Notes、`wiki.md`、发布流程）、`DSPlans/YeonBriefs/`（给珱垣的版本简报）、`PROJECT_TREE.md`。
- **宣传与展示口径归珱垣**：GitHub Release 正文、贴文、预览图，AI 不代写宣传稿。
- 发布流程见 `DSPlans/VerNotes/RELEASE_PROCESS.md`；双方职责见 `YeonPlans/协作方案/职责分配.md`（只读参照）。

## 红线

- 禁止提交 `.env`、密钥、测试账号凭据；线上账号勿改密、勿删库
- 推送前不确定的文件先问用户，不擅自决定
