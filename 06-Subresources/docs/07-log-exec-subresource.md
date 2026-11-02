# /log 和 /exec 子资源：为什么 K8s 把它们设计为子资源？

本文档解析 Kubernetes 为什么将流式日志（/log）和远程命令（/exec）设计为子资源，而不是主资源的一部分。

---

## 1. 核心问题：子资源到底是什么？

### 1.1 主资源 vs 子资源

```
主资源 = 对象的"期望状态"（Spec）+ "现实状态"（Status）
子资源 = 对对象的某个特定"操作"或"视图"
```

### 1.2 为什么需要子资源？

**因为有些操作不属于"修改期望状态"。**

---

## 2. /log 子资源

### 2.1 如果 /log 不是子资源会怎样？

假设 Pod 的日志是 Pod 对象的一个字段：

```yaml
apiVersion: v1
kind: Pod
metadata:
  name: my-pod
spec:
  containers: [...]
status:
  phase: Running
  # ❌ 如果日志在这里...
  log: "2026-11-01 12:00:00 INFO Starting app...\n2026-11-01 12:00:01 INFO ..."
```

**问题：**

| 问题 | 说明 |
|------|------|
| **数据爆炸** | 日志持续增长，Pod 对象会变成 MB/GB 级 |
| **etcd 压力** | etcd 是 KV 存储，不适合存储大文本流 |
| **并发冲突** | 多人同时读取时，RV 不断变化 |
| **无法流式输出** | `kubectl logs -f` 需要实时流，不是"读取字段" |

### 2.2 作为子资源的 /log

```bash
# /log 是一个独立的 API 端点
GET /api/v1/namespaces/default/pods/my-pod/log
```

**返回值不是 Pod 对象，而是一个** **HTTP 响应流**。

**为什么这是正确的？**

| 特性 | 说明 |
|------|------|
| **不修改对象** | 读取日志不会改变 Pod 的 RV |
| **流式传输** | 可以实时输出（`-f` 参数） |
| **不经过 etcd** | 直接从 Node 上的 kubelet 读取 |
| **独立权限** | 可以单独授权 `get pods/log` |

---

## 3. /exec 子资源

### 3.1 如果 /exec 不是子资源会怎样？

假设你想在 Pod 里执行命令，需要通过修改 Spec 来实现：

```yaml
# ❌ 荒谬的设计
apiVersion: v1
kind: Pod
spec:
  containers: [...]
  # 通过修改 spec 来"请求"执行命令？
  execRequest:
    command: ["ls", "-la"]
```

**问题：**
- 这是一个"操作"，不是"期望状态"
- 执行结果是临时的，不应该持久化到 etcd
- 需要双向通信（stdin/stdout/stderr）

### 3.2 作为子资源的 /exec

```bash
# /exec 是一个独立的 API 端点
POST /api/v1/namespaces/default/pods/my-pod/exec?command=ls&command=-la
```

**返回值不是 Pod 对象，而是一个** **WebSocket/SPDY 连接**。

**为什么这是正确的？**

| 特性 | 说明 |
|------|------|
| **不修改对象** | 执行命令不会改变 Pod 的 Spec/Status |
| **双向通信** | 支持 stdin/stdout/stderr 流 |
| **临时性** | 执行完就消失，不持久化 |
| **独立权限** | 可以单独授权 `create pods/exec` |

---

## 4. 核心设计哲学

### 4.1 子资源的本质

子资源 = **"动作"（Action）**，不是 **"状态"（State）**。

| 操作 | 类型 | 是否子资源？ | 为什么？ |
|------|------|------------|---------|
| 修改 replicas | 修改期望状态 | ❌ 不是（是 Spec 的一部分） |
| 读取日志 | 读取动态数据 | ✅ 是（/log） |
| 执行命令 | 执行临时操作 | ✅ 是（/exec） |
| 驱逐 Pod | 执行操作 | ✅ 是（/eviction） |
| 绑定 Pod 到 Node | 调度操作 | ✅ 是（/binding） |

### 4.2 为什么 CRD 不能有这些？

因为 CRD 是**声明式 API**，它的设计目标是：

```
用户声明期望（Spec） → Controller 同步到现实（Status）
```

**CRD 不支持"动作"类型的操作。** 如果你想让 CRD 支持"暂停"、"重启"、"审批"——你需要用其他模式（Annotation、独立 CRD、自定义 API Server）来模拟。

---

## 5. /log 和 /exec 的权限

### 5.1 RBAC 分离

```yaml
# 只能读 Pod，不能 exec
- apiGroups: [""]
  resources: ["pods"]
  verbs: ["get", "list"]

# 能执行命令（危险权限！）
- apiGroups: [""]
  resources: ["pods/exec"]
  verbs: ["create"]

# 能读日志
- apiGroups: [""]
  resources: ["pods/log"]
  verbs: ["get"]
```

### 5.2 为什么权限是分开的？

| 权限 | 风险 | 说明 |
|------|------|------|
| `get pods` | 低 | 只读元数据 |
| `get pods/log` | 中 | 可能包含敏感信息（密钥、Token） |
| `create pods/exec` | **高** | 等于在集群里执行任意命令 |

**生产环境中，运维人员可能有 `get pods` 权限，但不应该有 `pods/exec` 权限。**

---

## 6. 代码层面：client-go 如何调用子资源？

### 6.1 读取日志

```go
// 等价于 GET /api/v1/namespaces/{ns}/pods/{name}/log
req := clientset.CoreV1().Pods(ns).GetLogs(name, &corev1.PodLogOptions{
    Container: "app",
    Follow:    true,  // 流式输出
})
stream, _ := req.Stream(ctx)
```

### 6.2 执行命令

```go
// 等价于 POST /api/v1/namespaces/{ns}/pods/{name}/exec
req := clientset.CoreV1().RESTClient().
    Post().
    Resource("pods").
    Name(name).
    Namespace(ns).
    SubResource("exec").
    VersionedParams(&corev1.PodExecOptions{
        Command: []string{"ls", "-la"},
        Stdin:   true,
        Stdout:  true,
        Stderr:  true,
        TTY:     true,
    }, scheme.ParameterCodec)

exec, _ := remotecommand.NewSPDYExecutor(cfg, "POST", req.URL())
exec.StreamWithContext(ctx, remotecommand.StreamOptions{
    Stdin:  stdin,
    Stdout: stdout,
    Stderr: stderr,
    Tty:    true,
})
```

---

## 7. 总结

### 7.1 为什么 /log 和 /exec 是子资源？

| 原因 | 说明 |
|------|------|
| **不属于 Spec** | 它们不是"期望状态"，而是"动作"或"视图" |
| **不经过 etcd** | 数据直接从 Node/kubelet 读取，不存储 |
| **流式传输** | 需要实时流，不是"读取字段" |
| **独立权限** | 可以单独授权，实现最小权限 |
| **不改变 RV** | 读取日志/执行命令不修改对象的 ResourceVersion |

### 7.2 对比 CRD

| 维度 | Pod（内置资源） | CRD |
|------|---------------|-----|
| 子资源 | /log, /exec, /eviction, /binding, /status, /ephemeralcontainers | /status, /scale |
| 能自定义？ | ❌（硬编码在 K8s 源码中） | ❌（API Server 限制） |
| 模拟方案 | - | Annotation、独立 CRD、自定义 API Server |

**K8s 把 /log 和 /exec 设计为子资源，是因为它们是"动作"而非"状态"。CRD 不能有这些，是因为 CRD 是纯声明式 API。**
