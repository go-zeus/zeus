// Package app 提供 Zeus 应用入口。
//
// 本包实现 4 层渐进暴露 API：
//
//   - L1：app.Run(cfg, handler) —— 5 行启动（自动装配）
//   - L2：cfg.Registry URL scheme —— 配置驱动
//   - L3：app.NewApp(AddServer(s), WithLogger(l), ...) —— 类型装配（options.go）
//   - L4：components.NewApp(NewLogComponent(l), ...) —— 声明式组件装配
//
// L3 是 L4 的"扁平化版本"：用户用 WithXxx 选项模式而非逐个 NewXxxComponent。
// 内部 100% 复用 components.Container / Lifecycle / Component 体系，不绕过任何 L4 能力。
// 用户可在 NewApp(...) 参数末尾追加任意 components.NewXxxComponent(...) 实现渐进升级。
//
// 各文件分工：
//
//   - quickstart.go —— L1 入口 Run / L2 URL scheme 解析
//   - options.go —— L3 类型装配 NewApp + WithXxx 选项
//   - app.go —— L3 默认装配逻辑（applyDefaults / buildComponents）
package app
