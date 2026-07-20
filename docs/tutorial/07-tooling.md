# 7. 工具链与排错

Verba 把检查、格式化、生成和构建统一到一个命令中。

## Check

```powershell
verba check path/to/project
```

`check` 递归发现 `.vrb` 文件，然后执行解析、名称解析、类型检查和语法岛验证。目录中的所有文件会被视为同一个模块，因此每个独立项目应分别检查。

诊断包含稳定编号、文件、行列、问题说明和修复提示：

```text
main.vrb:8:1: error VRB1422: argument value requires string but received int
  hint: use a value with the expected type; Verba does not apply implicit conversions
```

## Fmt

```powershell
verba fmt path/to/project
verba fmt --check path/to/project
verba fmt --stdout path/to/main.vrb
```

- 默认模式就地格式化。
- `--check` 不修改文件，适合 CI。
- `--stdout` 只接受一个源文件。
- 语法岛原始内容默认保持不变。

格式化器是幂等的：连续执行两次不会产生第二次变化。

## Build

```powershell
verba build -o build/server.exe path/to/project
verba build --emit-go build/server.generated.go path/to/project
```

`build` 先完成所有前端检查，再生成格式化后的 Go 源码并调用 `go build`。`--emit-go` 便于学习生成结果和报告编译器问题。

## Run

```powershell
verba run path/to/project
verba run path/to/project -- argument1 argument2
```

`run` 在临时目录构建程序，前台运行，并把 `--` 后的值传给生成程序。

## 常见问题

### Duplicate declaration

如果一次检查多个独立教程目录，重复的函数或路由名会被合并到同一模块。请分别对每个项目运行 `check`。

### Port already in use

设置不同监听地址：

```powershell
$env:VERBA_ADDRESS = "127.0.0.1:9090"
verba run learn/04_http
```

### Go backend failed

先运行 `verba check`。如果合法程序仍产生无效 Go，请使用 `--emit-go` 保存生成源码，并在 issue 中附上 Verba 源码、诊断和版本号。

教程到这里已经覆盖当前稳定垂直切片。接下来可以阅读仓库根目录的 `design.md` 了解完整语言方向，或查看 `docs/implementation-status.md` 跟踪实现进度。
