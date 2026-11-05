# 07-client-anatomy · 拆开 client.New

## 目标

同样的四个操作，两种实现：

```text
① List   test 命名空间的 ConfigMap
② Get    demo-config
③ Create demo-client-anatomy
④ Delete 它
```

| 目录 | 做法 |
|---|---|
| `with-client/` | `client.New(...)` → `c.List / c.Get / c.Create / c.Delete` |
| `manual/` | 只用 `rest.RESTClient`，手写 URL 拼接 |

## 实测输出（两者完全一致）

```text
========== 版本 A：client.New ==========
[List] test 命名空间 ConfigMap 数量: 2
[Get ] demo-config data 键数量: 1
[Create] 成功，UID=85f3985d-50ce-426a-aeca-e25b2a003fdd
[Delete] 成功
```

```text
========== 版本 B：手写 rest.RESTClient ==========
[List] test 命名空间 ConfigMap 数量: 2
[Get ] demo-config data 键数量: 1
[Create] 成功，UID=1fb3425c-59e2-4bd2-834f-a90fd878a3ed
[Delete] 成功
```

## `client.New` 内部 vs 手写版本

| `client.New` 内部动作 | 手写版本对应 |
|---|---|
| `rest.CopyConfig` + 默认 UserAgent | `rest.CopyConfig(cfg)` |
| `rest.HTTPClientFor` | 同一个 |
| `scheme` 默认值 | 显式传 `scheme` |
| `apiutil.NewDynamicRESTMapper` | ❌ 跳过（手工写 `configmaps`） |
| `serializer.NewCodecFactory(scheme)` | ✅ 一样 |
| `apiutil.RESTClientForGVK(gvk)` | 手工设 `GroupVersion` + `APIPath` + `NegotiatedSerializer` |
| `clientRestResources` 缓存每种类型的 client | ❌ 不复刻 |
| `typedClient.Get` → `o.Get().NamespaceIfScoped(...).Resource(...)` | ✅ 手写同样的链式调用 |
| 挂载 `Cache.Reader` | ❌ 纯直连 |

## 这行代码到底在干什么

```go
rc.Get().
	Namespace("test").
	Resource("configmaps").
	Name("demo-config").
	Do(ctx).
	Into(&cm)
```

展开成 HTTP：

```text
GET https://<host>/api/v1/namespaces/test/configmaps/demo-config
```

`client.Get` 做的事就是：

```text
对象 *corev1.ConfigMap
   │ scheme.GVKForObject
   ▼
GVK ConfigMap/v1
   │ mapper.RESTMapping
   ▼
资源名 configmaps + namespaced=true
   │ 拼 URL
   ▼
GET /api/v1/namespaces/test/configmaps/demo-config
   │ 解码
   ▼
&cm
```

**手写版本把中间两步（GVK→资源名、拼 URL）直接写死了。**

## 踩坑记录：`APIPath` 不设会 404

第一版手写代码：

```text
List 失败: the server could not find the requested resource (get configmaps)
```

打印请求 URL：

```text
https://47.110.225.40:6443/v1/namespaces/test/configmaps
                               ↑ 少了 /api
```

原因：`rest.Config.APIPath` 默认为空，`RESTClient` 不会自动补前缀。

controller-runtime 的 `createRestConfig` 是这么处理的：

```go
if gvk.Group == "" {
	cfg.APIPath = "/api"      // core 组
} else {
	cfg.APIPath = "/apis"     // 其他组
}
```

补上 `restCfg.APIPath = "/api"` 后就正常了。

**这也说明：`client.New` 帮你处理的正是这些“路径细节”。**

## `client.New` 额外给了你什么

| 能力 | 手写版本 |
|---|---|
| 从对象自动推 GVK | 要自己写 GVK |
| 自动查 mapper 得资源名、是否 namespaced | 手写 `configmaps` / `Namespace(...)` |
| 每种类型的 RESTClient 缓存 | 每次重建 |
| `List` 自动处理 `XxxList` 后缀 | 手动 |
| 支持 Unstructured / PartialObjectMetadata | 不支持 |
| protobuf 协商 | 默认 JSON |
| Warning 日志 | 无 |
| 挂缓存（读走 informer） | 无 |

## 一句话

> **`client.New` = `RESTClientForGVK` + 类型→路径翻译 + 按类型缓存 + 可选挂缓存。**
> **底层就是 `rest.RESTClient` 的链式调用，只是把“从对象推路径”这件事自动化了。**

## 运行

```bash
cd 02Manager/07-client-anatomy
go run ./with-client
go run ./manual
# 或
make both
```