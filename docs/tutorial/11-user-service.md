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

生成的服务启动时读取 `VERBA_DATABASE_URL` 并验证连接：

```powershell
$env:VERBA_DATABASE_URL = "postgres://postgres:postgres@127.0.0.1:5432/verba?sslmode=disable"
$env:VERBA_ADDRESS = "127.0.0.1:8080"
./build/user-service.exe
```

先把 `schema.sql` 应用到数据库，再调用接口：

```powershell
curl -Method Post http://127.0.0.1:8080/users -ContentType application/json -Body '{"name":"Alice","email":"alice@example.com"}'
curl http://127.0.0.1:8080/users/550e8400-e29b-41d4-a716-446655440000
```

schema 快照只用于编译期证明，不会自动执行迁移。部署流程必须保证实际数据库与快照一致。
