# client-go 请求构建全流程解析

本章节记录 `client-go` 中 RESTClient 如何通过**链式调用 (Builder Pattern)** 构造 HTTP 请求，并最终将响应反序列化为 Go 结构体的完整生命周期。

---

## 1. 全景概览

```mermaid
graph TD
    %% 样式定义
    classDef method fill:#e1f5fe,stroke:#01579b,stroke-width:2px;
    classDef struct_field fill:#fff9c4,stroke:#fbc02d,stroke-width:2px;
    classDef action fill:#e8f5e9,stroke:#2e7d32,stroke-width:2px;
    classDef result fill:#f3e5f5,stroke:#7b1fa2,stroke-width:2px;

    Start([开始: c.client]) --> P1
    
    subgraph Phase 1 [第一阶段: 构造 *Request 对象 (Builder Pattern)]
        direction TB
        P1["1. Patch(pt)"]:::method
        P1 -- "创建 *Request 并设置 Verb/Header" --> R1["*Request 对象"]:::struct_field
        
        R1 --> N1["2. NamespaceIfScoped(ns)"]:::method
        N1 -- "填充 namespace 字段" --> R2["+ namespace='test'"]:::struct_field
        
        R2 --> Re1["3. Resource(res)"]:::method
        Re1 -- "填充 resource 字段" --> R3["+ resource='configmaps'"]:::struct_field
        
        R3 --> Na1["4. Name(name)"]:::method
        Na1 -- "填充 resourceName 字段" --> R4["+ resourceName='my-cm'"]:::struct_field
        
        R4 --> S1["5. SubResource(sub)"]:::method
        S1 -- "填充 subresource 字段" --> R5["+ subresource='' (未传参)"]:::struct_field
        
        R5 --> V1["6. VersionedParams(opts)"]:::method
        V1 -- "编码并填充 params 字段" --> R6["+ params={} (空 Options)"]:::struct_field
        
        R6 --> B1["7. Body(data)"]:::method
        B1 -- "填充 body 字段 (JSON 数据)" --> R7["+ body='{\"data\":{...}}'"]:::struct_field
    end

    subgraph Phase 2 [第二阶段: 执行与解析]
        R7 --> Do1["8. Do(ctx)"]:::action
        
        subgraph HTTP [HTTP 请求处理]
            Do1 -- "1. 组装 URL: /api/v1/namespaces/test/configmaps/my-cm" --> Send["2. 发送 HTTP PATCH 请求"]:::action
            Send -- "3. API Server 响应 JSON" --> Recv["4. 接收响应"]:::action
        end
        
        Recv --> Res["返回: Result 对象 (包含 JSON Body)"]:::result
    end

    subgraph Phase 3 [第三阶段: 结果映射]
        Res --> Into["9. Into(result)"]:::method
        Into -- "JSON 反序列化 (Unmarshal)" --> Final["最终结果: *corev1.ConfigMap 结构体已填充"]:::result
    end
```

---

## 2. 代码调用示例

```go
result1, err := clientset.CoreV1().ConfigMaps(namespace).Patch(
    ctx, 
    name,                       // "my-config"
    types.MergePatchType, 
    mergeData,                  // {"data":{"new-key":"value"}}
    metav1.PatchOptions{},
)
```

**底层等价实现** (`gentype/type.go`):
```go
func (c *Client[T]) Patch(ctx context.Context, name string, pt types.PatchType, 
    data []byte, opts metav1.PatchOptions, subresources ...string) (T, error) {
    
    result := c.newObject()
    err := c.client.Patch(pt).                    // 1. 创建 *Request
        NamespaceIfScoped(c.namespace, c.namespace != ""). // 2. 填 namespace
        Resource(c.resource).                     // 3. 填 resource 类型
        Name(name).                               // 4. 填 resource 名
        SubResource(subresources...).             // 5. 填子资源 (可选)
        VersionedParams(&opts, c.parameterCodec). // 6. 填 URL 参数
        Body(data).                               // 7. 填请求体
        Do(ctx).                                  // 8. 发送 HTTP 请求
        Into(result)                              // 9. 反序列化 JSON 响应
    return result, err
}
```

---

## 3. 请求结构体 `*Request` 定义

```go
type Request struct {
    c *RESTClient

    warningHandler WarningHandler
    rateLimiter    flowcontrol.RateLimiter
    backoff        BackoffManager
    timeout        time.Duration
    maxRetries     int

    // generic components accessible via method setters
    verb       string
    pathPrefix string
    subpath    string
    params     url.Values
    headers    http.Header

    // structural elements of the request
    namespace    string
    namespaceSet bool
    resource     string
    resourceName string
    subresource  string

    // output
    err error

    // only one of body / bodyBytes may be set
    body      io.Reader
    bodyBytes []byte
}
```

---

## 4. 逐步拆解

### 步骤 1: `c.client.Patch(pt)`

```go
// Patch begins a PATCH request. Short for c.Verb("Patch").
func (c *RESTClient) Patch(pt types.PatchType) *Request {
    return c.Verb("PATCH").SetHeader("Content-Type", string(pt))
}
```

**动作**：
1. `NewRequest(c)`：根据 `RESTClient` 配置创建 `*Request` 骨架（服务器地址、超时、限流等）。
2. `Verb("PATCH")`：设置 HTTP 方法。
3. `SetHeader("Content-Type", string(pt))`：设置 `Content-Type`（如 `application/merge-patch+json`）。

**结构体状态**：
```go
&Request{
    c:             <RESTClient>,
    verb:          "PATCH",
    pathPrefix:    "/api/v1",
    headers:       {"Accept": "application/json, */*"},
    rateLimiter:   <...>,
    timeout:       <30s>,
    
    // 🟡 未填充
    namespace:     "",
    resource:      "",
    resourceName:  "",
    body:          nil,
}
```

---

### 步骤 2: `NamespaceIfScoped(c.namespace, c.namespace != "")`

```go
func (r *Request) NamespaceIfScoped(namespace string, scoped bool) *Request {
    if scoped {
        return r.Namespace(namespace)
    }
    return r
}

func (r *Request) Namespace(namespace string) *Request {
    r.namespaceSet = true
    r.namespace = namespace
    return r
}
```

**动作**：
- 因为 `scoped=true`，调用 `r.Namespace("test")`。
- 填充 `namespace` 和 `namespaceSet` 字段。

**结构体变化**：
```go
&Request{
    namespace:    "test",       // 🔑 新填充
    namespaceSet: true,
    // ... 其他不变 ...
}
```

**意义**：告诉请求对象，URL 路径需要包含 `/namespaces/test`。

---

### 步骤 3: `Resource(c.resource)`

```go
func (r *Request) Resource(resource string) *Request {
    r.resource = resource
    return r
}
```

**动作**：填充 `resource` 字段为 `"configmaps"`。

**结构体变化**：
```go
&Request{
    resource: "configmaps",     // 🔑 新填充
    // ...
}
```

**URL 路径骨架**：`/api/v1/namespaces/test/configmaps/`

---

### 步骤 4: `Name(name)`

```go
func (r *Request) Name(resourceName string) *Request {
    r.resourceName = resourceName
    return r
}
```

**动作**：填充 `resourceName` 字段为 `"my-config"`。

**结构体变化**：
```go
&Request{
    resourceName: "my-config",  // 🔑 新填充
    // ...
}
```

**URL 路径骨架**：`/api/v1/namespaces/test/configmaps/my-config`

---

### 步骤 5: `SubResource(subresources...)`

```go
func (r *Request) SubResource(subresources ...string) *Request {
    subresource := path.Join(subresources...)
    r.subresource = subresource
    return r
}
```

**动作**：因为调用时**未传参**，Go 自动传入空切片 `[]string{}`。
- `path.Join([]string{}...)` 返回 `""`。
- `r.subresource` 保持为空。

**结构体变化**：
```go
&Request{
    subresource: "",            // 🔑 保持为空 (未传参)
    // ...
}
```

**何时使用？**
- 更新 CRD 状态时：`r.Status().Update(...)` → 底层传 `"status"`。
- 扩缩容时：`/scale` 子资源。

**本例 URL 路径**：`/api/v1/namespaces/test/configmaps/my-config` (无 `/status` 后缀)

---

### 步骤 6: `VersionedParams(&opts, c.parameterCodec)`

```go
func (r *Request) VersionedParams(obj runtime.Object, codec runtime.ParameterCodec) *Request {
    return r.SpecificallyVersionedParams(obj, codec, r.c.content.GroupVersion)
}

func (r *Request) SpecificallyVersionedParams(obj runtime.Object, codec runtime.ParameterCodec, version schema.GroupVersion) *Request {
    params, err := codec.EncodeParameters(obj, version)
    if err != nil {
        r.err = err
        return r
    }
    for k, v := range params {
        if r.params == nil {
            r.params = make(url.Values)
        }
        r.params[k] = append(r.params[k], v...)
    }
    return r
}
```

**动作**：
- 将 `metav1.PatchOptions{}` 编码为 URL 查询参数。
- 因为传入的是**空结构体**，`params` 依然为空。

**结构体变化**：
```go
&Request{
    params: url.Values{},       // 🔑 编码后为空 (因为 opts 为空)
    // ...
}
```

**如果传入 `PatchOptions{FieldManager: "my-manager"}`**：
```go
params: url.Values{
    "fieldManager": []string{"my-manager"},
}
```

**最终 URL 会变成**：`/api/v1/.../my-config?fieldManager=my-manager`

---

### 步骤 7: `Body(data)`

```go
func (r *Request) Body(obj interface{}) *Request {
    switch t := obj.(type) {
    case []byte:
        r.bodyBytes = t
    case io.Reader:
        r.body = t
    // ...
    }
    return r
}
```

**动作**：将 JSON Patch 数据存入 `body` 字段。

**结构体变化**：
```go
&Request{
    body: bytes.NewReader([]byte(`{"data":{"new-key":"value"}}`)), // 🔑 新填充
    // ...
}
```

---

### 📊 此时结构体的完整状态（调用 `Do` 之前）

| 字段 | 值 | 作用 |
| :--- | :--- | :--- |
| **verb** | `"PATCH"` | HTTP 方法 |
| **namespace** | `"test"` | URL 路径 |
| **resource** | `"configmaps"` | URL 路径 |
| **resourceName** | `"my-config"` | URL 路径 |
| **subresource** | `""` | 无子资源 |
| **params** | `{}` | 无查询参数 |
| **body** | `{...JSON Data...}` | 请求体 |
| **headers** | `{"Content-Type": "..."}` | 告诉服务器怎么解析 Body |

---

### 步骤 8: `Do(ctx)`

```go
func (r *Request) Do(ctx context.Context) Result {
    var result Result
    err := r.request(ctx, func(req *http.Request, resp *http.Response) {
        result = r.transformResponse(resp, req)
    })
    if err != nil {
        return Result{err: err}
    }
    return result
}
```

**动作**：
1. **组装 URL**：拼接所有路径和参数
   ```
   https://<host>/api/v1/namespaces/test/configmaps/my-config
   ```
2. **创建 HTTP Request**：`http.NewRequest("PATCH", url, body)`
3. **发送请求**：`http.Client.Do(req)` (含限流、重试、超时管理)
4. **接收响应**：把 HTTP Response Body 读入 `Result.body`。
5. **错误检查**：如果状态码是 4xx/5xx，解析为 `metav1.Status` 并存入 `Result.err`。

**返回**：`Result{body: [...], err: nil}` (或包含错误信息)

---

### 步骤 9: `Into(result)`

```go
func (r Result) Into(obj runtime.Object) error {
    if r.err != nil {
        return r.Error()
    }
    if len(r.body) == 0 {
        return fmt.Errorf("0-length response...")
    }
    
    // ✨ 核心魔法：JSON 反序列化
    out, _, err := r.decoder.Decode(r.body, nil, obj)
    if err != nil || out == obj {
        return err
    }
    
    // 检查是否返回了 Status 对象
    if t, ok := out.(*metav1.Status); ok {
        if t.Status != metav1.StatusSuccess {
            return errors.FromObject(t)
        }
    }
    return nil
}
```

**动作**：
- 把 API Server 返回的 JSON 字节流 (`r.body`) 反序列化到 `obj` 指针指向的结构体 (`result`)。
- 例如：`"data": {"key": "value"}` → `result.Data["key"] = "value"`。

**最终结果**：
- `err == nil`：`result` 变量已被填充为 `*corev1.ConfigMap`。
- `err != nil`：返回错误（如 409 Conflict 或解码失败）。

---

## 5. 总结

`client-go` 的请求构建遵循经典的 **Builder Pattern**：

| 阶段 | 步骤 | 核心职责 |
| :--- | :--- | :--- |
| **构造期** | `Patch()` → `Body()` | 逐步填充 `*Request` 结构体的各个字段 |
| **执行期** | `Do(ctx)` | 组装 URL，发送 HTTP 请求，返回 `Result` |
| **解析期** | `Into(result)` | 将 JSON 响应反序列化为 Go 结构体 |

这种设计的优势在于：
1. **链式调用**：代码可读性极高，意图清晰。
2. **延迟执行**：只有在 `Do()` 时才真正发起网络请求。
3. **类型安全**：通过泛型和强类型结构体，编译期即可发现大部分错误。
