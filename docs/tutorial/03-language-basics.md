# 3. 语言基础

本章示例位于 `learn/02_basics`。

## 不可变与可变绑定

`let` 创建不可重新绑定的名称：

```verba
let total to be call add price shipping
```

`var` 创建可变名称，之后必须使用 `set` 更新：

```verba
var attempts to be 0
set attempts to be call add attempts 1
```

Verba 不使用 `=`。`to be` 专门负责创建或更新绑定，`is` 与 `is not` 专门比较值。

## 字面量

布尔、整数和小数可以直接书写：

```verba
let enabled to be true
let retries to be 3
let ratio to be 0.75
```

一整行文本、URL 或路径使用类型引导词捕获：

```verba
let message to be text service is ready
let endpoint to be url https://api.example.com/v1/users
let config to be path C:\service\config.json
```

## 调用

简单调用把参数依次写在函数名后：

```verba
let normalized to be call lowercase value
let total to be call add subtotal tax
```

复杂调用使用命名参数块：

```verba
let result to be call send_message
begin
    with recipient address
    with body message
end
```

参数块避免括号和逗号，同时让参数含义在调用点可见。

## 条件

```verba
let positive to be call greater_than count 0

if positive
begin
    return text positive
end
else
begin
    return text zero_or_negative
end
```

条件必须是 `bool`。把整数、字符串或 record 直接作为条件会在 `verba check` 阶段报错。

## 循环

```verba
for item in items
begin
    call process item
end

while should_retry
begin
    call attempt
    set should_retry to be call retry_needed
end
```

`for` 只能遍历 `list T`，循环变量自动获得元素类型。`while` 条件必须是 `bool`。

验证本章示例：

```powershell
./build/verba.exe check learn/02_basics
./build/verba.exe build -o build/basics.exe learn/02_basics
```

下一章：[函数与类型](04-functions-and-types.md)。
