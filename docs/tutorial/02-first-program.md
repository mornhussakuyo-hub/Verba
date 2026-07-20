# 2. 第一个 Verba 程序

打开 `learn/01_hello/main.vrb`。最小程序从模块声明开始：

```verba
module hello

use http
use json
```

`module hello` 表示文件属于 `hello` 模块。一个项目可以有多个 `.vrb` 文件，但它们必须使用同一个模块名。

`use http` 声明程序需要 HTTP 能力，`use json` 允许后面的 JSON 响应。能力声明让代码审查者和工具链能够快速判断程序会接触哪些外部系统；使用能力却没有声明会产生 `VRB1710`。

## 声明路由

```verba
route hello_world
method get
path /
begin
    respond text 200 hello_world
end
```

逐行理解：

- `route hello_world` 声明名为 `hello_world` 的路由。
- `method get` 只接受 GET 请求。
- `path /` 匹配根路径。
- `begin` 和 `end` 包围路由体。
- `respond text 200 hello_world` 返回状态码 200 和文本正文。

换行终止普通语句。空行和缩进不参与语义，但官方格式化器会统一它们。

## 检查、构建与运行

```powershell
./build/verba.exe check learn/01_hello
./build/verba.exe build -o build/hello.exe learn/01_hello
./build/hello.exe
```

服务默认监听 `:8080`。另开终端访问：

```powershell
curl http://127.0.0.1:8080/
```

也可以让编译器临时构建并立即运行：

```powershell
./build/verba.exe run learn/01_hello
```

## 修改监听地址

```powershell
$env:VERBA_ADDRESS = "127.0.0.1:9090"
./build/hello.exe
```

下一章：[语言基础](03-language-basics.md)。
