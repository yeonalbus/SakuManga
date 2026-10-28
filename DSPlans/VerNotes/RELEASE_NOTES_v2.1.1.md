# SakuManga v2.1.1

> 自 v2.1.0（2026-09）以来，本版以**官方 Docker 镜像**为主题：正式提供 `ghcr.io/yeonalbus/sakumanga` 镜像（同时支持 `linux/amd64` 与 `linux/arm64`），NAS / 群晖 / 树莓派等 Linux 环境可直接拉取部署，无需 Windows exe 与 Go 编译环境。程序功能与 v2.1.0 一致（本版无功能与逻辑改动），升级沿用原方式即可。

## ✨ 新增功能

- **官方 Docker 镜像（GHCR）**：`ghcr.io/yeonalbus/sakumanga`，标签策略 `2.1.1` / `2.1` / `latest`，同时提供 `linux/amd64` 与 `linux/arm64` 两种架构（群晖、树莓派、Apple Silicon 等 arm64 设备可直接运行）
- **GitHub Actions 自动构建发布**：推送 `SakuManga-*` tag 时自动构建并推送镜像，也可在仓库 Actions 页面手动触发（`Docker Image (GHCR)` → Run workflow）
- **本地一键镜像脚本 `build-docker.bat`**：本机装了 Docker 时可用——`build-docker.bat` 构建并载入本地镜像供测试，`build-docker.bat push` 构建多架构并推送到 GHCR

## 🛠️ 优化改进

- **常见问题手册新增「九、Docker 部署」**：官方镜像用法、自建镜像的 Dockerfile、数据持久化（整目录挂载，避免 SQLite 的 `-wal` 半挂载丢数据）、容器内代理设置、升级方式、`.dockerignore` 与构建上下文等注意事项
- **README 补充 Docker 部署入口**：在「构建与运行」中与 exe / `--headless` 并列，方便 NAS 用户直接找到

## 🐛 Bug 修复

- 本版无功能与缺陷修复（与 v2.1.0 相同的代码，仅新增镜像发布能力与文档）

## ⚠️ 已知问题

- **镜像未在作者本机实测**：项目作者本地没有 Docker 环境，镜像由 GitHub Actions 在云端构建完成；首次使用若遇到启动、架构或挂载问题，请在 Issue 中反馈（附 `docker logs` 输出），会尽快修复
- 与 v2.1.0 相同的已知问题仍然存在：「疑似重复」簇暂不支持勾选删除；本地导入的书建立父子画廊关系需跑一次联网维护查重，且全量重扫会重置该关系；搜刮书签锚点定位依赖列表已加载
- 项目仍处于快速迭代期，可能存在未覆盖的边缘场景

## 📖 使用说明

- **Docker（推荐 NAS / Linux 长期运行）**：

  ```bash
  docker run -d --name sakumanga --restart unless-stopped \
    -p 8081:8081 \
    -v /your/data/dir:/app \
    ghcr.io/yeonalbus/sakumanga:2.1.1
  ```

  访问 `http://<主机IP>:8081`；数据（`manga.db`、`config.json`、`data/`、`logs/`）全部保存在挂载目录，删容器不丢数据；升级 = 换镜像标签后重建容器。
- **Windows exe**：下载 `SakuManga.exe` 双击运行（单文件内嵌前后端 + 自定义图标），运行后最小化到系统托盘
- **纯后端（NAS 无 Docker 环境）**：`SakuManga.exe --headless`
- 默认监听端口 `8081`，可在「E 站连接 → 网络」中修改；端口被占用时自动切换空闲端口
- 首次启动自动创建管理员账号 `admin` / `admin123`，登录后请尽快在「账户」中修改
- 容器内访问 E 站超时，请在「设置 → E 站连接 → 网络」显式填写宿主机代理（容器探测不到宿主系统代理）
- 升级安装：exe 方式直接替换文件；Docker 方式换镜像标签重建容器，数据均自动保留，数据库结构由启动时自动迁移
