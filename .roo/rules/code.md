# Code 模式 Git 自动提交与推送规则

每次完成代码修改并通过验证后，必须自动提交并推送到远程仓库（origin），禁止只提交不推送，禁止等待用户手动操作。

## 流程
1. 修改完成后先运行验证：后端 `cd backend && go test ./...`；前端 `npm run type-check`（或 `npm run build`）。
2. 验证通过后执行 `git status` 与 `git diff --stat` 确认改动范围。
3. **推送前安全检查**（提交前必须做）：
   - 核对 `git status` 的修改/未跟踪文件清单，确认没有应被 `.gitignore` 覆盖的运行时数据、构建产物、测试素材被误纳入（如 `manga.db`、`/data/`、`/Server/`、`/MangaExamlpe`、`/Test`、`backend/cover_cache/` 等）。
   - 改动内容不得包含账号凭据/密钥/敏感值：对照 `.dsh/redact.txt` 中的脱敏字段（EH 账号 ipb_pass_hash / igneous / sk），并确认测试账号类文档（`SakuManga测试账号,网站与Ehentai账户.md`）未被跟踪。
   - 若发现敏感文件已被跟踪，先 `git rm --cached <文件>` 移出并追加到 `.gitignore`，再继续。
4. 执行 `git add -A` 暂存全部改动（敏感文件已按上一步排除）。
5. 执行 `git commit`，提交信息必须描述实际改动内容，禁止使用「更新」「debug」等笼统消息。
6. 执行 `git push` 推送当前分支；若远程有新提交，先 `git pull --rebase` 再推。
7. 验证失败则不提交：修复后再提交，并在回复中说明失败原因。

## 推送失败处理
- push 失败（网络/认证/冲突）：**保留本地提交**，在回复中明确报告失败原因、已执行的命令与可重试的命令，不静默吞掉、不删除提交。
- 认证失败时检查 `git config credential.helper` 与 Windows 凭据管理器，必要时提示用户重新登录。

## Commit Message 规范（Conventional Commits 中文语义化）
格式：`<type>(<scope>): <中文描述>`
- type：feat / fix / refactor / style / docs / test / chore
- scope：backend / frontend / archive / offline / download / settings 等
- 示例：`feat(archive): 实现归档多线程并发下载`、`fix(offline): 持久化画廊被删状态并过滤更新列表`

## 提交粒度
- 改动较大时，按功能模块拆分为多个 commit（后端与前端可分开提交）。
- 改动较小或相互关联时，可合并为一个综合 commit，但消息需列出主要改动点。

## 测试与 Debug 授权
实施与验证过程中，允许编写额外脚本（Node/Playwright 等）并实际运行浏览器进行端到端测试与 Debug，替代纯静态审查。包括但不限于：启动本地服务（vite dev/preview、后端）、驱动真实浏览器验证交互、拦截/模拟请求断言行为、构建临时产物用于验证（验证后清理）。验证结果需在回复中说明。
