# Verba

Verba 是一门面向 Web 后端的静态类型语言。它使用单词、换行和 `begin` / `end` 组织核心程序结构，并通过显式语法岛接纳 JSON、SQL、HTML 和正则表达式。

仓库当前包含 Verba 0.1.0 的首个可运行工具链。编译器使用 Go 实现，CLI 可以检查、格式化、生成 Go 代码并构建可执行的 HTTP 服务。

## 快速开始

需要 Go 1.24 或更新版本。

```powershell
go build -o build/verba.exe ./cmd/verba
./build/verba.exe version
./build/verba.exe check examples/hello
./build/verba.exe build -o build/hello.exe examples/hello
./build/hello.exe
```

也可以将 CLI 安装到 Go 的可执行目录：

```powershell
go install ./cmd/verba
```

服务默认监听 `:8080`：

```text
GET /health
GET /greet/{name}
```

可以通过 `VERBA_ADDRESS` 修改监听地址。

PostgreSQL 项目还需要 `VERBA_DATABASE_URL`。生成程序使用 `database/sql` 与 pgx，并在启动时验证连接。

## CLI

```text
verba check [paths...]
verba fmt [--check] [--stdout] [paths...]
verba build [-o output] [--emit-go path] [paths...]
verba run [paths...] [-- program-arguments...]
verba audit [--json] [paths...]
verba version
verba help
```

- `check` 解析所有输入文件并执行模块、声明、类型、作用域、路由与语法岛检查。
- `fmt` 就地应用唯一格式；`--check` 只检查，适合 CI；`--stdout` 输出单个文件的格式化结果。
- `build` 生成 Go 主程序并调用 `go build`。默认输出到 `build/<module>`。
- `run` 在临时目录构建，然后以前台进程运行。
- `audit` 输出模块实际声明的能力和清单依赖；`--json` 适合 CI、容器策略和安全审查工具。

目录输入会递归查找 `.vrb` 文件，并跳过隐藏目录、`build`、`dist` 和 `vendor`。

项目可以提供 `verba.toml`，声明项目名、语义化版本、Go 目标、PostgreSQL schema 快照和依赖元数据。编译器会向父目录查找最近的清单，并检查清单名与源码 `module` 一致。没有清单的项目仍保持兼容。

## 0.1.0 支持范围

前端支持：

- `module`、`use`、`record`、`enum`、`function`、`route` 和 `embed`。
- `let`、`var`、`set`、`call`、`if` / `else`、`match` / `case`、`for`、`while`、`return`、`respond` 和 `transaction` 的解析。
- 基础类型以及 `optional`、`list`、`map`、`result` 类型构造。
- `get`、`is` / `is not`、`try call` 和命名参数块。
- JSON 与正则语法校验、HTML/text 模板槽位检查，以及 PostgreSQL schema、命名参数、列类型和结果行检查。
- 类型化作用域、函数参数、字段路径、条件、返回路径、optional 和 result / try 检查。
- 数值字面量按参数、返回值、赋值和算术上下文定型，并在编译期检查整数宽度与浮点范围。
- `decimal` 使用任意精度精确运算和无损 JSON number 编解码，不会降为 `float64`。
- JSON 解码与 UUID 解析使用真实 `result` 错误路径；HTML 模板默认转义，正则资源预编译。
- JSON、文本和空 HTTP 响应，以及 `sql_exec`、`sql_one`、`sql_optional`、`sql_many` 和显式事务的 Go 代码生成。
- `output result T E` 路由会把成功返回值编码为 200 JSON，并将请求错误、未找到和数据库错误稳定映射为 400、404 和 500。
- SQL 构建使用固定版本 `pgx/v5.7.6`；精确 decimal 支持数据库扫描和参数绑定，事务在 `try` 失败时回滚、正常退出时提交。
- `use` 能力与 `verba.toml` 依赖解析、缺失能力检查和确定性 capability 审计。

当前 SQL 适配器聚焦可静态证明的直接单表 PostgreSQL 语句。JOIN、子查询、集合运算、动态标识符、迁移执行和嵌套事务仍会被拒绝或留待后续版本；schema 快照与运行数据库的一致性由部署流程负责。

完整语言方向与后续范围见 [design.md](design.md)。

## 示例

```verba
module hello

use http

route health
method get
path /health
begin
    respond text 200 Verba is healthy
end
```

## 入门教程

从 [Verba 入门教程](docs/tutorial/README.md) 开始，通过十一个章节学习安装、基础语法、函数与类型、HTTP、语法岛、工具链、项目清单、精确数值、PostgreSQL 和完整用户服务。八个可执行项目位于 `learn/`。

## 开发

```powershell
go test ./...
go vet ./...
./scripts/build.ps1
./build/verba.exe fmt --check examples
./build/verba.exe check examples
```

主要包：

```text
cmd/verba              CLI 入口
internal/source        UTF-8 源文件、字节偏移与行列映射
internal/manifest      TOML 项目清单发现与验证
internal/region        核心区与原始语法岛的字节边界扫描
internal/lexer         核心 token、数值和受控字面量词法检查
internal/parser        行导向语法与 AST 构建
internal/resolve       use、依赖、能力需求与审计解析
internal/check         名称、类型、作用域和适配器检查
internal/sqlpostgres   PostgreSQL schema、参数和结果列分析
internal/format        幂等格式化器
internal/emitgo        Go 后端
internal/compiler      文件发现与编译流水线
internal/cli           终端命令
```

## 版本状态

版本：`0.1.0`

这是首个满足 [design.md](design.md) 明确 MVP 支持范围和八项验收标准的版本。它不代表完整语言设计已经结束；导入源码模块、稳定 typed IR、更丰富的适配器和语言服务器仍属于后续工作。
