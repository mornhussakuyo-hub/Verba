# 5. HTTP 服务

本章示例位于 `learn/04_http`。

## 路径参数

```verba
route greet
method get
path /hello/{name}
begin
    respond json 200 name
end
```

花括号中的路径参数会以同名 `string` 绑定进入路由作用域。生成的 Go 服务使用 `net/http` 的方法与路径模式。

## 标准请求绑定

每个路由自动获得：

| 名称 | 类型与用途 |
| --- | --- |
| 路径参数名 | `string` |
| `request_body` | `bytes` 请求体 |
| `request_headers` | 请求头映射 |
| `request_context` | 取消、截止时间和追踪上下文 |

JSON 请求体可以解码到 record。解码属于可失败操作，因此与 `try` 和 `result` 配合使用。

```verba
record uuid_request
begin
    field id string
end

route validate_uuid
method post
path /validate-uuid
begin
    let payload to be try call json_decode uuid_request request_body
    let raw_id to be get payload id
    let parsed_id to be try call parse_uuid raw_id
    respond json 200 parsed_id
end
```

没有声明输出类型的路由仍是一个简单 HTTP 错误边界。`try` 遇到无效 JSON 或 UUID 时会停止当前路由并返回 400；其他未分类失败返回 500。

## 类型化路由结果

生产服务通常希望应用错误具有稳定状态码。路由可以声明 `result` 输出：

```verba
enum app_error
begin
    case invalid_request
    case user_not_found
    case database_failure
end

route find_user
method get
path /users/{id}
output result uuid app_error
begin
    let user_id to be try call parse_uuid id
    return call ok user_id
end
```

`return call ok value` 生成 200 JSON 响应。也可以继续用 `respond json 201 value` 选择明确的成功状态。错误 case 使用固定约定：

| case | HTTP 状态 |
| --- | --- |
| `invalid_request`、`invalid_email` | 400 |
| `user_not_found` | 404 |
| `database_failure` | 500 |
| 其他 case | 500 |

`json_decode` 和 `parse_uuid` 原本返回字符串错误。类型化路由只有在错误枚举包含 `invalid_request` 时才允许用 `try` 自动传播它们。SQL 调用采用同样的严格规则：目标枚举必须包含 `database_failure`。遗漏 case 会在编译期报告，而不是运行时猜测。

## 响应

```verba
respond text 200 service ready
respond json 201 created
respond empty 204
```

- `text` 主体必须是字符串。
- `json` 会调用生成的 JSON 编码逻辑。
- `empty` 不能携带主体。
- 状态码必须位于 100 到 599。

`respond` 会终止当前路由分支。没有显式响应的路由默认返回 204。

## 运行与测试

```powershell
./build/verba.exe run learn/04_http
```

另开终端：

```powershell
curl http://127.0.0.1:8080/health
curl http://127.0.0.1:8080/hello/Alice
curl -Method Post http://127.0.0.1:8080/validate-uuid -ContentType application/json -Body '{"id":"550e8400-e29b-41d4-a716-446655440000"}'
```

下一章：[语法岛](06-syntax-islands.md)。
