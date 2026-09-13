#!/bin/sh
set -e

# 程序会把 manga.db / config.json / data / logs 落在「自身可执行文件所在目录」。
# 二进制放在 /opt/SakuManga，数据目录是 /app（镜像中声明为卷，通常被显式挂载）：
# 每次启动先把二进制复制进 /app 再运行，好处是——
#   1. 可以整目录挂载数据卷（含 SQLite 的 -wal / -shm 文件），不会出现半挂载导致的丢数据；
#   2. 升级镜像后重建容器即自动用上新版本，数据卷完全不受影响。
cp -f /opt/SakuManga/SakuManga /app/SakuManga
chmod +x /app/SakuManga

exec /app/SakuManga --headless
