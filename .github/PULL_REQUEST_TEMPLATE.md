<!-- 感谢你的贡献！请按以下模板填写 PR 描述。评审者会据此判断变更范围与影响。 -->

## What

<!-- 简述做了什么。一两句话即可。 -->

## Why

<!-- 为什么做这个改动？关联 issue：Closes #xxx，或说明动机。 -->

## How

<!-- 关键设计决策。如果有多方案，说明为何选这个。 -->

## Test

<!-- 如何验证你的改动 -->
- [ ] `go test -race -count=1 ./...` 通过
- [ ] 改了插件：`(cd plugins/xxx && GOWORK=off go test ./...)` 通过
- [ ] 改了 API：确认 L1/L2/L3/L4 兼容性（参见 [site/content/reference/api-stability.md](../site/content/reference/api-stability.md)）
- [ ] 新增代码有对应测试
- [ ] 新增代码覆盖率 ≥ 85%

## Checklist

- [ ] 导出符号有 godoc（中文注释，动词开头）
- [ ] 命名遵循 [CONTRIBUTING.md](../CONTRIBUTING.md#命名约定)（包名无下划线、首字母缩写大写等）
- [ ] 主仓不引入第三方依赖（如需依赖放 `plugins/`）
- [ ] [CHANGELOG.md](../CHANGELOG.md) 已更新（breaking / feature / fix 分类）
- [ ] commit message 遵循规范（`<type>(<scope>): <subject>`）

## Breaking Change

<!-- 如果是 breaking change，必须填写。否则删除本节。 -->

- 影响层级：L1 / L2 / L3 / L4 / plugins
- 迁移方式：
- 是否提供 Deprecated alias：是 / 否（v0.x 阶段默认不提供）

## Notes for Reviewers

<!-- 给评审者的额外说明：重点看哪里、有什么不确定的地方、需要特别测试的场景 -->
