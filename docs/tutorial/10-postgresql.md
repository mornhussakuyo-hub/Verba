# 10. PostgreSQL：类型化查询与事务

本章示例位于 `learn/07_postgres`。它把前面学过的 record、result、JSON、UUID、decimal 和项目清单连接成一个真实数据库边界。

## 准备 schema 快照

Verba 不会在编译时连接生产数据库。项目提交一份可审查的 PostgreSQL schema 快照：

```sql
CREATE TABLE accounts (
    id uuid PRIMARY KEY,
    name text NOT NULL,
    balance numeric NOT NULL,
    created_at timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP
);
```

然后在 `verba.toml` 中声明方言和路径：

```toml
[database]
dialect = "postgres"
schema = "schema.sql"
```

路径必须留在项目目录内。编译器会解析 `CREATE TABLE`，检查重复表和列、未知类型、主键非空性，并把 PostgreSQL 标量映射到 Verba 类型。例如 `uuid` 对应 `uuid`，`numeric` 对应精确 `decimal`，`timestamptz` 对应 `time`。

## 编写类型化 SQL 岛

动态值只能写成命名参数：

```verba
embed sql insert_account until end_insert_account
INSERT INTO accounts (id, name, balance)
VALUES (:id, :name, :balance)
RETURNING id, name, balance, created_at;
end_insert_account
```

编译器把参数按首次出现顺序改写成 PostgreSQL 的 `$1`、`$2`、`$3`，重复使用同名参数时会复用同一个编号。字符串、注释和 dollar-quoted string 内看似参数的文本不会被改写。

参数类型来自目标列。上面的 `:id`、`:name` 和 `:balance` 分别要求 `uuid`、`string` 和 `decimal`。绑定缺失、多余、重复或类型不兼容都会在 `verba check` 阶段失败。

## 结果行类型

查询直接选择列，或在 INSERT、UPDATE、DELETE 后使用 `RETURNING`：

```verba
embed sql find_account until end_find_account
SELECT id, name, balance, created_at
FROM accounts
WHERE id = :id;
end_find_account
```

如果结果列的名称和类型与一个 record 完全一致，编译器会复用它：

```verba
record account
begin
    field id uuid
    field name string
    field balance decimal
    field created_at time
end
```

没有匹配 record 时，编译器会为该 SQL 岛生成专用行类型。可空数据库列会成为 `optional T` 结果字段；参数本身仍使用基础标量类型，同时也允许绑定对应的 optional 值。

当前类型化适配器有意限制为直接单表 `SELECT`、`INSERT ... VALUES`、`UPDATE` 和 `DELETE`。JOIN、子查询、集合运算和动态标识符会收到明确诊断，而不是退化为未知行或运行时猜测。

## 四种执行接口

```verba
let created to be try call sql_one insert_account
begin
    with id id
    with name name
    with balance opening_balance
end
```

- `sql_exec` 用于不返回行的语句，成功值是受影响行数。
- `sql_one` 要求恰好一行；零行和多行都是错误。
- `sql_optional` 允许零行或一行，成功值是 `optional Row`。
- `sql_many` 读取所有行，成功值是 `list Row`。

四者都返回 `result ... string`。在返回自定义错误枚举的函数中，如果该枚举声明了 `database_failure`，`try` 会把驱动错误安全映射到这个 case。路由本身是错误边界，未处理的数据库错误会返回 HTTP 500。

## 显式事务

```verba
transaction database
begin
    try call sql_exec update_balance
    begin
        with balance balance
        with id account_id
    end
end
```

进入块时生成程序调用 `BeginTx`，块内 SQL 使用同一个 `*sql.Tx`。正常离开时提交；任何 `try` 失败先回滚，再沿函数 result 或路由错误边界传播。

事务块不能嵌套，也不能从内部直接 `return` 或 `respond`。这些限制保证每条控制流都有确定的提交或回滚结果。

## 构建与运行

首次构建 SQL 项目时，构建驱动会在隔离的临时 Go 模块中解析固定版本 `pgx/v5.7.6`：

```powershell
./build/verba.exe check learn/07_postgres
./build/verba.exe build -o build/postgres-service.exe learn/07_postgres
```

启动程序前设置连接串：

```powershell
$env:VERBA_DATABASE_URL = "postgres://postgres:postgres@127.0.0.1:5432/verba?sslmode=disable"
./build/postgres-service.exe
```

生成程序通过 `database/sql` 管理连接池，并在启动时 `Ping` 数据库。没有 `VERBA_DATABASE_URL`、连接失败或 schema 与运行数据库漂移时，程序会立即失败，不会带着不可用数据库静默启动。

可以用以下请求练习：

```powershell
curl.exe -X POST http://127.0.0.1:8080/accounts -H "Content-Type: application/json" -d '{"name":"Alice","opening_balance":19.90}'
curl.exe http://127.0.0.1:8080/accounts/550e8400-e29b-41d4-a716-446655440000
```

至此，你已经走完 Verba 当前工具链的数据库纵向路径：源码和语法岛经过静态检查，生成可构建的 Go 服务，并在运行时保持 HTTP、JSON、精确数值与 PostgreSQL 的类型边界。

下一章：[完整用户服务](11-user-service.md)。
