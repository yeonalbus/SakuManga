# Code 模式 Git 自动提交规则

每次完成代码修改并通过验证后，必须自动提交到 git，禁止等待用户手动提交。

## 流程
1. 修改完成后先运行验证：后端 `cd backend && go test ./...`；前端 `npm run type-check`（或 `npm run build`）。
2. 验证通过后执行 `git status` 与 `git diff --stat` 确认改动范围。
3. 执行 `git add -A` 暂存全部改动。
4. 执行 `git commit`，提交信息必须描述实际改动内容，禁止使用「更新」「debug」等笼统消息。
5. 验证失败则不提交：修复后再提交，并在回复中说明失败原因。

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
