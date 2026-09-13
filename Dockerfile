# syntax=docker/dockerfile:1

# ─────────────────────────────────────────────────────────────
# SakuManga 官方镜像（多架构：linux/amd64 + linux/arm64）
#
# 自动构建：推送 SakuManga-X.Y.Z tag 或手动触发（见 .github/workflows/docker-publish.yml）
# 本地构建：build-docker.bat（Windows）/ 见 VerNotes/wiki.md「Docker 部署」
#
# 设计说明：
#   - 仓库已入库 backend/webui/dist（前端构建产物），构建阶段无需 Node；
#   - 后端为纯 Go（SQLite 驱动 glebarez/sqlite 为纯 Go 实现），CGO_ENABLED=0 静态编译；
#   - 非 Windows 平台托盘/系统代理为无操作 stub，容器内以 --headless 模式运行。
# ─────────────────────────────────────────────────────────────
FROM golang:1.26-bookworm AS build

ARG BUILD_ID=dev
WORKDIR /src

# 依赖单独一层：go.mod / go.sum 未变时复用缓存，加快后续构建
COPY backend/go.mod backend/go.sum ./
RUN go mod download

COPY backend/ ./
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath \
      -ldflags "-s -w -X SakuManga/internal/version.Build=${BUILD_ID}" \
      -o /out/SakuManga .

FROM debian:bookworm-slim

# ca-certificates：访问 E-Hentai / GitHub 需要 HTTPS 证书
# tzdata：发布时间、更新扫描时刻等按本地时区换算
RUN apt-get update \
 && apt-get install -y --no-install-recommends ca-certificates tzdata \
 && rm -rf /var/lib/apt/lists/*

ENV TZ=Asia/Shanghai
WORKDIR /app

COPY --from=build /out/SakuManga /opt/SakuManga/SakuManga
COPY docker-entrypoint.sh /entrypoint.sh
RUN chmod +x /entrypoint.sh

# 程序以「可执行文件所在目录」为数据目录（manga.db / config.json / data / logs 都在这里）。
# 声明为卷：用户忘记显式挂载时，数据仍不会随容器删除而丢失。
VOLUME ["/app"]

EXPOSE 8081

LABEL org.opencontainers.image.title="SakuManga" \
      org.opencontainers.image.description="自建 E-Hentai 漫画管理器（在线浏览 + 本地库 + 阅读器）" \
      org.opencontainers.image.source="https://github.com/yeonalbus/SakuManga" \
      org.opencontainers.image.licenses="AGPL-3.0"

ENTRYPOINT ["/entrypoint.sh"]
