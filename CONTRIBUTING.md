# Contributing

感谢参与 Verba。当前阶段优先验证语言核心结构、诊断质量与小而完整的后端用例。

提交修改前请运行：

```powershell
go test ./...
go vet ./...
go run ./cmd/verba fmt --check examples
go run ./cmd/verba check examples
go run ./cmd/verba build -o build/hello.exe examples/hello
```

实现新语法时，请同时更新：

1. `design.md` 中的语言设计或开放问题；
2. parser、checker 和 formatter；
3. Go emitter 或明确的“前端可检查、后端暂不生成”诊断；
4. 至少一个有效测试和一个错误诊断测试。

诊断编号应保持稳定，并提供可执行的修复提示。格式化器必须幂等，语法岛原始内容默认不得被改写。
