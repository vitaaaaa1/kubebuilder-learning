# 10-multi-controller · 一个 Manager 挂多个 Controller

## 结论

**可以，而且这是标准做法。**

一个 Manager 上可以挂任意多个 Controller，每个 Controller：

- 有自己的名字（`configmap-controller` / `secret-controller`）
- 有自己的 workqueue 和 worker
- 有自己的 Reconciler
- 但**共享**同一个 cache、client、连接池

## 代码

```go
// 控制器 1：监听 ConfigMap
ctrl.NewControllerManagedBy(mgr).
	Named("configmap-controller").
	For(&corev1.ConfigMap{}).
	Complete(&reconciler{kind: "ConfigMap"})

// 控制器 2：监听 Secret
ctrl.NewControllerManagedBy(mgr).
	Named("secret-controller").
	For(&corev1.Secret{}).
	Complete(&reconciler{kind: "Secret"})
```

## 实测日志

启动阶段：

```text
INFO 已挂载 2 个 Controller
INFO Starting EventSource {"controller": "configmap-controller", "source": "kind source: *v1.ConfigMap"}
INFO Starting EventSource {"controller": "secret-controller",    "source": "kind source: *v1.Secret"}
INFO Starting Controller  {"controller": "configmap-controller"}
INFO Starting workers     {"controller": "configmap-controller", "worker count": 1}
INFO Starting Controller  {"controller": "secret-controller"}
INFO Starting workers     {"controller": "secret-controller", "worker count": 1}
```

事件阶段（各管各的）：

```text
kubectl create configmap demo-cm  →  Reconcile {"controller":"configmap-controller","request":"test/demo-cm"}
kubectl create secret demo-secret →  Reconcile {"controller":"secret-controller","request":"test/demo-secret"}
```

**ConfigMap 的变化不会触发 Secret 控制器，反之亦然。**

## 内部结构

```text
Manager
 ├── Cache（共享）
 │     ├── ConfigMap informer
 │     └── Secret informer
 ├── Client（共享）
 └── LeaderElection 组
       ├── configmap-controller
       │     ├── Queue
       │     ├── Worker ×1
       │     └── Source(kind=ConfigMap)
       └── secret-controller
             ├── Queue
             ├── Worker ×1
             └── Source(kind=Secret)
```

## 两种“监听多个资源”的方式

| | 多个 Controller | 一个 Controller + Owns/Watches |
|---|---|---|
| 队列 | 每个资源一个 | 共用一个 |
| Reconciler | 每个资源一个 | 一个 |
| 重试/并发 | 互相独立 | 共享配置 |
| 适用场景 | 资源之间**没有从属关系** | 主资源 + 从属资源（如 Deployment → ReplicaSet） |
| 代码隔离 | 好 | 差（一个 Reconciler 里多种分支） |

**没有从属关系就开多个 Controller；有 owner 关系就用 Owns。**

## 注意事项

| 项 | 说明 |
|---|---|
| 名字必须唯一 | 重名直接启动失败（`controller with name ... already exists`） |
| Reconciler 实例 | 每个 Controller 传自己的，不要共用可变状态 |
| 缓存共享 | 两个 Controller 的 informer 都在同一个 cache 里，只起一次 |
| 并发 | 两个 Controller 的 worker 并行跑，互不阻塞 |

## 运行

```bash
cd 02Manager/10-multi-controller
go run .

# 另开终端
kubectl create configmap demo-cm -n test --from-literal=a=1
kubectl create secret generic demo-secret -n test --from-literal=b=2
kubectl delete configmap demo-cm -n test
kubectl delete secret demo-secret -n test
```

## 一句话总结

> **一个 Manager 可以挂多个 Controller；它们共享缓存和连接，但各自拥有独立的队列、worker 和 Reconciler。**