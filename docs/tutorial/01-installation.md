# 1. 安装与环境

Verba 编译器当前使用 Go 实现，并把 Verba 程序生成为 Go 后端程序。因此开发环境需要 Go 1.24 或更新版本。

## 检查 Go

```powershell
go version
```

输出中的版本号应不低于 `go1.24`。如果命令不存在，请先从 Go 官方发行版安装，并重新打开终端。

## 构建编译器

在仓库根目录运行：

```powershell
go build -trimpath -o build/verba.exe ./cmd/verba
./build/verba.exe version
```

预期输出：

```text
verba 0.1.0
```

`-trimpath` 会从生成的二进制中移除本机源码绝对路径，使构建结果更容易复现。

## 安装到 Go 工具目录

也可以运行：

```powershell
go install ./cmd/verba
verba version
```

如果系统找不到 `verba`，请执行 `go env GOPATH`，并把其下的 `bin` 目录加入 `PATH`。

## 编辑器设置

Verba 目前使用 4 个空格缩进、UTF-8 编码且不允许 BOM。核心区由格式化器统一为 LF 换行；语法岛内部字节保持原样。缩进帮助阅读，但不决定语义；真正的块边界是 `begin` 与 `end`。

可以随时运行格式化器确认文件风格：

```powershell
./build/verba.exe fmt --check learn/01_hello
```

下一章：[第一个 Verba 程序](02-first-program.md)。
