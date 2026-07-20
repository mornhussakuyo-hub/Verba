# Verba

## Verba 语言设计文档

> 一种核心结构无标点、通过语法岛接纳 JSON / SQL / HTML / Regex 的静态类型后端语言

> **核心命题**
>
> 符号可以存在于数据、字面量和嵌入语言中，但不应承担核心程序结构。程序逻辑由单词、换行以及 begin / end 组织；外部协议保持原样，并通过明确边界进入语言。

版本 0.1  ·  状态 Draft  ·  2026-07

# 摘要

Verba 是一门面向 Web 后端与服务端程序的静态类型语言。名称由 verb 自然衍生而来，短、易输入，也对应程序由动作词和结构词组织的设计。Verba 不追求让源文件中的每一个字节都没有符号，而是重新规定符号的归属：控制流、声明、作用域、函数调用和类型表达式尽量只由单词构成；数字、URL、路径等符号被约束在受控字面量中；JSON、SQL、HTML、正则表达式等外部语法则进入显式的“语法岛”。本文将这套无标点的核心结构层称为 Verb Core，中文简称“词核”。

这一设计试图解决的并不是“键盘上不要出现标点”这一表层问题，而是降低程序逻辑中的视觉噪音、结构错配和符号负担。阅读者在浏览核心逻辑时，应主要看到有意义的词语；一旦出现大量标点，便可以立刻判断自己已经进入数据、协议或专用语言区域。

> **一句话定义**
>
> Verba 是一门“词语负责结构、符号留在边界”的后端语言。

## 设计结论概览

- **核心结构：**使用关键字、标识符、数字、下划线、空白和换行；代码块使用 begin / end。
- **符号政策：**不做全局禁令。符号允许出现在数值、URL、路径、字符串以及语法岛内部，但不作为核心运算符、分隔符或作用域标记。
- **表达式政策：**优先采用单词运算和一行一个计算步骤；MVP 不鼓励深层嵌套表达式。
- **类型系统：**静态类型、无隐式 null、无隐式数值转换，以 optional 与 result 表达缺失和错误。
- **后端集成：**HTTP、JSON、SQL 是一等能力；JSON 与 SQL 不被重新发明，而是通过适配器进入编译器。
- **实现路线：**首个编译器使用 Go 编写并生成 Go 代码，再调用 gofmt 与 go build。

## 目录

| **章节** | **内容**                       |
| -------------- | ------------------------------------ |
| 1–3           | 愿景、目标、符号边界模型             |
| 4–7           | 源文件、核心语法、类型与控制流       |
| 8–10          | 语法岛、HTTP / JSON / SQL 后端模型   |
| 11–13         | 安全、诊断、编译器与工具链           |
| 14–16         | MVP 范围、路线图、完整示例与开放问题 |

# 1 语言愿景

大多数现代语言把大量结构信息编码进标点：括号表示调用，花括号表示作用域，逗号分隔参数，点号访问成员，等号赋值，箭头表示返回类型，尖括号表示泛型。它们紧凑而成熟，但也造成了另一种阅读成本：结构被压缩成密集符号，错误常常表现为缺失、错配或嵌套层级混乱。

Verba 的目标不是证明符号“坏”，而是把结构重新展开成可读单词。begin 与 end 明确表达作用域；call 表达调用；let 与 to be 表达绑定；is 表达相等判断；with 表达命名参数；result、optional、list 等词语直接表达类型构造。符号仍被保留，但只在它们真正属于的领域中出现。

> **视觉契约**
>
> 核心逻辑应当看起来像结构化技术语言，而不是自然语言；它可以朗读，但必须保持严格、可静态分析、无上下文猜测。

## 1.1 预期使用场景

- REST / RPC 服务、后台管理系统、微服务与内部 API。
- 数据库驱动的 CRUD、任务处理、消息消费者和定时作业。
- 教学、语言设计实验、语音输入和大模型生成代码。
- 强调可读性、可审计性和稳定部署的中小型服务。

## 1.2 非目标

- 不是自然语言编程：编译器不会猜测语义，也不会接受模糊句子。
- 不是字符级“纯字母挑战”：URL、JSON、SQL、正则等无需被翻译成单词。
- 不是以最少字符为目标的竞赛语言，也不追求取代 C/C++ 的底层控制。
- MVP 不提供宏、运算符重载、复杂泛型、高级元编程和任意语法扩展。
- 不重新设计 JSON、HTTP、SQL 等成熟协议。

## 1.3 核心设计原则

| **编号** | **原则**     | **含义**                                               |
| -------------- | ------------------ | ------------------------------------------------------------ |
| P1             | 词语负责结构       | 声明、调用、条件、作用域与类型构造由关键字表达。             |
| P2             | 符号具有归属       | 符号只出现在字面量、数据或专用语法中。                       |
| P3             | 边界必须显式       | 进入原始文本或外部语言时，源码必须留下清楚的开始与结束标记。 |
| P4             | 词法隔离，语义集成 | 语法岛独立解析，但可以参与类型检查、绑定检查和代码生成。     |
| P5             | 一行一个意图       | 优先拆分复杂表达式，减少嵌套与优先级规则。                   |
| P6             | 安全默认           | SQL 绑定、HTML 转义、错误传播和资源管理采用安全行为。        |
| P7             | 工具先行           | 格式化、诊断、语言服务器和可重复构建属于语言本体。           |

# 2 符号边界模型

Verba 不采用“允许字符白名单”作为唯一规则，而是把源码划分为三个层级。这个模型决定了词法分析器何时按核心语言解析，何时把内容交给专用解析器。

## 2.1 第一级：核心结构区

核心结构区承担模块、声明、类型、控制流、调用和资源生命周期。结构标记只使用关键字、标识符、空白与换行。下划线与数字是标识符和字面量的一部分。

```verba
module user_service
use http
use json

function health
output response
begin
    respond text 200 service healthy
end
```

这里没有括号、花括号、逗号、分号、点号或符号运算符。换行终止普通语句，begin / end 终止块。缩进只用于阅读，编译器不依赖缩进判断作用域。

## 2.2 第二级：受控字面量区

数值、URL、文件路径、时间、路由路径和单行文本可以包含符号，但它们必须由类型上下文或引导关键字捕获，并且边界明确。符号属于该字面量，不参与核心语法。

```verba
let retry_count to be 3
let threshold to be 0.875
let temperature to be -20.5
let endpoint to be url https://api.example.com/v1/users?page=1
let config_path to be path C:\service\config.json
path /users/{user_id}
let message to be text user was not found
```

例如 url、path、text 和路由声明中的 path 会捕获该行剩余内容；数值解析器只处理完整的数值 token。核心解析器不把其中的斜杠、冒号、问号或句点视为结构符号。

## 2.3 第三级：语法岛区

当内容需要跨行，或本身是一门完整语言时，使用 embed 声明创建语法岛。岛内允许任意字符，由对应适配器解析。

```verba
embed json default_config until end_default_config
{
  "host": "0.0.0.0",
  "port": 8080,
  "debug": false
}
end_default_config
```

终止标识符必须是合法核心标识符、单独占一行，并在当前文件中与该岛配对。岛内即使出现同名文本，只要没有形成完全相同的独立终止行，就不会结束语法岛。

> **关键区别**
>
> Verba 不是“没有符号”，而是“结构无标点”。这使语言能够尊重现有协议，而不让协议的标点扩散到核心逻辑。

# 3 源文件与词法规则

## 3.1 文件与编码

- 源文件暂定扩展名为 .vrb；UTF-8 编码，不带 BOM。
- 一份文件声明一个 module；模块可以由多个文件组成。
- 换行是普通语句的结束边界；空行没有语义。
- 缩进建议使用 4 个空格，由格式化器统一，但缩进不参与语法。
- 核心关键字使用小写；普通标识符推荐 snake_case。

## 3.2 标识符

标识符以字母或下划线开头，后续可以包含字母、数字和下划线。MVP 区分大小写，但格式化器与官方风格只推荐 ASCII 小写 snake_case；非 ASCII 标识符暂不进入 MVP，以减少工具链和同形字符风险。

| **示例** | **结果**                       |
| -------------- | ------------------------------------ |
| user           | 合法                                 |
| user_id_2      | 合法                                 |
| _internal      | 合法，但保留给内部或生成代码         |
| 2user          | 非法：不能以数字开头                 |
| user-name      | 非法核心标识符；连字符不是标识符字符 |

## 3.3 注释

为了不引入 // 或 #，单行注释使用 note。note 之后的整行被当作原始文本，不再参与核心解析；因此注释中可以自然出现中文和任意符号。多行说明使用 text 语法岛或专门的 comment 岛。

```verba
note This handler is intentionally idempotent.
note 这里可以写中文，也可以写 URL: https://example.com

embed comment migration_note until end_migration_note
This block may contain examples, punctuation, and long explanations.
end_migration_note
```

## 3.4 保留字策略

保留字按能力分组，避免一次性占用大量常见单词。MVP 核心保留字如下：

| **类别** | **关键字**                                                  |
| -------------- | ----------------------------------------------------------------- |
| 结构           | module use begin end                                              |
| 声明           | record enum function route field input output                     |
| 绑定与关系     | let var set to be is not                                          |
| 调用           | call with try                                                     |
| 控制流         | if else match case while for in return                            |
| 类型           | bool int uint float decimal string bytes optional list map result |
| 外部内容       | embed until text url path note                                    |
| 后端           | method respond bind                                               |

# 4 核心语法

## 4.1 模块与依赖

```verba
module account_service

use http
use json
use sql postgres
use time
```

use 引入语言能力或包。use sql postgres 表示启用 SQL 能力并选择 postgres 方言。依赖解析、版本锁定和包地址属于项目清单，而不是在每个源文件中使用带符号的 URL。

## 4.2 记录与枚举

```verba
record user
begin
    field id uuid
    field name string
    field email string
    field roles list role
    field nickname optional string
end

enum role
begin
    case admin
    case member
    case guest
end
```

字段必须显式使用 field。类型构造采用前缀词语：list role、optional string、map string user。这样不需要尖括号、方括号或问号。

## 4.3 函数

```verba
function normalize_email
input value string
output string
begin
    let lowered to be call lowercase value
    let trimmed to be call trim lowered
    return trimmed
end
```

参数和返回值各占一行。MVP 允许多个 input，但只允许一个 output 类型；多值返回通过 record 或 result 表达。函数体必须使用 begin / end。

## 4.4 变量与赋值

```verba
let user_id to be call new_uuid
var retry_count to be 0
set retry_count to be call add retry_count 1
```

- **let ... to be ...：** 创建不可重新绑定的名称；默认选择。
- **var ... to be ...：** 创建允许后续 set 的可变变量；必须显式声明。
- **set ... to be ...：** 更新已有可变变量或可变字段。to be 明确承担传统赋值号 = 的职责。

to be 是由两个连续关键字组成的绑定标记，只能用于 let、var 和 set。它不产生布尔值，也不能作为函数调用。

## 4.5 字段访问

成员访问不使用点号。get 读取字段路径，set 可以写入可变值。路径由连续标识符组成；编译器依据静态类型逐级解析。

```verba
let email to be get request_user profile email
set mutable_user profile display_name to be new_name
```

get user profile email 等价于传统语法中的 user.profile.email。由于路径中只能出现字段名，不会和函数参数发生歧义。

# 5 表达式与控制流

## 5.1 一行一个计算步骤

Verba 刻意不鼓励深层嵌套。MVP 中，普通 call 的参数只能是名称或字面量；一个 call 不能直接嵌套另一个 call。需要组合时，先绑定中间结果。

```verba
let subtotal to be call multiply unit_price quantity
let total to be call add subtotal shipping_fee
let is_large to be call greater_than total 1000
```

这种约束牺牲少量紧凑性，换取稳定解析、明确诊断、语音输入友好和更容易审查的差异。后续版本可以加入受限的固定元数前缀表达式，但不应破坏“一行一个意图”的默认风格。

## 5.2 运算符是普通单词

| **类别** | **内建函数 / 谓词**                             |
| -------------- | ----------------------------------------------------- |
| 算术           | add subtract multiply divide remainder negate         |
| 相等关系       | is  is not                                            |
| 顺序比较       | greater_than less_than greater_equal less_equal       |
| 逻辑           | and or not                                            |
| 位运算         | bit_and bit_or bit_xor bit_not shift_left shift_right |
| 文本           | concat trim lowercase uppercase contains starts_with  |

除 is 与 is not 外，它们在语义上可以作为编译器内建，也可以在标准库中表现为普通函数。is 与 is not 是不可重载的核心关系表达式，分别对应传统语法中的 == 与 !=。它们表示值相等，不表示对象身份、类型判断或模式匹配。两侧在 MVP 中必须是名称或字面量，类型必须相同且可比较；不进行隐式类型转换，结果类型为 bool。基础标量、string、bytes、uuid、time、duration、url、path 和 enum 可比较；optional T 仅在 T 可比较时可比较。record、list 和 map 的结构相等暂不进入 MVP。

```verba
let unchanged to be current_value is previous_value
let changed to be current_value is not previous_value
```

因此，to be 与 is 在视觉和语法上具有固定分工：to be 创建或更新绑定，is 只比较值。

## 5.3 条件与循环

```verba
let adult to be call greater_equal age 18
let verified to be get user verified
let allowed to be call and adult verified

if allowed
begin
    return access_granted
end
else
begin
    return access_denied
end
```

简单的相等条件可以直接使用关系表达式：

```verba
if requested_role is admin
begin
    return access_granted
end
```

```verba
for item in items
begin
    call process_item item
end

while should_retry
begin
    call attempt_request
    set should_retry to be call retry_needed
end
```

## 5.4 复杂调用的参数块

固定元数、简单位置参数使用单行 call；可选参数、命名参数、回调或适配器调用使用参数块。

```verba
let message_id to be try call send_email
begin
    with recipient user_email
    with subject subject_text
    with body body_text
    with retry_count 3
end
```

参数块由 begin / end 包围，with 显式给出参数名。它不会与函数体混淆，因为解析器已经处于 call 表达式上下文。

# 6 类型系统与错误模型

## 6.1 基础类型

| **类别** | **类型**                                                  |
| -------------- | --------------------------------------------------------------- |
| 布尔与整数     | bool int uint int8 int16 int32 int64 uint8 uint16 uint32 uint64 |
| 数值           | float32 float64 decimal                                         |
| 文本与二进制   | string bytes                                                    |
| 后端常用       | uuid time duration url path                                     |
| 结构           | record enum                                                     |
| 构造类型       | optional T  list T  map K V  result T E                         |

int 与 uint 固定为 64 位，不随编译器宿主或目标架构变化。decimal 用于金额等需要精确十进制语义的场景；字面量和运算使用任意精度有理数，不经过二进制浮点。有限十进制可以无损编解码为 JSON number；除法产生的非终止十进制必须显式处理，不能静默舍入。float 不应用于货币。uuid、time、duration、url、path 是强类型值，而不是字符串别名。

## 6.2 无隐式 null

任何可能缺失的值必须写成 optional T。普通 T 永远不为 null。optional 值通过 match、if is_some 或 try_optional 等标准操作处理。

```verba
function display_name
input user_value user
output string
begin
    let nickname to be get user_value nickname

    if call is_some nickname
    begin
        return call unwrap nickname
    end

    return get user_value name
end
```

## 6.3 result 与 try

可失败函数返回 result T E。try 是显式错误传播：成功时解包 T，失败时从当前函数返回同类型错误。它不是异常，也不会跨越类型边界。

```verba
function load_user
input user_id uuid
output result user app_error
begin
    let row to be try call repository_find_user user_id
    let user_value to be try call map_user_row row
    return call ok user_value
end
```

路由函数可以由编译器根据 app_error 的映射生成 HTTP 响应；未处理错误必须在编译期被发现。

## 6.4 类型转换

不同数值宽度、string 与 uuid、string 与 url 之间均不做隐式转换。转换通过 parse、cast 或专用构造函数完成，并在可能失败时返回 result。

# 7 语法岛系统

## 7.1 通用声明形式

```verba
embed island_kind resource_name until terminator_name
raw content belongs to the selected island adapter
terminator_name
```

island_kind 选择适配器，resource_name 把岛内容绑定成模块级不可变资源，terminator_name 决定原始内容边界。原始内容不经过核心 lexer，而是以字节与源位置交给适配器。

## 7.2 词法状态机

1. 核心 lexer 读取 embed、岛类型、资源名、until 和终止标识符。
2. 读取完声明行后切换到 RAW_ISLAND 状态。
3. 逐行读取原始内容，不解释其中任何核心关键字或符号。
4. 遇到完全等于终止标识符的独立行时退出岛状态。
5. 把原始字节、起止位置、岛类型与资源名交给适配器注册表。

## 7.3 适配器职责

| **能力** | **说明**                             |
| -------------- | ------------------------------------------ |
| parse          | 解析或验证岛内语法，产生带源位置的诊断。   |
| bindings       | 提取需要由核心代码绑定的参数或槽位。       |
| type_info      | 给出输入、输出、字段或资源类型信息。       |
| emit           | 生成目标代码、静态资源或运行时调用。       |
| format         | 可选：在不破坏原语义的前提下格式化岛内容。 |

## 7.4 绑定必须在岛外完成

语法岛默认不能隐式读取核心变量。核心代码通过 bind / with 显式传值，避免字符串拼接和注入风险。

```verba
embed sql find_user until end_find_user
SELECT id, name, email
FROM users
WHERE id = :user_id;
end_find_user

let user_row to be try call sql_one find_user
begin
    with user_id requested_id
end
```

SQL 适配器提取 :user_id，并检查参数块中是否恰好存在同名绑定、类型是否兼容以及是否有多余绑定。

## 7.5 首批官方岛类型

| **岛类型** | **编译期行为**           | **运行时结果**       |
| ---------------- | ------------------------------ | -------------------------- |
| json             | 完整语法验证，可推断常量结构   | 不可变 JSON 常量或编码资源 |
| sql              | 按方言解析、提取参数与结果列   | 预编译查询描述             |
| html             | 解析模板、提取槽位、检查转义   | 模板资源                   |
| regex            | 编译正则、检查方言             | 预编译模式                 |
| text             | 原样保存，可选模板槽位         | 字符串或字节资源           |
| shell            | 默认仅语法保存，需显式能力授权 | 受限制的外部进程描述       |

# 8 后端一等模型

## 8.1 路由声明

```verba
route get_user
method get
path /users/{user_id}
output result user app_error
begin
    let user_id to be try call path_uuid user_id
    let user_value to be try call service_find_user user_id
    respond json 200 user_value
end
```

method 和 path 是路由元数据。path 行的剩余部分由 HTTP 路径解析器处理，因此斜杠与花括号不会进入核心语法。路径参数被注入为同名的原始字符串绑定；开发者必须显式解析为 uuid、int 等目标类型。

## 8.2 请求输入

| **来源** | **标准绑定**                          |
| -------------- | ------------------------------------------- |
| 路径参数       | 与 path 模板中的名称相同，初始类型为 string |
| 查询参数       | query_values，或通过 query T 解码           |
| 请求头         | request_headers                             |
| 请求体         | request_body，类型 bytes 或 stream          |
| 上下文         | request_context，含取消、截止时间、追踪信息 |

## 8.3 响应

```verba
respond json 201 created_user
respond text 404 user not found
respond empty 204
```

respond 是终止语句。响应格式、状态码和主体类型在编译期检查。框架可以为 record 自动生成 JSON 编码器；字符串不会被误当作 JSON。

带有 `output result T E` 的路由同时也是类型化 HTTP 错误边界。`return call ok value` 默认生成 `200` JSON 响应，显式 `respond` 可以选择其他成功状态；`return call error value` 和 `try` 传播的错误由编译器统一映射。MVP 采用按错误 case 名称确定的稳定约定：`invalid_request` 与 `invalid_email` 映射为 `400`，`user_not_found` 映射为 `404`，`database_failure` 以及未列出的 case 映射为 `500`。`json_decode` 与 `parse_uuid` 的字符串错误只有在目标错误枚举声明 `invalid_request` 时才能自动转换，否则类型检查失败。后续版本可以增加显式状态元数据，但不得改变这套 MVP 默认值。

## 8.4 JSON 的日常使用不需要原始岛

JSON 岛适合常量、示例、schema 或外部配置。普通请求体应直接解码为静态 record，普通响应应直接编码 record。

```verba
record create_user_request
begin
    field name string
    field email string
end

let payload to be try call json_decode create_user_request request_body
let created to be try call user_service_create payload
respond json 201 created
```

这样既尊重 JSON 协议，又避免业务代码退化为动态字典访问。字段映射、必填性、未知字段策略和命名规则由 JSON 派生配置控制。

# 9 SQL 集成设计

## 9.1 SQL 保持 SQL

```verba
embed sql insert_user until end_insert_user
INSERT INTO users (id, name, email)
VALUES (:id, :name, :email)
RETURNING id, name, email, created_at;
end_insert_user
```

Verba 不发明“纯单词 SQL”。SQL 语法由数据库生态决定；语言只负责边界、绑定、类型与执行生命周期。

## 9.2 查询执行接口

| **接口**  | **语义**                       |
| --------------- | ------------------------------------ |
| sql_exec        | 执行不返回行的语句，返回受影响行数。 |
| sql_one         | 要求恰好一行，返回 result Row E。    |
| sql_optional    | 允许零或一行，返回 optional Row。    |
| sql_many        | 返回 list Row 或流。                 |
| sql_transaction | 执行显式事务块。                     |

## 9.3 事务

```verba
transaction database
begin
    let order_row to be try call sql_one insert_order
    begin
        with order_id order_id
        with user_id user_id
    end

    try call sql_exec decrease_stock
    begin
        with product_id product_id
        with quantity quantity
    end
end
```

事务块内的 try 失败会触发回滚；正常离开块时提交。事务对象由编译器隐式传入该块中的 SQL 操作，但不会泄露到块外。

## 9.4 SQL 安全规则

- 禁止把普通 string 直接拼接进 SQL 语法岛。
- 所有动态值必须通过参数绑定；标识符动态化需要受限枚举或适配器专用 API。
- 编译器检查缺失绑定、多余绑定、重复绑定和基础类型不兼容。
- 生产构建可要求 schema 快照，以检查表、列和返回类型。
- 日志默认不打印敏感绑定值。

# 10 安全与能力边界

## 10.1 语法岛不是逃生舱

如果任何原始块都可以任意访问变量并执行系统命令，语言的安全性会迅速退化。因此岛类型必须由适配器注册，适配器声明自己的能力、输入、输出和代码生成方式。未知岛类型是编译错误。

## 10.2 显式能力

```verba
module report_service

use http
use sql postgres
use capability network_client
use capability file_read
```

网络客户端、文件写入、子进程、环境变量读取等高风险能力可以在模块或项目清单中显式声明。工具链因此能够进行静态审计、生成最小容器权限并在测试中替换能力实现。

## 10.3 HTML 与模板

HTML 岛默认将绑定值进行上下文相关转义。只有显式的 trusted_html 类型可以跳过转义，而该类型不能由普通 string 隐式构造。

```verba
embed html profile_page until end_profile_page
<html>
  <body>
    <h1>{{ title }}</h1>
    <p>{{ biography }}</p>
  </body>
</html>
end_profile_page

let page to be call render profile_page
begin
    with title page_title
    with biography user_biography
end
```

双花括号属于 HTML 模板岛，不属于核心语法。适配器提取 title 与 biography 槽位，并检查核心绑定。

## 10.4 Shell 岛

shell 岛不进入 MVP 默认能力。启用时必须显式声明 process 能力，参数通过结构化 argv 绑定，不允许把未验证字符串拼成单个 shell 命令。官方建议优先提供进程 API 而不是 shell 文本。

# 11 诊断与开发体验

## 11.1 错误信息原则

- 指出源文件、行列、相关声明和建议修复。
- 核心错误与岛内错误统一呈现，但标明产生诊断的适配器。
- 尽量使用语言中的词语描述，而不是暴露编译器内部 AST 名称。
- 一个编译周期尽可能报告多个独立错误。

## 11.2 示例诊断

```text
error VRB1204 at user_service.vrb line 42 column 8

    let valid to be email is
                   ^^^^^^^^^^^^

is requires a right operand but none was provided

suggestion
    let valid to be email is expected_email
```

```text
error SQL2107 in island insert_user

parameter :email is declared by SQL but not bound by the call at line 67

available bindings
    id
    name
```

```text
error VRB0311 at user_service.vrb line 89

expected end for route create_user before end of file
```

## 11.3 格式化器

官方格式化器 verba fmt 具有唯一风格：关键字小写、4 空格缩进、声明之间固定空行、参数块逐行排列。格式化必须幂等。默认情况下不格式化语法岛；适配器可以选择提供独立格式化能力。

## 11.4 语言服务器

- 跳转到定义、查找引用、重命名与文档悬停。
- 核心语法和各语法岛的嵌套高亮。
- SQL 参数、HTML 槽位与核心绑定之间的跨区域跳转。
- 路由列表、数据库查询列表和能力审计视图。
- 生成代码可查看但默认只读。

# 12 编译器架构

## 12.1 首版技术路线

首版编译器使用 Go 编写，并生成可读的 Go 源码。生成阶段完成后调用 gofmt 和 go build。这样可以复用 Go 的垃圾回收、网络栈、并发运行时、跨平台构建和部署生态，同时让语言团队把精力集中在语法、类型检查和语法岛集成。

## 12.2 编译流水线

1. **Source Manager：**读取 UTF-8、维护文件与位置映射。
2. **Region Scanner：**识别核心区、受控字面量和语法岛边界。
3. **Core Lexer：**生成关键字、标识符、数值和换行 token。
4. **Parser：**手写递归下降解析器，构建 AST。
5. **Name Resolver：**处理模块、作用域、字段路径和前向引用。
6. **Type Checker：**检查调用、控制流、optional / result 与响应类型。
7. **Island Registry：**调用 JSON / SQL / HTML 等适配器并合并诊断。
8. **Typed IR：**把核心 AST 与岛资源降低为稳定的中间表示。
9. **Go Emitter：**生成 Go 包、运行时桥接、路由和资源。
10. **Build Driver：**运行 gofmt、go test、go build，并重映射错误位置。

## 12.3 推荐目录结构

```text
cmd/verba
internal/source
internal/region
internal/lexer
internal/parser
internal/ast
internal/resolve
internal/types
internal/check
internal/island
internal/island/json
internal/island/sql
internal/island/html
internal/ir
internal/emitgo
internal/diagnostic
runtime/http
runtime/sql
runtime/json
```

## 12.4 适配器接口草案

```go
type Adapter interface {
    Kind() string
    Parse(ctx Context, raw []byte) (Artifact, []Diagnostic)
    Bindings(artifact Artifact) []Binding
    TypeInfo(artifact Artifact) TypeInfo
    Emit(ctx EmitContext, artifact Artifact) (Generated, []Diagnostic)
}
```

这是编译器实现接口，不是 Verba 源语法。适配器必须是确定性的；同一输入、编译器版本和配置应产生相同结果。

# 13 语法草案

以下 EBNF 只用于规范描述。MVP 语法保持行导向：普通语句由换行终止，块由 begin / end 终止，原始内容由终止行终止。

```ebnf
program          = module_decl, { top_level_decl } ;
module_decl      = "module", identifier, newline ;
top_level_decl   = use_decl | record_decl | enum_decl |
                   function_decl | route_decl | embed_decl ;

block            = "begin", newline, { statement }, "end", newline ;
record_decl      = "record", identifier, newline, record_block ;
field_decl       = "field", identifier, type_expr, newline ;
function_decl    = "function", identifier, newline,
                   { input_decl }, output_decl, block ;

statement        = let_stmt | var_stmt | set_stmt | call_stmt |
                   if_stmt | for_stmt | while_stmt | return_stmt |
                   respond_stmt | transaction_stmt ;

let_stmt         = "let", identifier, "to", "be", expression, newline ;
var_stmt         = "var", identifier, "to", "be", expression, newline ;
set_stmt         = "set", field_path, "to", "be", expression, newline ;

expression       = relation_expr | try_call_expr | call_expr |
                   get_expr | atom | controlled_literal ;
relation_expr    = atom, "is", [ "not" ], atom ;
try_call_expr    = "try", call_expr ;
call_expr        = "call", identifier, { atom } |
                   "call", identifier, newline, argument_block ;
argument         = "with", identifier, expression, newline ;

type_expr        = identifier |
                   "optional", type_expr |
                   "list", type_expr |
                   "map", type_expr, type_expr |
                   "result", type_expr, type_expr ;

embed_decl       = "embed", identifier, identifier,
                   "until", identifier, newline,
                   raw_lines, terminator_line ;
```

## 13.1 需要避免的语法歧义

| **问题**     | **MVP 决策**                                                       |
| ------------------ | ------------------------------------------------------------------------ |
| 函数参数何时结束   | 普通 call 以换行结束；参数只能是 atom。复杂参数使用 begin / end 参数块。 |
| 嵌套调用如何解析   | MVP 禁止普通 call 直接嵌套，要求中间 let。                               |
| 绑定与相等如何区分 | let / var / set 必须包含 to be；值相等使用 is，不相等使用 is not。       |
| 文本何时结束       | text / url / path 等上下文捕获行剩余部分。                               |
| 语法岛何时结束     | 自定义终止标识符必须独占一行。                                           |
| 成员路径何时结束   | get / set 根据静态类型逐字段消费名称，且语句由换行结束。                 |

# 14 MVP 范围与验收标准

## 14.1 MVP 必须支持

- module、use、record、enum、function、route。
- let、var、set、call、if / else、for、return、respond。
- 基础类型、optional、list、result。
- get 字段访问与静态类型检查。
- 受控 text / url / path 字面量。
- 通用 embed 机制，以及 json、sql postgres 两个适配器。
- HTTP 路由、JSON record 编解码、参数化 SQL 查询。
- Go 代码生成、格式化、构建和基础测试。
- verba check、verba fmt、verba build、verba run 四个命令。

## 14.2 MVP 暂缓

- 用户自定义泛型、trait / interface、继承。
- 宏、编译期执行、运算符重载。
- 任意表达式嵌套和用户自定义语法。
- 原生机器码后端、手动内存管理。
- 分布式包仓库与复杂版本求解。
- 完整异步语法、actor 和高级并发抽象。
- 无约束 shell 岛。

## 14.3 可验证验收标准

1. 示例用户服务可以编译为 Go，并通过 go build。
2. JSON 岛中的非法 JSON 在编译期报告精确位置。
3. SQL 中缺失或多余绑定会阻止构建。
4. 对 optional 值未解包直接使用会阻止构建。
5. 所有 begin / end 错配都能给出所属声明。
6. verba fmt 连续执行两次不产生变化。
7. 生成的路由能正确处理成功、解析失败、未找到和数据库错误。
8. 编译器测试覆盖 lexer、parser、类型检查、岛边界和代码生成。

# 15 完整示例

下面的示例展示一个最小用户创建与查询服务。它不是最终标准库 API，但体现了语言的预期视觉与边界。

```verba
module user_service

use http
use json
use sql postgres
use uuid
use time

record create_user_request
begin
    field name string
    field email string
end

record user
begin
    field id uuid
    field name string
    field email string
    field created_at time
end

enum app_error
begin
    case invalid_request
    case invalid_email
    case user_not_found
    case database_failure
end

embed regex email_pattern until end_email_pattern
^[A-Za-z0-9._%+-]+@[A-Za-z0-9.-]+\.[A-Za-z]{2,}$
end_email_pattern

embed sql insert_user until end_insert_user
INSERT INTO users (id, name, email)
VALUES (:id, :name, :email)
RETURNING id, name, email, created_at;
end_insert_user

embed sql find_user until end_find_user
SELECT id, name, email, created_at
FROM users
WHERE id = :id;
end_find_user

function create_user
input payload create_user_request
output result user app_error
begin
    let name to be get payload name
    let email to be get payload email
    let email_valid to be call regex_match email_pattern email
    let email_invalid to be call not email_valid

    if email_invalid
    begin
        return call error invalid_email
    end

    let user_id to be call new_uuid

    let row to be try call sql_one insert_user
    begin
        with id user_id
        with name name
        with email email
    end

    return call ok row
end

route create_user_route
method post
path /users
output result user app_error
begin
    let payload to be try call json_decode create_user_request request_body
    let created to be try call create_user payload
    respond json 201 created
end

route get_user_route
method get
path /users/{id}
output result user app_error
begin
    let user_id to be try call parse_uuid id

    let found to be try call sql_optional find_user
    begin
        with id user_id
    end

    let missing to be call is_none found

    if missing
    begin
        return call error user_not_found
    end

    let user_value to be call unwrap found
    respond json 200 user_value
end
```

注意：示例中 regex 与 SQL 保留各自原生符号；HTTP path 保留斜杠和花括号；核心声明、调用、控制流、字段访问和错误传播没有依赖标点结构。

# 16 设计决策与开放问题

## 16.1 已确认决策

| **决策** | **结论** | **理由**                                   |
| -------------- | -------------- | ------------------------------------------------ |
| 全局禁止符号   | 拒绝           | 会迫使 JSON、URL、SQL 等协议变形，损害生态兼容。 |
| 核心结构无标点 | 采用           | 保留设计特色，同时允许协议和数据原样存在。       |
| 代码块         | begin / end    | 明确、可朗读、不依赖缩进。                       |
| 语句结束       | 换行           | 避免分号，并支持行导向解析。                     |
| 复杂调用       | 参数块         | 避免括号、逗号与参数边界歧义。                   |
| 外部语法       | 通用 embed 岛  | 边界统一，可插拔验证与代码生成。                 |
| 首版后端       | 生成 Go        | 快速获得成熟运行时和部署能力。                   |
| 错误机制       | result + try   | 可静态检查，无隐式异常。                         |

## 16.2 仍需原型验证

- **字段访问：**get payload email 是否足够自然，还是需要 field payload email。
- **单行文本：**text 捕获行剩余部分是否会影响自动补全和代码重排。
- **数值符号：**负号和小数点应视为普通数值 token，还是要求类型引导。
- **参数块：**let result to be call f 后紧接 begin 的语法在大型代码中是否足够清晰。
- **SQL 类型检查：**MVP 使用 schema 快照、数据库在线检查，还是仅检查参数名称。
- **包系统：**项目清单格式与依赖地址是否使用 TOML，或由 Verba 自己提供配置语法。

## 16.3 推荐的下一步

1. 用 300–500 行 Go 写出 region scanner，首先验证语法岛边界。
2. 实现只支持 module、function、let、call、begin、end 的最小 parser。
3. 选取 3 个真实后端函数，分别用 Go、Verilog 风格伪代码和 Verba 编写，对比可读性。
4. 实现 JSON 岛验证和一个最小 HTTP health 路由代码生成。
5. 再加入 record、JSON 解码和 SQL 参数绑定，形成第一个可运行服务。
6. 在原型稳定前，不急于设计高级类型、并发和包仓库。

> **最终判断**
>
> 这个方向已经具备独立语言的核心创新点：不是把所有符号机械替换成英文，而是建立“核心结构—受控字面量—语法岛”的分层语法体系。真正的价值在边界设计、静态集成和后端安全，而不是表面的无符号。

# 附录 A 语言风格速查

| **传统写法概念**  | **Verba 倾向写法**           |
| ----------------------- | ---------------------------------- |
| 函数调用 f(a, b)        | call f a b                         |
| 复杂命名参数            | call f + begin / with / end 参数块 |
| 作用域 { ... }          | begin ... end                      |
| 不可变绑定 x = value    | let x to be value                  |
| 可变赋值 x = value      | set x to be value                  |
| 相等判断 x == value     | x is value                         |
| 不等判断 x != value     | x is not value                     |
| 成员 user.profile.email | get user profile email             |
| 可空 User?              | optional user                      |
| Result<User, Error>     | result user app_error              |
| a && b                  | call and a b                       |
| return JSON             | respond json status value          |
| 原始 SQL / JSON         | embed kind name until terminator   |

# 附录 B 项目清单草案

项目清单可以使用单独的标准格式（例如 TOML），因为它属于构建数据而不是核心语言源码。若未来希望统一视觉，也可以提供 Verba 风格清单。首版建议优先复用成熟格式。

```toml
name = "user_service"
version = "0.1.0"
target = "go"

[database]
dialect = "postgres"
schema = "schema.sql"

[dependencies]
verba_http = "0.1"
verba_sql = "0.1"
```

# 附录 C 术语

| **术语**    | **定义**                                              |
| ----------------- | ----------------------------------------------------------- |
| Verb Core（词核） | Verba 中由核心 lexer 与 parser 直接处理的无标点程序结构层。 |
| 受控字面量        | 含符号但边界由类型上下文明确限定的单值。                    |
| 语法岛            | 由 embed 声明包围、交给专用适配器处理的原始区域。           |
| 适配器            | 负责语法岛解析、绑定、类型信息与代码生成的编译器插件。      |
| 词法隔离          | 岛内字符不被核心 lexer 解释。                               |
| 语义集成          | 岛资源仍参与核心类型检查、绑定检查和构建。                  |
| 结构无标点        | 核心控制结构不依赖括号、花括号、逗号、分号和符号运算符。    |

*— End of Design Draft —*
