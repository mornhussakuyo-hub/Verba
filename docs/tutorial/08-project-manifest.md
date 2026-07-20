# 8. 项目清单

`.vrb` 文件描述程序逻辑，`verba.toml` 描述整个项目。把构建信息放在独立清单中，可以避免在每个源码文件里重复版本、后端和数据库配置。

`learn/01_hello/verba.toml` 是最小清单：

```toml
name = "hello"
version = "0.1.0"
target = "go"
```

- `name` 必须是合法的 Verba 标识符，并且与每个源码文件的 `module` 名相同。
- `version` 使用完整的语义化版本，例如 `0.1.0` 或 `1.2.0-beta.1`。
- `target` 当前只能是 `go`。

清单是标准 TOML。编译器会拒绝未知字段、无效 UTF-8、BOM、错误版本以及不受支持的目标，而不是静默忽略拼写错误。

## 清单发现

执行以下命令时，编译器会从源码共同目录开始向父目录查找最近的 `verba.toml`：

```powershell
verba check learn/01_hello
verba build learn/01_hello
```

找到清单后，默认构建产物使用 `name` 命名。没有清单的旧项目仍可构建，此时编译器使用源码中的模块名。

不要在包含多个独立项目的共同父目录放置一个共享清单。每个可独立构建的项目应拥有自己的 `verba.toml`。

## 数据库配置

启用 SQL 项目时，可以声明方言和 schema 快照：

```toml
name = "account_service"
version = "0.1.0"
target = "go"

[database]
dialect = "postgres"
schema = "schema.sql"
```

`schema` 必须是项目内存在的相对文件，不能使用绝对路径或 `..` 逃出项目目录。这为后续 SQL 行类型推导提供确定、可审查的输入。

当前编译器已经验证数据库配置和 schema 路径，但 SQL 驱动及查询执行仍在后续运行时里实现。

## 依赖声明

依赖使用 TOML 表：

```toml
[dependencies]
verba_http = "0.1"
verba_sql = "0.1"
```

依赖名必须是合法 Verba 标识符，版本约束不能为空。源码中的 `use verba_http` 必须能在依赖表找到同名条目，`verba audit` 会报告它是否使用。当前版本尚未开放外部包下载、锁文件和跨模块符号导入，因此依赖解析只证明声明关系，不代表远端源码已经安装。

## 常见诊断

如果清单名和模块名不一致：

```text
main.vrb:1:1: error VRB1003: module other does not match project hello
  hint: change the declaration to module hello or update verba.toml
```

如果 schema 逃出项目目录：

```text
verba.toml:1:1: error VRB0907: database schema must stay inside the project directory
  hint: use a relative path without escaping the project root
```

至此，你已经走完从安装、语言基础、HTTP 与语法岛，到工具链和项目组织的完整入门路线。继续阅读仓库根目录的 `design.md` 可以了解 Verba 的长期语言设计。
