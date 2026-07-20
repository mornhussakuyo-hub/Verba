# Verba 入门教程

这套教程面向第一次接触 Verba、编译器或后端开发的读者。章节按学习顺序排列，每一章只引入少量新概念，并给出可以直接运行或检查的示例。

## 学习路线

1. [安装与环境](01-installation.md)：构建 `verba` 命令并确认环境。
2. [第一个 Verba 程序](02-first-program.md)：理解模块、能力、路由与代码块。
3. [语言基础](03-language-basics.md)：变量、字面量、调用和控制流。
4. [函数与类型](04-functions-and-types.md)：record、enum、optional 与 result。
5. [HTTP 服务](05-http-service.md)：路径参数、请求数据和响应。
6. [语法岛](06-syntax-islands.md)：JSON、Regex、HTML、Text 与 SQL。
7. [工具链与排错](07-tooling.md)：check、fmt、build、run 和诊断。
8. [项目清单](08-project-manifest.md)：项目身份、构建目标、数据库和依赖元数据。
9. [数值类型与精确小数](09-numeric-types.md)：整数范围、上下文常量、浮点数和 decimal。
10. [PostgreSQL](10-postgresql.md)：schema 快照、类型化查询、结果行和事务。
11. [完整用户服务](11-user-service.md)：组合 HTTP、JSON、UUID、正则、PostgreSQL 与类型化路由错误。

## 建议学习方式

每一章都按同一个节奏完成：

1. 先阅读对应的 `learn/` 源码，再对照章节中的语法说明。
2. 运行该章给出的 `fmt --check`、`check` 或 `build` 命令。
3. 对 HTTP 项目实际启动服务，并执行章节中的请求。
4. 修改一个字面量、类型或参数，观察诊断后再恢复源码。

教程中的八个项目形成一条递进路线：

| 项目 | 对应章节 | 完成标志 |
| --- | --- | --- |
| `learn/01_hello` | 1–2 | 能启动服务并访问两个 GET 路由 |
| `learn/02_basics` | 3 | 能解释绑定、调用和控制流的检查规则 |
| `learn/03_types` | 4 | 能使用 record、enum、optional 和 result |
| `learn/04_http` | 5 | 能发送路径参数和 JSON 请求，并理解错误状态 |
| `learn/05_islands` | 6 | 能检查并渲染 JSON、Regex、HTML 与 Text 岛 |
| `learn/06_numeric` | 9 | 能区分整数、浮点数和精确 decimal |
| `learn/07_postgres` | 8、10 | 能创建、查询并更新同一条数据库记录 |
| `learn/08_user_service` | 11 | 能验证 201、200、400、404，并理解 500 错误边界 |

## 约定

- Verba 源文件扩展名是 `.vrb`。
- 示例命令默认在仓库根目录运行。
- Windows 示例使用 PowerShell；Linux 和 macOS 将 `verba.exe` 换成 `verba`。
- 每个示例目录是一个独立项目。不要把多个使用相同模块名或声明名的教程目录一次性交给 `verba check`。

## 一分钟体验

```powershell
go build -o build/verba.exe ./cmd/verba
./build/verba.exe check learn/01_hello
./build/verba.exe run learn/01_hello
```

另开一个终端：

```powershell
curl http://127.0.0.1:8080/
curl http://127.0.0.1:8080/hello/Alice
```

回到运行服务的终端按 `Ctrl+C` 停止进程。

## 一次验证全部教程

仓库的验证脚本会格式检查、静态检查、审计并构建八个教程项目；数据库项目只在运行时需要 PostgreSQL：

```powershell
./scripts/verify.ps1
```

脚本最后输出 `Verba verification completed` 即表示所有教程源码都通过当前工具链。CI 会在 Windows 和 Ubuntu 上运行同一脚本，因此章节中的项目始终与编译器同步。

下一步从[安装与环境](01-installation.md)开始。
