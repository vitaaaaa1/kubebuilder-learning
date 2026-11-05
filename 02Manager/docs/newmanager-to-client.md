# 从 NewManager 到 Client：拆解总结

> 本文汇总两个实验目录的结论：
>
> | 目录 | 主题 | 内容 |
> |---|---|---|
> | `06-manager-anatomy/` | 拆开 `ctrl.NewManager` | `with-manager/` 与 `manual/` 两个等价实现 |
> | `07-client-anatomy/` | 拆开 `client.New` | `with-client/` 与 `manual/`（直接手写 `rest.RESTClient`） |
>
> 所有数据均来自真实集群（namespace `test`，context `dev-cluster`），
> 依赖版本：controller-runtime `v0.25.2`、client-go / apimachinery `v0.37.1`。

---

## 1. 全景图

```text
kubeconfig
   │ clientcmd
   ▼
*rest.Config ──────────────► rest.HTTPClientFor ──► *http.Client
   │                                                     │
   │                                                     ▼
   └──────────────► apiutil.NewDynamicRESTMapper ◄───────┘
                            │  (discovery 懒加载)
                            ▼
                     meta.RESTMapper
                            │
        ┌───────────────────┴────────────────────┐
        ▼                                        ▼
   cache.New(cfg, {Scheme,Mapper,HTTPClient})   client.New(...)
        │                                   ┌────┴────┐
        │                                   │         │
        │                             带 Cache    不带 Cache
        │                                   │         │
        ▼                                   ▼         ▼
   Cache（informer 容器）            cachedClient  apiReader
        │                            (读内存)    (读 apiserver)
        │
        ├─ Start(ctx)          启动 List+Watch
        └─ WaitForCacheSync()  等首次全量同步
```

一句话分工：

| 层 | 职责 |
|---|---|
| `rest.Config` | 连哪里、怎么认证 |
| `http.Client` | TLS + 认证 + 连接池 |
| `RESTMapper` | Go 类型 ↔ 资源 URL |
| `Cache` | informer 容器（数据驻留内存） |
| `client` | 读缓存 / 写直连 |
| `rest.RESTClient` | 真正的 HTTP 请求与编解码 |

---

## 2. 逐层拆解

### 2.1 `cfg := ctrl.GetConfigOrDie()`

**做了什么**

```text
$KUBECONFIG → ~/.kube/config → in-cluster ServiceAccount
```

解析成 `*rest.Config`。

**实测**

```text
Host         : https://47.110.225.40:6443
CAData len   : 1359      ← 集群 CA
CertData len : 1379      ← 客户端证书（mTLS）
KeyData len  : 1675
cfg.QPS      : -1        ← controller-runtime 特意关闭客户端限速
```

**关键点**

- `QPS = -1` 不是 kubeconfig 写的，是 `pkg/client/config/config.go:101` 设的：
  默认关闭客户端限速，交给 apiserver 的 APF。
- `QPS`/`Burst` 只在 `rest.RESTClient` 里生效，与 `HTTPClientFor` 无关。

| | 对应文件 |
|---|---|
| 06 | `manual/main.go:32` |
| 07 | `with-client/main.go:20` |

---

### 2.2 `httpClient, err := rest.HTTPClientFor(cfg)`

**做了什么**

把 `rest.Config` 翻译成 `*http.Client`：

```text
TLS（CA + 客户端证书）
认证（Bearer / Basic / exec 插件 / Impersonate）
代理、User-Agent
Timeout
```

**实测**

```text
httpClient : *http.Client
Timeout    : 0s
Transport  : *transport.trackedTransport
```

**关键点**

- **不发请求**，只是装配。
- `Timeout = 0`：靠 `context` 控制超时。
- 后面 mapper / cache / client 共用这一个实例 → 共享连接池。

---

### 2.3 `mapper, err := apiutil.NewDynamicRESTMapper(cfg, httpClient)`

**做了什么**

```go
client, _ := discovery.NewDiscoveryClientForConfigAndClient(cfg, httpClient)

return &mapper{
	mapper: restmapper.NewDiscoveryRESTMapper(
		[]*restmapper.APIGroupResources{}),   // ← 空表
	client: client,
	...
}
```

**懒加载机制**

```text
第一次 RESTMapping
  └── 空表 → NoMatch
        └── addKnownGroupAndReload
              └── aggregated discovery（/api + /apis）
                    └── 重建表 → 重试成功
```

**实测**

```text
构造 mapper        : 17.417µs
第一次 RESTMapping : 245~261ms      GVR=/v1, Resource=configmaps
第二次 RESTMapping : 18~27µs
Deployment 第一次  : 17µs           ← 聚合发现已把全量资源拉回
Widget（不存在）   : 41ms → no matches for kind "Widget"
```

**关键点**

- 构造时零网络请求；成本推迟到第一次使用。
- 一次聚合发现覆盖**所有 group**，所以后面任何 Kind 都是微秒级。
- 并发安全：读 `RLock`，reload `Lock`。

---

### 2.4 `cmCache, err := cache.New(cfg, cache.Options{...})`

**做了什么**

```go
opts, _ := defaultOpts(cfg, opts)       // 补默认值
newCacheFunc := newCache(cfg, opts)     // 造单个 informerCache 的工厂

if len(opts.DefaultNamespaces) > 0 {
	defaultCache = newMultiNamespaceCache(...)   // 每个 ns 一个
} else {
	defaultCache = newCacheFunc(...)             // 全命名空间一个
}
```

**实测**

```text
cache 类型: *cache.multiNamespaceCache
Start 之前 Get: err = the cache is not started, can not read objects
Start 之后 Get: err = <nil>
```

**`DefaultNamespaces` 的语义**

```go
map[string]cache.Config{
	"test": {},     // key = 命名空间名；value = 该命名空间的缓存配置
}
```

- `"test"` 是 **map 的键**，不是 `Config` 的字段
- `{}` 是零值 `cache.Config`（不过滤、不转换）
- `""`（`cache.AllNamespaces`）表示“其余所有命名空间”
- **只限制缓存，不限制写和 `apiReader`**

**关键点**

- 构造时 0 个 informer，一个请求都不发
- informer 是**懒创建**的：第一次 `Get`/`List`/`Watch` 才建
- 默认 `SyncPeriod` 10 小时（resync，不是重新 List）

---

### 2.5 `client.New` 两次

```go
// ① cachedClient：读缓存、写直连
cachedClient, _ := client.New(cfg, client.Options{
	Scheme: scheme, Mapper: mapper, HTTPClient: httpClient,
	Cache: &client.CacheOptions{Reader: cmCache},
})

// ② apiReader：全直连
apiReader, _ := client.New(cfg, client.Options{
	Scheme: scheme, Mapper: mapper, HTTPClient: httpClient,
})
```

**构造过程（两者完全一样，只差最后一步）**

| # | 动作 |
|---|---|
| 1 | `rest.CopyConfig` |
| 2 | 补 `UserAgent` |
| 3 | 装 `WarningHandler` |
| 4 | 缺省补 `HTTPClient`/`Scheme`/`Mapper`/`Log` |
| 5 | 建 `clientRestResources`（含 `serializer.CodecFactory`） |
| 6 | 建 metadata-only 客户端 |
| 7 | 组装 `typedClient` / `unstructuredClient` / `metadataClient` |
| 8 | 有 `Cache.Reader` 就挂上，并预计算 `DisableFor` 的 GVK |

**实测构造耗时**

```text
client.New 第 1 次: 15µs
client.New 第 2 次:  8µs
client.New 第 3 次:  7µs
```

**读写路由**

```text
Get / List           → cache（有 Cache.Reader 时）
Create/Update/Delete → typedClient（永远直连）
```

**实测（缓存未启动时）**

```text
Create（写）: err = <nil>                       ← 直连，成功
Get（读）   : the cache is not started, ...     ← 走缓存，失败
```

**两个 client 的差异**

| | cachedClient | apiReader |
|---|---|---|
| 读 | 内存 | 直连 |
| 写 | 直连 | 直连 |
| 一致性 | 最终一致 | 强一致 |
| 受 `DefaultNamespaces` 限制 | ✅ | ❌ |
| 依赖缓存启动 | ✅ | ❌ |
| 实测 List 数量 | 2（仅 test） | 119（全集群） |

---

### 2.6 `go cmCache.Start(ctx)` + `WaitForCacheSync(ctx)`

**`Start` 做了什么**

```go
if ip.started { return errors.New("informer already started") }
ip.ctx = ctx
for _, i := range ip.tracker.Structured { ip.startInformerLocked(i) }
ip.started = true
close(ip.startWait)

<-ctx.Done()          // ← 阻塞在这里
ip.waitGroup.Wait()
```

**关键点**

| 行为 | 说明 |
|---|---|
| 阻塞 | 必须 `go` 出去，否则到不了 `WaitForCacheSync` |
| 幂等性 | 重复调用返回 `informer already started` |
| `started` 标志 | 之后新建的 informer 会自动启动 |
| 一个 informer 一个 goroutine | `RunWithContext` 里跑 List+Watch |

**`WaitForCacheSync` 做了什么**

```text
waitForStarted(ctx)                      ← 等 startWait 关闭
  └── cache.WaitForCacheSync(所有 HasSynced)
```

**实测**

```text
Start 之前 WaitForCacheSync: false（300ms 超时）
```

**一个容易误判的现象**

`manual/main.go` 里没有提前 `GetInformer`，所以：

```text
WaitForCacheSync: ok=true 耗时=64µs      ← 空集合，等于没等
第一次 List:      2 个, 耗时=355ms       ← 这里才懒创建 + 等同步
第二次 List:      2 个, 耗时=126µs
```

对比提前 `GetInformer` 的情况：

```text
只有 ConfigMap:            ok=true 耗时=101ms
ConfigMap+Secret+Service:  ok=true 耗时=101ms    ← 并行，与数量无关
```

**结论：`WaitForCacheSync` 只等“已注册”的 informer。**

---

### 2.7 读的路径

```text
cachedClient.List
  └── cache.List
        └── getInformerForKind
              └── Informers.Get
                    ├── 已存在 → 直接用
                    ├── 不存在 → 创建（started=true 时自动启动）
                    └── !HasSynced → 阻塞等同步
```

`apiReader.List` 则直接：

```text
typedClient.List
  └── rest.RESTClient.Get().Namespace().Resource().Do()
```

---

### 2.8 再往下：`client.New` 内部就是 `rest.RESTClient`

`07-client-anatomy/manual` 手工复刻：

```go
codecs := serializer.NewCodecFactory(scheme)

restCfg := rest.CopyConfig(cfg)
restCfg.GroupVersion = &corev1.SchemeGroupVersion
restCfg.NegotiatedSerializer = codecs.WithoutConversion()
restCfg.APIPath = "/api"                    // ← core 组；其它组是 "/apis"

rc, _ := rest.RESTClientForConfigAndClient(restCfg, httpClient)

rc.Get().Namespace("test").Resource("configmaps").Name("demo-config").
	Do(ctx).Into(&cm)
```

**两版输出完全一致**

```text
[List] test 命名空间 ConfigMap 数量: 2
[Get ] demo-config data 键数量: 1
[Create] 成功，UID=...
[Delete] 成功
```

**踩坑：`APIPath` 不设会 404**

```text
List 失败: the server could not find the requested resource (get configmaps)
实际 URL: https://47.110.225.40:6443/v1/namespaces/test/configmaps
                                   ↑ 少了 /api
```

controller-runtime 在 `createRestConfig` 里补的：

```go
if gvk.Group == "" {
	cfg.APIPath = "/api"
} else {
	cfg.APIPath = "/apis"
}
```

---

## 3. `NewManager` 的 13 个动作：覆盖情况

| # | `NewManager` 做的事 | 06-manual 覆盖 | 说明 |
|---|---|---|---|
| 1 | `setOptionsDefaults` | ❌ | 默认值、注入点 |
| 2 | RESTMapper | ✅ | 2.3 |
| 3 | Cache | ✅ | 2.4 |
| 4 | cachedClient | ✅ | 2.5 |
| 5 | apiReader | ✅ | 2.5 |
| 6 | EventRecorder | ❌ | |
| 7 | Leader Election 锁 | ❌ | |
| 8 | Metrics server（绑端口） | ❌ | |
| 9 | Health probe（绑端口） | ❌ | |
| 10 | Pprof（绑端口） | ❌ | |
| 11 | Webhook server | ❌ | |
| 12 | 6 组 runnables | ❌ | |
| 13 | 优雅关闭字段 | ❌ | |

**结论：数据面（2–5）可以手写复刻；生命周期面（1、6–13）才是 `NewManager` 的真正价值。**

---

## 4. 十个关键结论

1. `NewManager` / `cache.New` / `client.New` **都不发网络请求**（毫秒内完成）。
2. `NewManager` 的唯一副作用是**绑定端口**（metrics/healthz/pprof）。
3. `RESTMapper` 懒加载：首次查询约 250ms，之后微秒级。
4. 聚合发现一次拉全量，所以第二个 Kind 免费。
5. `Cache` 构造时 0 个 informer；informer 在第一次使用时创建。
6. `cache.Start` 阻塞到 ctx 取消，必须放 goroutine。
7. `WaitForCacheSync` 只等已注册的 informer；未注册的会在首次访问时补等。
8. `cachedClient` 与 `apiReader` 是同一个构造函数，差 `Cache.Reader`。
9. 写永远直连；读走缓存时未启动会返回 `ErrCacheNotStarted`。
10. `DefaultNamespaces` 只约束缓存，`apiReader` 与写操作不受限（2 vs 119）。

---

## 5. FAQ / 常见误解

**Q：`"test": {}` 里的 `"test"` 是 `cache.Config` 的字段吗？**

不是。`map[string]cache.Config` 的 **key 是命名空间名**，`{}` 是零值配置。

**Q：`NewManager` 会连 apiserver 吗？**

不会。实测 450µs。真正的连接发生在第一次 List/Watch。

**Q：`client.New` 之后就能读了吗？**

- `apiReader`：能。
- `cachedClient`：要先 `cache.Start` + 同步，否则 `ErrCacheNotStarted`。

**Q：为什么 `WaitForCacheSync` 只花了 64µs？**

因为当时没有注册任何 informer，等的是空集合。

**Q：写完之后马上用 `cachedClient` 读，读不到？**

正常。写直连、读缓存，中间有事件传播延迟。

**Q：client-go 在哪？**

一直在用：`rest`（请求）、`tools/cache`（informer）、`discovery`（mapper）。
只是没用到最上层的 `kubernetes.Clientset`。

---

## 6. 还没覆盖的部分（控制面）

```text
NewControllerManagedBy
  ├── For      → source.Kind（Watch 缓存）
  └── Complete → Build
        ├── doWatch      → controller.Watch
        └── doController → controller.New + mgr.Add

mgr.Start 的 6 组 runnables
  Warmup → Others → LeaderElection → Caches → Webhooks → HTTPServers
```

这部分对应你之前问的“谁消费缓存里的变化、Reconcile 怎么被调度”。

---

## 7. 运行方式

```bash
cd 02Manager

# 06：NewManager vs 手动组装
cd 06-manager-anatomy && make both

# 07：client.New vs rest.RESTClient
cd 02Manager/07-client-anatomy && make both
```

预期差异：

```text
06：前 7 行输出几乎一致；只有 with-manager 多 6 行关闭日志
07：四个操作输出完全一致
```