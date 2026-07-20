# 11. 完整用户服务

本章示例位于 `learn/08_user_service`。它与 `design.md` 的完整示例保持一致，也是 MVP 的端到端验收项目。

## 项目组成

目录包含三个文件：

| 文件 | 用途 |
| --- | --- |
| `main.vrb` | record、错误 enum、正则、SQL、函数和路由 |
| `schema.sql` | PostgreSQL schema 快照，供编译器静态检查 |
| `verba.toml` | 模块身份、Go 目标和数据库配置 |

先执行静态检查与构建：

```powershell
./build/verba.exe fmt --check learn/08_user_service
./build/verba.exe check learn/08_user_service
./build/verba.exe audit learn/08_user_service
./build/verba.exe build -o build/user-service.exe learn/08_user_service
```

这个过程会同时证明：

- JSON 请求 record 可以静态解码。
- UUID 与 email 验证不会发生隐式类型转换。
- SQL 绑定名称、参数类型和结果列与 schema 一致。
- `optional user` 必须先检查再 `unwrap`。
- 路由错误能够转换为确定的 HTTP 状态。

## 创建用户

`create_user` 先验证 email，再生成 UUID，最后执行带 `RETURNING` 的插入：

```verba
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
```

这里的 SQL 失败会在函数边界转换为 `database_failure`。如果 `app_error` 没有这个 case，`verba check` 会拒绝程序。

路由使用显式 `201` 响应，同时让解析错误和业务错误继续通过 `try` 传播：

```verba
route create_user_route
method post
path /users
output result user app_error
begin
    let payload to be try call json_decode create_user_request request_body
    let created to be try call create_user payload
    respond json 201 created
end
```

## 查询用户

查询返回 `optional user`。缺失不是数据库故障，因此代码显式把它转换成 `user_not_found`：

```verba
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
```

状态映射为：无效 JSON、UUID 或 email 返回 400，用户不存在返回 404，数据库失败返回 500。未知错误也安全地回落到 500。

## 连接 PostgreSQL

先设置连接串并应用 schema，再启动生成的服务：

```powershell
$env:VERBA_DATABASE_URL = "postgres://postgres:postgres@127.0.0.1:5432/verba?sslmode=disable"
psql $env:VERBA_DATABASE_URL -f learn/08_user_service/schema.sql
$env:VERBA_ADDRESS = "127.0.0.1:8080"
./build/user-service.exe
```

保持服务运行，在另一个 PowerShell 终端创建用户，并使用响应中的 ID 立即查询同一个用户：

```powershell
$created = Invoke-RestMethod -Method Post -Uri http://127.0.0.1:8080/users -ContentType application/json -Body '{"name":"Alice","email":"alice@example.com"}'
$created
Invoke-RestMethod -Uri "http://127.0.0.1:8080/users/$($created.id)"
```

POST 应返回 201，随后 GET 应返回相同的 `id`、`name` 和 `email`。使用响应 ID 避免了固定 UUID 在空数据库中返回 404 的歧义。

## 验证错误边界

使用 `curl.exe -i` 可以同时看到响应头和状态码：

```powershell
curl.exe -i -X POST http://127.0.0.1:8080/users -H "Content-Type: application/json" -d '{"name":"Bad Email","email":"not-an-email"}'
curl.exe -i http://127.0.0.1:8080/users/not-a-uuid
curl.exe -i http://127.0.0.1:8080/users/00000000-0000-0000-0000-000000000000
```

三次请求应依次返回 400、400 和 404。数据库不可用或 SQL 执行失败时返回 500；未知的应用错误也只会回落到 500，不会泄漏内部错误文本。

## 完成检查

完成本章后，你应该能够证明：

- `fmt --check`、`check`、`audit` 和 `build` 全部成功。
- 创建响应为 201，并能用返回的 UUID 查询到同一用户。
- 非法 email、非法 UUID 和不存在的用户分别得到 400、400 和 404。
- schema 快照只负责编译期证明，实际数据库仍需要独立迁移。

至此，Verba 0.1.0 的入门路线完成。回到[教程首页](README.md)可以重新选择章节，或运行 `./scripts/verify.ps1` 一次验证全部教程项目。
