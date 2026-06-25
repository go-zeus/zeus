# Security Policy

感谢您帮助 Zeus 团队发现并负责任地披露安全问题。

## 支持的版本

Zeus 当前处于 `v0.x` 预发布阶段，仅维护最新 `main` 分支。安全补丁不回填到旧版本。

| 版本 | 安全支持 |
|------|----------|
| `main` 分支 | ✅ |
| 历史标签（如有） | ❌ |

`v1.0.0` 正式发布后将按 [SemVer](https://semver.org/lang/zh-CN/) 维护策略提供版本支持矩阵。

## 报告漏洞

**请勿通过 GitHub Public Issue 报告安全问题**（公开报告会让攻击者在补丁发布前利用漏洞）。

请使用以下任一私有渠道：

### 方式一（推荐）：GitHub Private Vulnerability Reporting

在仓库主页点击 **Security** → **Report a vulnerability**，按模板填写。
此渠道端到端加密，仅仓库管理员可见。

### 方式二：邮件

发送邮件至：**security@go-zeus.dev**（占位邮箱，正式邮箱将在 v1.0.0 前公开）

主题行使用 `[SECURITY] Zeus - <简短描述>`。

**报告内容请包含**：

- 受影响包路径（例如 `github.com/go-zeus/zeus/server/http`）
- 复现步骤（最小可复现示例最佳）
- 影响评估（远程/本地、机密性/完整性/可用性）
- 建议的修复方向（可选）

## 响应 SLA

| 阶段 | 时间窗 |
|------|--------|
| 确认收到报告 | 48 小时内 |
| 初步评估（是否受理） | 7 天内 |
| 补丁发布（严重漏洞） | 30 天内 |
| 补丁发布（普通漏洞） | 90 天内 |

如未在 SLA 内回复，欢迎再次跟进（团队成员可能错过通知）。

## 报告范围

### 受理

- 框架核心包的 SQL 注入、XSS、SSRF、认证绕过、路径穿越、反序列化漏洞
- 默认配置下的不安全行为（例如 server/http 默认未启用 timeout）
- 依赖的第三方库存在已知 CVE（仅当主仓使用受影响版本时）
- plugins/ 下插件的安全问题

### 不受理

- 用户在业务代码中误用框架导致的漏洞（请阅读文档）
- 仅在用户禁用默认安全配置（如显式 `WithoutAutoClustering()`）后才能触发的问题
- DoS 类攻击需要极高权限或本地访问的场景
- 社会工程学攻击

## 责任披露

- 我们遵循 [Responsible Disclosure](https://en.wikipedia.org/wiki/Responsible_disclosure) 原则
- 在补丁发布前，**不会公开披露漏洞详情**
- 报告者将在补丁发布时获得致谢（如愿意具名）
- 不提供漏洞赏金（bounty），但欢迎社区贡献

## 安全最佳实践（用户侧）

使用 Zeus 时建议遵循以下实践：

1. **升级到最新 `main`**：v0.x 阶段，安全修复即时合入 main
2. **启用 server timeout**：默认装配已包含合理超时，自定义 server 时显式设置
3. **recovery 中间件必启**：L1 `app.Run` 默认启用，L3/L4 用户需 `WithMiddleware(recovery.New())`
4. **生产环境关闭 debug log**：避免敏感字段（cluster/baggage）写入日志文件
5. **`cache/key` 默认不记录**：避免敏感 key 进入 trace（`WithRecordKey(true)` 谨慎开启）
6. **HTTPS**：生产部署务必在入口启用 TLS（zeus 本身只关注应用层，HTTPS 由反向代理层处理）

## 联系

- 一般问题：[GitHub Discussions](https://github.com/go-zeus/zeus/discussions)
- Bug 反馈（非安全）：[GitHub Issues](https://github.com/go-zeus/zeus/issues)
- 安全问题：上述私有渠道
