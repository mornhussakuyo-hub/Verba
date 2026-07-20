# Changelog

本项目遵循语义化版本。首个公开开发版本从 0.1.0 开始。

## Unreleased

- 新增类型化作用域和函数参数、字段路径、条件、返回路径、optional、result / try 检查。
- 新增 `match` / `case` 解析、类型检查和 Go `switch` 代码生成。
- 新增正则编译期验证与预编译资源、HTML/text 模板槽位检查和默认 HTML 转义。
- 修复 JSON 解码错误被忽略的问题，并实现可失败 UUID 解析和随机 UUID v4 生成。
- 新增多章节中文入门教程、五个学习项目、跨平台全量验证脚本和 HTTP 端到端测试。

## 0.1.0 - 2026-07-14

- 新增 `verba check`，支持多文件解析、名称/作用域检查、类型引用检查和 HTTP 路由检查。
- 新增 JSON 语法岛校验，以及 SQL 命名参数与 `with` 绑定检查。
- 新增幂等的 `verba fmt`、CI 友好的 `--check` 和单文件 `--stdout`。
- 新增 `verba build` Go 后端以及 `verba run`。
- 支持生成记录、枚举、函数和基于 `net/http` 的路由。
- 新增 hello HTTP 服务、单元测试、端到端构建脚本和跨平台 CI。
