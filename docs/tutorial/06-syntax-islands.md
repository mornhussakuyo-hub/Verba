# 6. 语法岛

本章示例位于 `learn/05_islands`。

语法岛允许 JSON、SQL、HTML 和正则表达式保留原生符号，同时把边界显式交给 Verba 编译器。

## 通用结构

```verba
embed json metadata until end_metadata
{
  "name": "Verba"
}
end_metadata
```

`end_metadata` 必须独占一行。岛内的 `begin`、`end` 和其他 Verba 关键字都只是原始内容。

## JSON

JSON 岛会在编译期完整解析。非法 JSON 会产生 `JSON2001`，并定位到岛内行。

JSON 岛适合常量、示例和配置。普通 HTTP 请求体使用 `json_decode Record request_body`，普通响应使用 `respond json`。

## Regex

```verba
embed regex username_pattern until end_pattern
^[a-z][a-z0-9_]{2,15}$
end_pattern
```

终止标识符必须与声明中的名称完全相同、顶格且独占一行。岛内内容不会经过核心 lexer；其中的括号、引号、冒号或看似 Verba 的单词都不会改变外层结构。`verba fmt` 也会逐字节保留岛内正文。

正则会在编译期验证并在生成程序启动时预编译。调用时复用编译结果：

```verba
let valid to be call regex_match username_pattern value
```

## HTML 与 Text 模板

```verba
embed html page until end_page
<h1>{{ title }}</h1>
end_page
```

模板槽位必须使用标识符。调用 `render` 时必须恰好绑定每个槽位：

```verba
let output to be call render page
begin
    with title page_title
end
```

HTML 模板默认对绑定值进行转义；text 模板只做文本替换。

## SQL

```verba
embed sql find_user until end_find_user
SELECT id, name FROM users WHERE id = :id;
end_find_user
```

所有动态值必须通过命名参数块绑定。编译器会结合 `verba.toml` 配置的 PostgreSQL schema 快照，检查缺失、多余、重复和类型不兼容的绑定，并推导直接单表查询的结果行。

Go 后端使用 `database/sql` 与固定版本 pgx 生成 `sql_exec`、`sql_one`、`sql_optional`、`sql_many` 和事务代码。完整用法见第十章 [PostgreSQL：类型化查询与事务](10-postgresql.md)。

下一章：[工具链与排错](07-tooling.md)。
