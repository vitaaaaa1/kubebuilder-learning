# Manager Options (ctrl.Options) 详解

本文档详细记录了 `sigs.k8s.io/controller-runtime` 中 Manager 的核心配置项。

## 基础配置

| 字段 | 类型 | 默认值 | 说明 |
| :--- | :--- | :--- | :--- |
| **Scheme** | `*runtime.Scheme` | client-go 默认 scheme | **必传**。定义资源类型（GVK）与 Go 结构体的映射关系。 |
| **MapperProvider** | `func(c *rest.Config) (meta.RESTMapper, error)` | 默认 RESTMapper | 用于将 Go 类型映射到 K8s API 的 REST 映射器。通常无需修改。 |
| **Logger** | `logr.Logger` | 全局 `log.Log` | 自定义 Manager 的日志输出。 |
| **BaseContext** | `BaseContextFunc` | `context.Background` | 为所有 Runnable 提供初始 Context。 |

---

## 缓存配置 (Cache)

缓存决定了 Manager 如何在本地内存中同步集群数据。

```go
Cache: cache.Options{
    // 只缓存特定命名空间的资源（大幅降低内存占用）
    DefaultNamespaces: map[string]cache.Config{
        "test": {},
    },
    // 还可以设置同步超时、禁用缓存等
},
```

| 字段 | 类型 | 说明 |
| :--- | :--- | :--- |
| **Cache** | `cache.Options` | 配置默认的 Shared Informer 缓存。默认会 Watch 所有 Namespace 的所有被请求资源。 |
| **NewCache** | `cache.NewCacheFunc` | **底层 API**。自定义缓存创建函数，仅在你非常了解机制时使用。 |

---

## 客户端配置 (Client)

| 字段 | 类型 | 说明 |
| :--- | :--- | :--- |
| **Client** | `client.Options` | 配置 Client 行为。默认情况下：**读操作走缓存，写操作直接请求 API Server**。 |
| **NewClient** | `client.NewClientFunc` | **底层 API**。自定义 Client 创建函数。 |

---

## 领导选举 (Leader Election)

用于防止多副本 Controller 重复执行任务。只有抢到锁的副本才会启动 Controller。

```go
LeaderElection:          true,
LeaderElectionID:        "my-controller-lock",
LeaderElectionNamespace: "kube-system",
```

| 字段 | 类型 | 默认值 | 说明 |
| :--- | :--- | :--- | :--- |
| **LeaderElection** | `bool` | `false` | 是否开启领导选举。生产环境强烈建议开启。 |
| **LeaderElectionResourceLock** | `string` | `"leases"` | 选举锁的资源类型。从 v0.12 起默认为 `leases`。 |
| **LeaderElectionID** | `string` | - | 锁的资源名称。不同 Controller 应使用不同 ID。 |
| **LeaderElectionNamespace** | `string` | 运行 Pod 的 Namespace | 锁资源创建的 Namespace。 |
| **LeaseDuration** | `*time.Duration` | `15s` | 非领导者等待强制获取领导权的时长。 |
| **RenewDeadline** | `*time.Duration` | `10s` | 领导者重试刷新领导权的时长。 |
| **RetryPeriod** | `*time.Duration` | `2s` | 选举动作之间的重试间隔。 |
| **LeaderElectionReleaseOnCancel** | `bool` | `false` | 管理器停止时是否主动释放锁（加速新领导者选举）。 |
| **LeaderElectionLabels** | `map[string]string` | - | 为选举 API 调用添加自定义标签。 |
| **LeaderElectionResourceLockInterface** | `resourcelock.Interface` | - | 提供自定义资源锁接口（如跨集群 MultiLock）。 |

---

## 监控与健康检查

| 字段 | 类型 | 默认值 | 说明 |
| :--- | :--- | :--- | :--- |
| **Metrics** | `metricsserver.Options` | - | Prometheus 指标服务器配置。 |
| **Metrics.BindAddress** | `string` | `:8080` | 指标暴露地址。设为 `"0"` 禁用。 |
| **HealthProbeBindAddress** | `string` | `:8081` | 健康检查探针绑定地址。设为 `"0"` 禁用。 |
| **ReadinessEndpointName** | `string` | `"readyz"` | 就绪检查端点名。 |
| **LivenessEndpointName** | `string` | `"healthz"` | 存活检查端点名。 |
| **PprofBindAddress** | `string` | 禁用 | pprof 性能分析服务地址。**注意：包含敏感信息，勿公开暴露**。 |

---

## Webhook 服务器

| 字段 | 类型 | 说明 |
| :--- | :--- | :--- |
| **WebhookServer** | `webhook.Server` | 自定义 Webhook 服务器。默认由 Manager 自动创建。 |

---

## 控制器全局配置

| 字段 | 类型 | 说明 |
| :--- | :--- | :--- |
| **Controller** | `config.Controller` | 所有注册在该 Manager 下的 Controller 的全局配置（如最大并发数、重试间隔等）。 |

---

## 优雅关闭

| 字段 | 类型 | 默认值 | 说明 |
| :--- | :--- | :--- | :--- |
| **GracefulShutdownTimeout** | `*time.Duration` | - | Manager 停止前等待 Runnable 完成的时长。`0` = 禁用，负数 = 无限等待。 |

---

## 已废弃

| 字段 | 说明 |
| :--- | :--- |
| **EventBroadcaster** | 已废弃。可能导致 goroutine 泄漏。 |

---

## 典型配置示例

```go
import (
    "time"
    "sigs.k8s.io/controller-runtime/pkg/cache"
    metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"
)

mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), ctrl.Options{
    Scheme: scheme,
    
    // 1. 领导选举（防止多副本冲突）
    LeaderElection:          true,
    LeaderElectionID:        "my-operator-lock",
    LeaderElectionNamespace: "kube-system",
    
    // 2. 限制缓存范围（节省内存）
    Cache: cache.Options{
        DefaultNamespaces: map[string]cache.Config{
            "production": {},
            "staging":    {},
        },
    },
    
    // 3. 监控和探针
    Metrics: metricsserver.Options{
        BindAddress: ":8080",
    },
    HealthProbeBindAddress: ":8081",
    
    // 4. 优雅关闭
    GracefulShutdownTimeout: ptr.To(30 * time.Second),
})
```
