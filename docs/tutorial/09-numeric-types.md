# 9. 数值类型与精确小数

Verba 不会在不同数值类型之间做隐式转换。数值字面量会先保持为常量，再根据函数参数、返回类型、赋值目标或另一个操作数确定具体类型。

## 整数宽度

`int` 和 `uint` 在所有目标平台上固定为 64 位。需要协议字段、数据库列或二进制格式的精确宽度时，可以使用 `int8`、`int16`、`int32`、`int64`、`uint8`、`uint16`、`uint32` 和 `uint64`。

```verba
function next_code
input current uint8
output uint8
begin
    return call add current 1
end
```

这里的 `1` 根据 `current` 和返回类型解析为 `uint8`。编译器会在生成 Go 代码前检查范围，因此 `return 256` 会得到 `VRB1460`，而不是留给后端产生难懂的溢出错误。

无上下文的整数字面量默认为 `int`：

```verba
let attempts to be 3
```

## 浮点数

`float32` 和 `float64` 使用 IEEE 754 浮点语义。带小数点或指数且没有其他上下文的字面量默认为 `float64`。

```verba
function ratio
output float32
begin
    return 1.25
end
```

编译器会拒绝超出目标浮点类型范围的常量。浮点数适合测量值和允许舍入误差的计算，不适合金额。

## Decimal

`decimal` 使用任意精度有理数保存十进制字面量，不经过二进制浮点数，因此常见金额计算是精确的：

```verba
function exact_total
output decimal
begin
    return call add 0.1 0.2
end
```

结果精确等于 `0.3`，不会变成 `0.30000000000000004`。`add`、`subtract`、`multiply`、`divide`、`remainder`、`negate` 和顺序比较都支持 `decimal`。

JSON 编解码同样保持精度。有限十进制会输出为 JSON number，并去掉无意义的末尾零。像 `1 / 3` 这样的结果没有有限十进制表示，编码为 JSON 时会明确失败，避免悄悄舍入。

## 不做隐式转换

下面的调用不会把 `int` 变量自动缩窄为 `int8`：

```verba
function consume
input value int8
begin
end

function invalid
begin
    let value to be 10
    call consume value
end
```

字面量可以在目标类型范围内按上下文定型，但已经绑定的值拥有确定类型。后续版本会提供显式 `cast`、`parse` 和可能失败的转换函数。

## 运行示例

`learn/06_numeric` 提供精确金额加法的 HTTP 示例：

```powershell
./build/verba.exe check learn/06_numeric
./build/verba.exe build -o build/numeric.exe learn/06_numeric
./build/numeric.exe
```

访问 `GET /total` 会得到：

```json
20
```

如果修改示例，把 `0.10` 传给整数参数，或把 `256` 传给 `uint8` 参数，`verba check` 会在源码位置直接报告类型或范围错误。
