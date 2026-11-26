# 贡献指南（Contributing）

感谢你关注 **SakuManga**！

## 协作说明

本项目主要由作者负责产品规划与路线设计，并结合 AI 辅助进行开发与维护。

- **小修小补**（如文档错别字、明确的 Bug 修复、小范围体验优化）欢迎直接提交 Pull Request。
- **较大功能或架构改动**建议先在 [Issues](https://github.com/yeonalbus/SakuManga/issues) 或 [Discussions](https://github.com/yeonalbus/SakuManga/discussions) 中沟通确认方向，避免重复开发或偏离定位。

---

## 本地开发环境

### 环境要求

- **Go**：1.26+（参见 `backend/go.mod`）
- **Node.js**：22.18+ 或 24.12+（参见 `package.json`）

### 启动开发模式

```bash
# 1. 安装前端依赖
npm install

# 2. 启动后端服务（默认监听 http://127.0.0.1:8081）
cd backend && go run .

# 3. 另开终端启动前端开发服务器（Vite 会自动代理 API 到后端）
npm run dev
```

### 单文件打包（Windows）

```bat
build-release.bat
```

> 说明：后端通过 `//go:embed all:dist` 内嵌 `backend/webui/dist` 静态资源，若仅修改前端代码并需要重新打包后端，请先执行 `npm run build` 并将 `dist/` 同步至 `backend/webui/dist`。

---

## 提交前检查与规范

提交 PR 前请确认以下检查通过：

```bash
# 后端单元测试
cd backend && go test ./...

# 前端类型检查与代码检查
npm run type-check
npm run lint
```

### Commit Message 规范

建议遵循 Conventional Commits 语义化格式：

```text
<type>(<scope>): <简明描述>
```

常见 `type`：`feat`（新功能）、`fix`（修复）、`refactor`（重构）、`docs`（文档）、`style`（格式）、`test`（测试）、`chore`（构建/杂项）。

---

## 协议与合规边界

1. **开源协议**：本项目源码采用 **[AGPL-3.0](LICENSE)** 开源，提交代码即视为同意你的贡献同样以 AGPL-3.0 协议分发。
2. **禁止提交敏感或运行时数据**：切勿将 `manga.db`、`config.json`、`data/`、`cover_cache/`、个人 E 站 Cookie / 凭据提交入库。
3. **第三方数据**：运行时拉取的 EhTagTranslation 标签汉化数据遵循 `CC BY-NC-SA 3.0`，请勿将原始数据文件静态提交进源码仓库。
