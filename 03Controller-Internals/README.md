# 03 Controller: 运行机制与 WorkQueue

本专题深入 Controller 的核心逻辑：它是如何把 API Server 的事件转化为代码中的 `Reconcile` 调用的。

## 1. 核心概念

1.  **For()**: 定义**主资源 (Primary Resource)**。主资源的创建/更新/删除会**直接**触发 Reconcile。
2.  **Owns()**: 定义**子资源 (Child Resource)**。通过 OwnerReference 关联。当子资源变化时，Controller 会找到其 Owner（主资源），并把 Owner 的 Key 放入队列。
3.  **Watches()**: 定义**外部资源 (External Resource)**。用于监听那些与主资源没有 Owner 关系，但逻辑上相关的资源。

---

## 2. For() 主资源与 req 的身份

在开始 `Owns()` 实验之前，我们必须先理解 `For()` 的深层含义。

### 💡 核心概念：For() 决定 `req` 的身份

`For()` 不仅仅定义了 Controller 要监听什么资源，它更决定了 **`Reconcile(ctx, req)` 函数中 `req` 参数到底代表谁**。

1.  **唯一的“主角”**：
    *   一个 Controller **只能有一个 `For()`**。它定义了该 Controller 的主要职责是调和哪种资源。
    *   如果你写 `.For(&corev1.ConfigMap{})`，那么这个 Controller 的核心任务就是确保 ConfigMap 的状态符合预期。

2.  **`req` 永远指向主资源**：
    *   **当 ConfigMap 自己发生变化时**：`req.Name` 是 ConfigMap 的名字。
    *   **当 Secret (通过 Owns 关联的子资源) 发生变化时**：Controller 会根据 Secret 的 `ownerReferences` 找到其 Owner (ConfigMap)，然后将 **ConfigMap 的名字** 放入 `req.Name`。
    *   **结论**：无论是因为什么原因触发的 Reconcile，你在代码里调用 `r.Get(ctx, req.NamespacedName, &cm)` 时，拿到的永远是 `For()` 里定义的那个资源实例。

3.  **如果换掉 For()**：
    *   如果你把代码改成 `.For(&corev1.Secret{})`，那么 `req.Name` 就会变成 Secret 的名字。此时如果你还尝试用 `r.Get` 去获取 ConfigMap，代码就会报错，因为它拿到的是 Secret 的标识。

---

## 3. Owns() 所有权绑定实验

### 实验目标
验证当子资源 (Secret) 发生变化时，Controller 能否通过 `OwnerReference` 自动找到其父资源 (ConfigMap) 并触发 Reconcile。

### 实验原理
*   **代码配置**: `.For(&corev1.ConfigMap{}).Owns(&corev1.Secret{})`
*   **触发流程**: Secret 变化 -> Informer 捕获 -> EventHandler 检查 `ownerReferences` -> 发现 Owner 是 ConfigMap -> 将 ConfigMap 的 Key 放入 WorkQueue -> 触发 Reconcile。

### 实验记录

#### 阶段 1: 手动关联验证（已完成）
*   **方法**: 手动创建 ConfigMap，获取其 UID，然后在创建 Secret 的 YAML 中显式填写 `ownerReferences` 指向该 UID。 `03Controller-Internals/example-resource/child-secret.yaml`
*   **结果**: 成功验证。当手动创建的 Secret 发生变化时，Controller 日志中打印了针对 ConfigMap 的 Reconcile 事件。
*   **结论**: `Owns()` 机制生效，Controller 能够识别并路由子资源事件。
*   **痛点**: 手动管理 UID 过于繁琐，不符合真实 Operator 的开发模式。

#### 阶段 2: 自动创建与关联（已完成）
*   **代码实现**:
    *   在 `Reconcile` 中获取 ConfigMap 后，定义一个 Secret 对象。
    *   使用 `ctrl.SetControllerReference(&cm, &secret, r.Scheme())` 自动将 ConfigMap 的 UID 注入到 Secret 的 `ownerReferences` 中。
    *   调用 `r.Create(ctx, &secret)` 创建。
*   **现象观察**:
    *   创建 ConfigMap 后，Controller 自动创建了 Secret。
    *   **关键发现**：日志中 `Reconcile triggered!` 打印了**两次**。
        1.  **第一次**：ConfigMap 创建 -> 触发 Reconcile -> 代码执行 `Create` 生成 Secret。
        2.  **第二次**：Secret 被创建 -> `Owns()` 机制捕获到 Secret 的变化 -> 检查 `ownerReferences` 发现属于 ConfigMap -> **再次触发** ConfigMap 的 Reconcile。
*   **核心结论**: `Owns()` 让 Controller 具备了"记忆力"和"自愈能力"。它不仅知道子资源是自己创建的，而且当子资源发生变化（或被删除）时，它会立即通过父资源（ConfigMap）的 Reconcile 重新介入处理。

### 💡 核心概念总结：Owner 与 OwnerReference

1.  **什么是 OwnerReference？**
    *   它是 Kubernetes 资源 `metadata` 中的一个列表。
    *   它声明："我是被某个资源（Owner）创建的"。
    *   **作用 1（垃圾回收）**: 当 Owner（如 ConfigMap）被删除时，K8s 会自动把所有带有其 OwnerReference 的子资源（如 Secret）一起删除。
    *   **作用 2（Controller 关联）**: Controller 通过 `Owns()` 监听子资源。当子资源变化时，Controller 读取 `ownerReferences`，找到 Owner 的名字，然后触发该 Owner 的 Reconcile。

2.  **为什么 Reconcile 触发了两次？**
    *   **第 1 次**: ConfigMap 变化 -> 触发 Reconcile -> 代码 `Create` 了 Secret。
    *   **第 2 次**: Secret 创建完成 -> `Owns()` 监听到 Secret 新增 -> 查 Owner 是 ConfigMap -> 再次触发 ConfigMap 的 Reconcile。
    *   **意义**: 这证明了 Controller 对子资源的生命周期完全掌控。

---

## 4. Watches() 外部资源监听

### 实验目标
验证修改无关的外部资源（如 `Deployment`）能否通过自定义映射逻辑（MapFunc），触发主资源（ConfigMap）的 Reconcile。

### 场景描述
`For` 只能定义"主角"（ConfigMap），`Owns` 只能处理"亲生孩子"（Secret）。
如果我们想监听一个**完全没有 Owner 关系的外部资源**（比如 Deployment 变了，我要去更新 ConfigMap 的版本号），该怎么办？

这就是 `Watches()` 的作用。

### 代码逻辑拆解

`Watches` 的核心在于**自定义映射函数 (MapFunc)**。我们可以把它理解为为 Controller 配了一个**翻译官**。

**代码结构：**
```go
Watches(
    &appsv1.Deployment{}, // 1. 安装监控：监听 Deployment
    handler.EnqueueRequestsFromMapFunc(func(ctx context.Context, o client.Object) []ctrl.Request {
        // 2. 翻译官 (MapFunc) 开始工作
        log.Info("🕵️ Detected a Deployment change!", "name", o.GetName())
        
        // 3. 返回指令：告诉 Controller 接下来该处理谁
        return []ctrl.Request{
            {NamespacedName: types.NamespacedName{
                Name:      "my-test-cm", // 强制指向 ConfigMap
                Namespace: "test",
            }},
        }
    }),
)
```

### 💡 核心概念总结：MapFunc 的作用

1.  **输入 (`o client.Object`)**：
    *   这是**发生变化的那个外部资源**（例如 Deployment）。
    *   如果 Deployment 叫 `nginx-deploy`，那么 `o.GetName()` 就是 `"nginx-deploy"`。

2.  **输出 (`[]ctrl.Request`)**：
    *   这是你给 Controller 下达的指令。
    *   你可以在这里**完全自定义**想让 Controller 去处理谁。
    *   你可以返回 `my-test-cm`，也可以返回 `your-test-cm`，甚至可以返回 10 个不同的 ConfigMap 让它们同时触发 Reconcile。

3.  **与 Owns 的区别**：
    *   **Owns() 是自动的**：它会自动查"爸爸"是谁（通过 OwnerReference），然后返回爸爸的名字。你不需要写代码。
    *   **Watches() 是手动的**：你想让 Controller 去处理谁，完全由你在 `return` 里定义。

### 结论
你操作的是 Deployment，但 Controller 却去处理了 ConfigMap。
这就是 `Watches` 的强大之处：**你可以将集群里任何资源的任何风吹草动，映射成你的业务逻辑。**

---

## 5. Watches 实验完整代码逻辑 (`cmd/Watches/main.go`)

本节记录了 `cmd/Watches/main.go` 中实现的完整业务逻辑与核心知识点。

### 5.1 Manager 初始化配置
*   **Scheme 注册**:
    *   `corev1.AddToScheme(scheme)`: 注册 `ConfigMap`, `Secret` 等核心资源。
    *   `appsv1.AddToScheme(scheme)`: **必须注册**。因为 `Watches` 监听了 `Deployment`，如果不注册，Manager 启动时无法识别该类型并报错。

### 5.2 Controller 事件监听链
```go
ctrl.NewControllerManagedBy(mgr).
    For(&corev1.ConfigMap{}).                     // 1. 监听主资源 ConfigMap
    Watches(                                      // 2. 监听外部资源 Deployment
        &appsv1.Deployment{},
        handler.EnqueueRequestsFromMapFunc(       // 3. 自定义映射逻辑 (翻译官)
            func(ctx context.Context, o client.Object) []ctrl.Request {
                // 无论哪个 Deployment 变了，我们都强制去调和名为 "my-test-cm" 的 ConfigMap
                return []ctrl.Request{
                    {NamespacedName: types.NamespacedName{
                        Name:      "my-test-cm", 
                        Namespace: "test",
                    }},
                }
            },
        ),
    ).
    Complete(&SimpleReconciler{Client: mgr.GetClient()})
```

### 5.3 Reconcile 业务逻辑 (自愈与同步)
当 `Reconcile` 被触发时，执行以下逻辑：

1.  **获取 ConfigMap**: 使用 `r.Get` 获取主资源实例。
2.  **查找 Secret**: 尝试获取 `{ConfigMapName}-child-secret`。
3.  **状态检查与执行**:
    *   **情况 A: Secret 不存在 (`IsNotFound`)**
        *   **动作**: 调用 `r.Create` 创建新 Secret。
        *   **关键点**: 使用 `ctrl.SetControllerReference` 建立 OwnerReference，实现级联删除和关联。
    *   **情况 B: Secret 已存在 (`else` 分支)**
        *   **意图**: 强制将 Secret 的数据同步为 ConfigMap 中的最新数据。
        *   **动作**: 调用 `r.Update` 更新 Secret。
        *   **⚠️ 重要机制 (ResourceVersion)**:
            *   **代码现状**: 代码中尝试 `r.Update` 一个**新创建的 Secret 对象** (`secretToUpdate := &corev1.Secret{...}`)。
            *   **运行结果**: **会失败！** 报错 `OptimisticLockError`。
            *   **原因**: Kubernetes 的 `Update` 操作是**强一致性**的，它要求传入的对象必须包含从 API Server `Get` 到的 `ResourceVersion` (RV)。新对象没有 RV，K8s 拒绝更新。
            *   **正确做法**:
                1.  先 `r.Get(ctx, key, &existingSecret)`。
                2.  修改 `existingSecret.Data`。
                3.  再 `r.Update(ctx, &existingSecret)`。
                *   *或者使用 `r.Patch` (局部更新)，不需要 RV。*

### 5.4 实验验证现象
1.  **启动**: 观察日志是否成功注册 `Deployment` 的 EventSource。
2.  **创建 Deployment**: 
    *   日志打印 `🕵️ Detected a Deployment change!`。
    *   触发 `my-test-cm` ConfigMap 的 Reconcile。
    *   如果 Secret 不存在，则创建；如果存在，尝试更新（会因 RV 缺失报错）。

---

## 5. Watches 实验完整代码逻辑 (`cmd/Watches/main.go`)

本节记录 `cmd/Watches/main.go` 中实现的完整业务逻辑。

### 5.1 Manager 初始化配置
*   **Scheme 注册**: 同时注册了 `corev1` (ConfigMap, Secret) 和 `appsv1` (Deployment)。
    *   *注意*：如果不注册 `appsv1`，Controller 启动时会因无法识别 Deployment 类型而报错。

### 5.2 Controller 事件监听链
```go
ctrl.NewControllerManagedBy(mgr).
    For(&corev1.ConfigMap{}).                     // 1. 监听 ConfigMap 变化
    Watches(                                      // 2. 监听 Deployment 变化
        &appsv1.Deployment{},
        handler.EnqueueRequestsFromMapFunc(
            func(ctx context.Context, o client.Object) []ctrl.Request {
                // 3. MapFunc: 将 Deployment 事件映射为特定 ConfigMap 的请求
                return []ctrl.Request{
                    {NamespacedName: types.NamespacedName{
                        Name:      "my-test-cm", // 硬编码的目标 ConfigMap 名称
                        Namespace: "test",
                    }},
                }
            },
        ),
    ).
    Complete(&SimpleReconciler{...})
```

### 5.3 Reconcile 业务逻辑
`03Controller-Internals/cmd/Watches/main.go`
当 `Reconcile` 被触发（无论是 ConfigMap 变化还是 Deployment 变化导致），代码执行以下流程：

1.  **获取主资源**: 根据 `req` 从 API Server (Cache) 获取对应的 `ConfigMap`。
2.  **查找子资源**: 尝试获取名为 `{ConfigMap.Name}-child-secret` 的 Secret。
3.  **分支处理**:
    *   **如果 Secret 不存在 (`IsNotFound`)**:
        *   创建一个新的 Secret。
        *   **关键操作**: 使用 `ctrl.SetControllerReference(&cm, &secret, r.Scheme())` 设置 OwnerReference。
        *   调用 `r.Create` 保存到集群。
    *   **如果 Secret 已存在 (`else` 分支)**:
        *   构造一个新的 Secret 对象（包含最新的 ConfigMap 数据）。
        *   调用 `r.Update` **强制更新**该 Secret。
        *   *逻辑含义*: 只要 ConfigMap 变了，或者 Deployment 变了（触发了映射），Controller 都会强制把 Secret 的数据同步为最新状态。

### 5.4 实验验证现象
1.  **启动**: 需确保 Scheme 注册了 `appsv1`，否则启动报错。
2.  **创建 ConfigMap**: 触发 Reconcile -> 自动创建 Child Secret。
3.  **修改 Child Secret**: 触发 `Owns` (虽然代码里没显式写 `Owns`，但如果配置了会触发) -> 这里代码中没有 `Owns(&corev1.Secret{})`，所以单纯修改 Secret 不会触发。
4.  **修改 Deployment**: 触发 `Watches` -> 打印 `🕵️ Detected a Deployment change!` -> 调用 `Reconcile` -> **强制更新 Secret 的数据**。

---

### 💡 核心避坑指南：Update 必须基于 Get

在 Kubernetes 中更新资源时，**绝对不能**自己 `new` 一个对象去 `Update`（除非使用 `Patch`）。

**错误写法**：
```go
newObj := &v1.ConfigMap{ObjectMeta: ..., Data: ...} // 自己创建的，没有 ResourceVersion
r.Update(ctx, newObj) // ❌ 报错！
```

**正确写法**：
```go
// 1. 先 Get
oldObj := &v1.ConfigMap{}
r.Get(ctx, key, oldObj)

// 2. 修改 Get 返回的对象
oldObj.Data["key"] = "value"

// 3. Update
r.Update(ctx, oldObj) // ✅ 成功
```

**为什么？**
1.  **ResourceVersion (RV)**：K8s 每个资源都有 RV（版本号）。`Update` 是**全量替换**，K8s 要求你提交的对象的 RV 必须与服务器一致，以确保你没有覆盖别人的修改（乐观锁）。
2.  **New 的对象没有 RV**：自己 `new` 的对象 RV 为空，K8s 会拒绝更新。
3.  **例外 (Patch)**：`Patch` 是局部更新（类似指令“把 A 改成 B"），不需要全量对象，所以不需要 Get。
