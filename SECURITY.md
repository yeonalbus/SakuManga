# 安全策略（Security Policy）

## 支持的版本

目前仅对最新的次版本分支提供安全修复与更新：

| 版本 | 是否支持 |
| :--- | :--- |
| 最新正式版本（[Latest Release](https://github.com/yeonalbus/SakuManga/releases/latest)） | ✅ 支持 |
| 更早历史版本 | ❌ 建议升级至最新版本 |

---

## 报告安全漏洞

如果你发现了安全漏洞（例如鉴权绕过、任意文件读写、XSS、敏感凭据泄露等），**请勿直接通过公开 Issue 披露细节**，以免给正在运行自托管实例的用户带来风险。

请通过 GitHub 的 **Private Vulnerability Reporting** 私密渠道提交：

👉 [提交私密安全报告（GitHub Security Advisory）](https://github.com/yeonalbus/SakuManga/security/advisories/new)

提交时请尽量附带：
- 受影响的版本号与部署方式
- 漏洞复现步骤或 PoC 说明
- 潜在影响范围与修复建议（如有）

---

## 自托管安全基线建议

1. **修改默认密码**：首次启动会自动创建默认管理员 `admin / admin123`，登录后请立即前往「设置 → 账户」修改为高强度密码。
2. **公网暴露需前置反代与 HTTPS**：如需将 SakuManga 暴露至公网，强烈建议通过 Nginx / Caddy / Cloudflare Tunnel 等前置反向代理启用 HTTPS，并妥善配置访问控制。
3. **保护凭据与配置**：`config.json` 与 `manga.db` 中包含站点会话与账号配置信息，请勿将运行目录直接暴露在公开共享或 Web 根目录中。
4. **定期备份**：升级大版本或执行批量维护操作前，建议备份 `manga.db` 与 `config.json`。
