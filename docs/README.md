# Internal Drafts

这是 Zeus 的**内部草稿区**，存放设计讨论、未完成的 RFC、技术决策记录。

## ⚠️ 不是用户文档

**正式用户文档**在 [`site/content/`](../site/content/)，可通过以下方式查看：

```bash
cd site
hugo server    # 本地预览：http://localhost:1313/zeus/
```

推送到 `main` 分支后会自动部署到 `https://go-zeus.github.io/zeus/`。

## 目录内容

```
docs/
└── superpowers/specs/    # 设计 RFC（如 driver 架构重构）
```

## 何时在这里写

- 设计 RFC / 架构演进讨论
- 还在调研、未定型的方案
- 内部技术决策记录

文档稳定后，请迁移到 `site/content/` 对应章节（reference / guide / architecture），然后从本目录删除。
