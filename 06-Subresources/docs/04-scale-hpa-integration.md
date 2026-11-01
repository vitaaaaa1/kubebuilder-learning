# Scale 子资源：让 CRD 支持 HPA

本文档记录如何让自定义资源（CRD）支持 Kubernetes 的 HPA（Horizontal Pod Autoscaler），核心是启用 `/scale` 子资源。

---

## 1. 为什么需要 `/scale` 子资源？

### 1.1 问题场景

HPA 的工作原理：

```
HPA 控制器
    ↓
1. 读取 Deployment.spec.replicas（期望值）
2. 根据 CPU/内存指标计算新的 replicas
3. 写入 Deployment.spec.replicas
```

**但 HPA 只能操作标准资源（Deployment/StatefulSet）**，因为它：
- 知道去哪个字段读 `replicas`（`spec.replicas`）
- 知道去哪个字段读 `readyReplicas`（`status.readyReplicas`）
- 知道去哪个端点写入（`/scale` 子资源）

**如果你的 CRD 想被 HPA 控制，必须告诉 HPA：**
1. 我的 `replicas` 字段在哪里？
2. 我的 `readyReplicas` 字段在哪里？
3. 我的 `/scale` 端点是什么？

### 1.2 `/scale` vs `/status` 对比

| 维度 | `/status` | `/scale` |
|------|----------|---------|
| **用途** | 分离 Status 与 Spec | 让 HPA 能读写 replicas |
| **隔离范围** | 只允许写 `status` | 只允许读写 `replicas` + `selector` |
| **HPA 能用吗？** | ❌ 不能 | ✅ 能 |
| **RBAC 隔离** | `update status` | `update .../scale` |
| **API 端点** | `.../status` | `.../scale` |

---

## 2. 核心概念：Scale Spec/Status

Kubernetes 的 `/scale` 子资源返回的是一个 **`autoscaling/v1.Scale`** 对象：

```yaml
apiVersion: autoscaling/v1
kind: Scale
metadata:
  name: my-crd
  namespace: test
spec:
  replicas: 3          # HPA 读写这个字段
status:
  replicas: 3          # 当前 replicas
  selector: "app=my-crd"  # Label Selector（HPA 用来找到 Pod）
```

**你的 CRD 需要"翻译"自己的字段到这个标准格式。**

---

## 3. Marker 怎么写？

### 3.1 启用 `/scale` 子资源

```go
// +kubebuilder:subresource:scale:specPath=.spec.replicas,statusPath=.status.readyReplicas,selectorPath=.status.selector
type DemoApp struct {
    metav1.TypeMeta   `json:",inline"`
    metav1.ObjectMeta `json:"metadata,omitempty"`

    Spec   DemoAppSpec   `json:"spec,omitempty"`
    Status DemoAppStatus `json:"status,omitempty"`
}
```

**3 个路径参数：**

| 参数 | 含义 | 必填？ |
|------|------|-------|
| `specPath` | 你的 CRD 中 `replicas` 字段的路径 | ✅ 必填 |
| `statusPath` | 你的 CRD 中 `readyReplicas` 字段的路径 | ✅ 必填 |
| `selectorPath` | 你的 CRD 中 Label Selector 的路径 | ❌ 可选（HPA 需要） |

### 3.2 ⚠️ 常见坑

| 坑 | 现象 | 解决 |
|---|------|------|
| **路径写错** | HPA 报 `unable to fetch replicas` | 确认路径是 **JSON 路径**（如 `.spec.replicas`），不是 Go 字段路径 |
| **空行问题** | `controller-gen` 忽略 Marker | Marker 紧贴 struct，无空行/块注释 |
| **selector 为空** | HPA 创建成功但不工作 | `selectorPath` 必须指向一个 `string` 类型的字段（Label Selector 字符串） |

---

## 4. 生成的 CRD YAML 长什么样？

运行 `make manifests` 后，生成的 CRD 中会包含：

```yaml
spec:
  versions:
    - name: v1alpha1
      subresources:
        status: {}    # 如果也启用了 /status
        scale:
          specReplicasPath: .spec.replicas
          statusReplicasPath: .status.readyReplicas
          labelSelectorPath: .status.selector
```

**验证：**

```bash
kubectl get crd demoapps.demo.vitaaaaa.icu -o yaml | grep -A 5 subresources
```

应该看到上面的 `scale:` 块。

---

## 5. 代码怎么调？

### 5.1 Controller 读取/写入 Scale

```go
// 读取 scale
scale := &autoscalingv1.Scale{}
err := r.SubResource("scale").Get(ctx, &app, scale)
fmt.Printf("replicas: %d\n", scale.Spec.Replicas)

// 写入 scale
scale.Spec.Replicas = 5
err = r.SubResource("scale").Update(ctx, &app, client.SubResourceUpdateOptions{
    SubResourceBody: scale,
})
```

### 5.2 HPA 怎么用？

创建 HPA 指向你的 CRD：

```yaml
apiVersion: autoscaling/v2
kind: HorizontalPodAutoscaler
metadata:
  name: demoapp-hpa
  namespace: test
spec:
  scaleTargetRef:
    apiVersion: demo.vitaaaaa.icu/v1alpha1
    kind: DemoApp
    name: status-demo     # ← 指向你的 CRD 实例
  minReplicas: 1
  maxReplicas: 10
  metrics:
    - type: Resource
      resource:
        name: cpu
        target:
          type: Utilization
          averageUtilization: 50
```

**HPA 会自动：**
1. 读取 `DemoApp.spec.replicas`（当前值）
2. 根据 CPU 使用率计算新值
3. 写入 `DemoApp.spec.replicas`（通过 `/scale` 端点）

---

## 6. 完整实验流程

### 6.1 前置条件

1. 集群已安装 **metrics-server**（HPA 需要它获取 CPU/内存指标）
2. CRD 已启用 `/scale` 子资源

### 6.2 实验步骤

```bash
# 1. 创建 CRD
kubectl apply -f cmd/scale-subresource/crd.yaml

# 2. 创建 DemoApp 实例
kubectl apply -f cmd/scale-subresource/sample.yaml

# 3. 验证 /scale 端点
kubectl get --raw /apis/demo.vitaaaaa.icu/v1alpha1/namespaces/test/demoapps/status-demo/scale | jq

# 4. 创建 HPA
kubectl apply -f cmd/scale-subresource/hpa.yaml

# 5. 观察 HPA 状态
kubectl get hpa demoapp-hpa -n test -w

# 6. 手动调整 replicas（通过 /scale）
kubectl scale demoapp status-demo --replicas=5 -n test

# 7. 验证 replicas 变了
kubectl get demoapp status-demo -n test -o jsonpath='{.spec.replicas}'
```

### 6.3 验证 HPA 工作

```bash
# 查看 HPA 详情（应该看到当前/目标 CPU 使用率和 replicas）
kubectl describe hpa demoapp-hpa -n test

# 观察 DemoApp replicas 自动变化
kubectl get demoapp status-demo -n test -w
```

---

## 7. Controller 自动同步 replicas 到 status

当 HPA 修改了 `spec.replicas` 后，你的 Controller 应该：

```go
func (r *DemoAppReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
    var app DemoApp
    r.Get(ctx, req.NamespacedName, &app)

    // 实际逻辑：创建/更新 Pod，然后更新 status
    app.Status.ReadyReplicas = calculateReadyReplicas(&app)
    app.Status.Replicas = app.Spec.Replicas  // ← 同步 replicas 到 status
    r.Status().Update(ctx, &app)

    return ctrl.Result{}, nil
}
```

**为什么需要 `Status.Replicas = Spec.Replicas`？**

因为 `/scale` 子资源的 `statusReplicasPath` 指向 `.status.readyReplicas`，HPA 会读这个值确认缩放是否生效。

---

## 7. 深入理解：为什么需要 `/scale`？

### 7.1 kubectl scale 和 HPA 读取的是同一个接口吗？

**是的！完全同一个 API 端点。**

它们访问的是同一个接口，只是 **HTTP 方法（操作方向）不同**：

| 操作 | 方式 | 行为 | 对应 JSON |
|------|------|------|----------|
| **kubectl scale** | **写 (PATCH/PUT)** | 发送请求，修改副本数 | `{"spec": {"replicas": 4}}` |
| **HPA 读取** | **读 (GET)** | 发送请求，获取当前副本数 | 返回 `Scale` 对象中的 `spec.replicas` |

**共同访问的接口：**
```text
/apis/demo.vitaaaaa.icu/v1alpha1/namespaces/test/demoapps/scale-demo/scale
```

**数据流图解：**

```text
                [ /scale 接口 ]
               /               \
              /                 \
   [kubectl scale]        [HPA Controller]
   "我要改成 4 个!"        "现在有几个？"
         ↓                   ↓
   (PATCH /scale)       (GET /scale)
         ↓                   ↓
         \                   /
          \                 /
      [ API Server / CRD ]
             ↓
   更新/读取 .spec.replicas
```

**为什么这很重要？**
正因为它们都走同一个门，你只需要在 CRD 里定义**一次**路径映射：

```yaml
scale:
  specReplicasPath: .spec.replicas  # 定义好这个，两个工具都能自动工作
```

- 当你运行 `kubectl scale ... --replicas=4`，API Server 知道要把 `4` 填到 `.spec.replicas`。
- 当 HPA 问"现在有几个"时，API Server 知道去 `.spec.replicas` 拿数字回答它。

---

### 7.2 Kubernetes 的设计哲学："标准接口模式" (Adapter Pattern)

只要你的 CRD 实现了 `/scale` 接口（标准协议），它就瞬间获得了整个 Kubernetes 生态圈的认可。

**为什么这很厉害？**

这意味着 Kubernetes 的核心组件（如 HPA、kubectl、Cluster Autoscaler）**根本不需要认识你的 CRD**。

**没有 `/scale` 时（反面教材）：**
HPA 代码里必须写死每一种资源：
```go
// ❌ HPA 必须认识每一种资源
if kind == "Deployment" { ... }
else if kind == "StatefulSet" { ... }
else if kind == "MyCRD" { ... } // 每次出新 CRD，K8s 都要改代码？不可能
```

**有了 `/scale` 后（K8s 的真实实现）：**
HPA 代码变成了通用的：
```go
// ✅ HPA 只需要认识 /scale 接口
scale, _ := client.Get("/scale")  // 读取标准接口
currentReplicas = scale.Spec.Replicas

newReplicas := calculate(...)
client.Put("/scale", Scale{Replicas: newReplicas}) // 写入标准接口
```

**现实世界的比喻：**

| 概念 | 现实世界 | Kubernetes 对应 |
|------|---------|----------------|
| **CRD (你的 DemoApp)** | **灯泡**（任何牌子、形状） | 你的自定义工作负载 |
| **/scale 子资源** | **E27 标准螺口** | `autoscaling/v1.Scale` 协议 |
| **HPA / kubectl** | **墙壁开关** | 通用扩缩容工具 |

只要你的灯泡（CRD）装上了标准螺口（`/scale`），不管你是圆的、方的、还是 LED 的，墙壁开关（HPA）都能直接控制它，**不需要关心灯泡内部长什么样**。

**总结：**

| 概念 | 你的理解 | K8s 术语 |
|------|------|---------|
| **接口** | "调度"接口 | **扩缩容协议 (Scaling Protocol)** |
| **载体** | `/scale` | `autoscaling/v1.Scale` 对象 |
| **目的** | 统一管理 | **解耦 (Decoupling)**：让 HPA 不需要感知 CRD 的存在 |

所以，启用 `/scale` 不仅仅是加了个功能，而是让你的 CRD 从一个"孤立资源"变成了一个**符合 Kubernetes 工业标准的 Operator**。

---

## 8. 常见问题

### Q1: HPA 报 `unable to fetch resource`

**原因：** CRD 没有启用 `/scale` 子资源。

**解决：** 确认 Marker 正确，重新 `make manifests` + `kubectl apply`。

### Q2: HPA 报 `selector is required`

**原因：** `selectorPath` 未配置或为空。

**解决：** 在 CRD 的 `status` 中添加 `selector` 字段，并在 Marker 中配置 `selectorPath`。

### Q3: `kubectl scale` 不生效

**原因：** 用了 `r.Update()` 而不是 `r.SubResource("scale").Update()`。

**解决：** 直接写 `spec.replicas` 会触发完整资源的 Update，但如果 Controller 也在改 Spec 可能 409。推荐 Controller 也通过 `/scale` 读写。

### Q4: `/status` 和 `/scale` 能同时启用吗？

**可以！** Marker 写在一起：

```go
// +kubebuilder:subresource:status
// +kubebuilder:subresource:scale:specPath=.spec.replicas,statusPath=.status.readyReplicas,selectorPath=.status.selector
type DemoApp struct { ... }
```

---

## 9. 核心结论

| 维度 | 普通资源 | 启用 `/scale` 的 CRD |
|------|---------|---------------------|
| **HPA 能用？** | ✅ 能 | ✅ 能 |
| **`kubectl scale` 能用？** | ✅ 能 | ✅ 能 |
| **需要手动同步 replicas？** | ❌ 不需要 | ✅ Controller 需要同步 `Status.Replicas` |
| **RBAC 隔离** | `update .../scale` | `update .../scale` |

**最佳实践：**
- 如果你的 CRD 需要被 HPA 控制，必须启用 `/scale`
- 如果只需要 Status 隔离，用 `/status`
- 两个可以同时启用

---

*上一步：[Kubebuilder 开发实践](./03-kubebuilder-implementation.md)*
