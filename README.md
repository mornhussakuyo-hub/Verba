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

## CLI

```text
verba check [paths...]
verba fmt [--check] [--stdout] [paths...]
verba build [-o output] [--emit-go path] [paths...]
verba run [paths...] [-- program-arguments...]
verba version
verba help
```

- `check` 解析所有输入文件并执行模块、声明、类型、作用域、路由与语法岛检查。
- `fmt` 就地应用唯一格式；`--check` 只检查，适合 CI；`--stdout` 输出单个文件的格式化结果。
- `build` 生成 Go 主程序并调用 `go build`。默认输出到 `build/<module>`。
- `run` 在临时目录构建，然后以前台进程运行。

目录输入会递归查找 `.vrb` 文件，并跳过隐藏目录、`build`、`dist` 和 `vendor`。

## 0.1.0 支持范围

前端支持：

- `module`、`use`、`record`、`enum`、`function`、`route` 和 `embed`。
- `let`、`var`、`set`、`call`、`if` / `else`、`for`、`while`、`return`、`respond` 和 `transaction` 的解析。
- 基础类型以及 `optional`、`list`、`map`、`result` 类型构造。
- `get`、`is` / `is not`、`try call` 和命名参数块。
- JSON 语法校验，以及 SQL 命名参数和 `with` 绑定的一致性检查。
- JSON、文本和空 HTTP 响应的 Go 代码生成。

0.1.0 的 Go 后端暂不生成数据库执行代码。SQL 岛及绑定可以通过 `verba check` 验证；包含 `sql_exec`、`sql_one`、`sql_optional`、`sql_many` 或 `transaction` 的程序在 `build` 阶段会收到明确诊断。这样不会在未选择数据库驱动和连接模型时生成行为不可靠的程序。

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
internal/parser        行导向解析器与语法岛扫描
internal/check         名称、类型、作用域和适配器检查
internal/format        幂等格式化器
internal/emitgo        Go 后端
internal/compiler      文件发现与编译流水线
internal/cli           终端命令
```

## 版本状态

版本：`0.1.0`

这是首个可用的垂直切片，不代表 [design.md](design.md) 中所有 MVP 条目均已完成。当前重点是验证 Verba 的核心结构、诊断体验和 Go 后端闭环。
