# 4. 函数与类型

本章示例位于 `learn/03_types`。

## Record

record 是具有固定字段的静态类型：

```verba
record profile
begin
    field display_name string
end
```

字段访问使用 `get`，不使用点号：

```verba
let name to be get user_value profile display_name
```

编译器会沿字段路径逐级解析类型。字段不存在，或在非 record 值上使用 `get`，都会产生诊断。

## Enum

```verba
enum role
begin
    case admin
    case member
end
```

枚举值是静态类型值，而不是任意字符串。不同 enum 之间不能隐式转换。

枚举很适合配合 `match`：

```verba
match value
begin
    case admin
    begin
        return text administrator
    end
    case member
    begin
        return text member
    end
    else
    begin
        return text unknown
    end
end
```

case 类型必须与 match 值一致，重复 case 会在编译期报错。需要从 match 返回值时，每个 case 和 else 都必须终止。

## 函数

```verba
function normalize_name
input value string
output string
begin
    let trimmed to be call trim value
    return call lowercase trimmed
end
```

函数可以有多个 `input`，但只有一个 `output`。声明输出后，每条控制流路径都必须返回兼容值。

## Optional

可能缺失的值必须显式写成 `optional T`：

```verba
field nickname optional string
```

访问 optional 值时先检查，再解包：

```verba
let nickname to be get user_value nickname
let present to be call is_some nickname

if present
begin
    return call unwrap nickname
end
```

直接穿过 optional 值读取字段会收到 `VRB1440`，避免隐式空值错误。

## Result 与 Try

可能失败的函数返回 `result T E`：

```verba
function load_name
output result string app_error
begin
    return call error unavailable
end
```

`try call` 在成功时得到 `T`，失败时把同一个错误类型 `E` 返回给外层函数。错误类型不一致会收到 `VRB1437`，必须显式转换或处理。

验证本章示例：

```powershell
./build/verba.exe check learn/03_types
```

下一章：[HTTP 服务](05-http-service.md)。
