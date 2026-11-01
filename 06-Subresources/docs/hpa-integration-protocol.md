# HPA 如何管理 CRD：完整协议解析

本文档解析 HPA（Horizontal Pod Autoscaler）如何关联到 Deployment 或自定义 CRD，以及 CRD 需要满足哪些条件才能被 HPA 自动扩缩容。

---

## 1. HPA 的"关联"机制

### 1.1 用户声明目标资源

HPA 通过 `scaleTargetRef` 声明要管理哪个资源：

```yaml
apiVersion: autoscaling/v2
kind: HorizontalPodAutoscaler
spec:
  scaleTargetRef:
    apiVersion: demo.vitaaaaa.icu/v1alpha1   # ← API 版本
    kind: ScalableApp                         # ← 资源类型
    name: my-scalable-app                     # ← 资源名称
```

### 1.2 HPA 发现资源的过程

```
HPA Controller 启动
      ↓
读取 scaleTargetRef: {apiVersion, kind, name}
      ↓
发现这个资源在集群中是否存在
      ↓
检查该资源是否有 /scale 子资源
      ↓
有 → 开始工作
没有 → 报错 "the HPA controller was unable to get the target's current scale"
```

---

## 2. HPA 从 CRD 读取/写入的信息

### 2.1 完整数据流

```
HPA Controller
      ↓ 每 15 秒轮询一次
[1] GET /scale 端点
      ↓ 获取 3 个关键信息
      - spec.replicas（当前副本数）
      - status.replicas（实际就绪数）
      - status.selector（Pod 选择器）
      ↓
[2] 根据 selector 向 Metrics Server 请求
      ↓ "这个 selector 下的 Pod CPU/内存是多少？"
      ↓ 获取平均 CPU 使用率（如 60%）
      ↓
[3] 计算目标副本数
      ↓ 公式：desired = ceil(current × currentUsage / targetUsage)
      ↓ 例：desired = ceil(2 × 60/50) = 3
      ↓
[4] PATCH /scale 端点
      ↓ 写入新的 replicas: 3
      ↓
CRD 的 .spec.replicas 被修改为 3
```

### 2.2 HPA 需要的 3 个字段

| 字段 | 用途 | 对应 CRD 路径 | 必填？ |
|------|------|--------------|-------|
| **期望副本数** | HPA 读取当前值，写入新值 | `specReplicasPath` | ✅ 必填 |
| **实际就绪数** | HPA 确认缩放是否生效 | `statusReplicasPath` | ✅ 必填 |
| **Label Selector** | HPA 找到 Pod 采集指标 | `labelSelectorPath` | ✅ 必填 |

---

## 3. CRD 需要实现的接口

### 3.1 最低要求：启用 `/scale` 子资源

```yaml
# CRD 定义中必须包含
subresources:
  scale:
    specReplicasPath: .spec.replicas
    statusReplicasPath: .status.readyReplicas
    labelSelectorPath: .status.selector
```

### 3.2 你的 CRD 必须满足的条件

| 条件 | 说明 | 不满足的后果 |
|------|------|------------|
| ✅ 有 `/scale` 子资源 | HPA 通过 `/scale` 端点读写 | 报错：`unable to get the target's current scale` |
| ✅ `specReplicasPath` 指向 `int32` 字段 | HPA 读/写副本数 | 报错：`invalid type for replicas` |
| ✅ `statusReplicasPath` 指向 `int32` 字段 | HPA 确认缩放结果 | HPA 显示 `unknown`  replicas |
| ✅ `labelSelectorPath` 指向 `string` 字段 | HPA 找到 Pod 采集指标 | HPA 报 `missing selector` |
| ✅ Pod 有 `resources.requests.cpu` | HPA 计算使用率 | HPA 报 `missing request for cpu` |
| ✅ `metrics-server` 已安装 | 提供 CPU/内存指标 | HPA 报 `unable to fetch metrics` |

---

## 4. 标准资源（Deployment）vs 自定义资源（CRD）

### 4.1 Deployment 为什么开箱即用？

Deployment 是 Kubernetes 内置资源，它在 API Server 注册时就**已经定义好了** `/scale` 子资源：

```yaml
# Deployment 的内置 /scale 定义（不需要用户配置）
subresources:
  scale:
    specReplicasPath: .spec.replicas
    statusReplicasPath: .status.replicas
    labelSelectorPath: .spec.selector.matchLabels  # 注意：从 spec 读取
```

**你不需要做任何事**，Deployment 天生支持 HPA。

### 4.2 CRD 为什么需要手动配置？

CRD 是你自定义的资源，API Server 不知道：
- 哪个字段代表 "replicas"？
- 哪个字段代表 "readyReplicas"？
- 哪个字段代表 "selector"？

所以**你必须在 CRD 定义中显式声明这些映射关系**。

---

## 5. 一个完整的 HPA + CRD 集成示例

### 5.1 CRD 定义

```yaml
apiVersion: apiextensions.k8s.io/v1
kind: CustomResourceDefinition
metadata:
  name: scalableapps.demo.vitaaaaa.icu
spec:
  group: demo.vitaaaaa.icu
  names:
    kind: ScalableApp
    plural: scalableapps
  scope: Namespaced
  versions:
    - name: v1alpha1
      schema:
        openAPIV3Schema:
          type: object
          properties:
            spec:
              type: object
              properties:
                replicas: { type: integer }      # ← HPA 读写这个
                image: { type: string }
            status:
              type: object
              properties:
                readyReplicas: { type: integer } # ← HPA 读取确认
                selector: { type: string }       # ← HPA 用来找 Pod
      subresources:
        scale:
          specReplicasPath: .spec.replicas
          statusReplicasPath: .status.readyReplicas
          labelSelectorPath: .status.selector
```

### 5.2 用户的 CRD 实例

```yaml
apiVersion: demo.vitaaaaa.icu/v1alpha1
kind: ScalableApp
metadata:
  name: my-app
spec:
  replicas: 2
  image: nginx:1.25
```

### 5.3 你的 Controller 的职责

你的 Controller 需要：

1. **创建 Deployment/Pod**，且 Pod 必须带有和 `status.selector` 匹配的 Label：
   ```yaml
   metadata:
     labels:
       app: my-app  # ← 必须匹配 status.selector
   ```

2. **同步 Status**：
   ```go
   app.Status.ReadyReplicas = deployment.Status.ReadyReplicas
   app.Status.Selector = "app=" + app.Name
   r.Status().Update(ctx, &app)
   ```

### 5.4 HPA 配置

```yaml
apiVersion: autoscaling/v2
kind: HorizontalPodAutoscaler
spec:
  scaleTargetRef:
    apiVersion: demo.vitaaaaa.icu/v1alpha1
    kind: ScalableApp
    name: my-app
  minReplicas: 1
  maxReplicas: 5
  metrics:
    - type: Resource
      resource:
        name: cpu
        target:
          averageUtilization: 50
```

---

## 6. 常见问题

### Q1: HPA 报 `the HPA controller was unable to get the target's current scale`

**原因：** CRD 没有启用 `/scale` 子资源。
**解决：** 添加 `subresources.scale` 配置，重新 `kubectl apply -f crd.yaml`。

### Q2: HPA 报 `missing request for cpu`

**原因：** Pod 没有声明 `resources.requests.cpu`。
**解决：** 在 Container 定义中添加：
```yaml
resources:
  requests:
    cpu: 100m
```

### Q3: HPA 显示 `cpu: <unknown>`

**原因 1：** `metrics-server` 未安装。
**原因 2：** Pod 的 Label 和 `status.selector` 不匹配。
**解决：** 确保 Pod 带有正确的 Label，且 `status.selector` 与之匹配。

### Q4: HPA 显示 `TARGETS: 0%/50%` 但 replicas 不变

**原因：** 实际 CPU 使用率低于目标（50%），HPA 认为不需要扩容。
**这是正常行为**。如果想测试扩容，可以：
1. 降低 HPA 目标（如 10%）
2. 或在 Pod 里制造 CPU 负载（如 `stress --cpu 1`）

---

## 7. 总结：CRD 被 HPA 管理的 Checklist

- [ ] CRD 启用了 `/scale` 子资源
- [ ] `specReplicasPath` 指向一个 `int32` 字段
- [ ] `statusReplicasPath` 指向一个 `int32` 字段
- [ ] `labelSelectorPath` 指向一个 `string` 字段
- [ ] Controller 创建 Pod 时带有匹配的 Label
- [ ] Controller 同步 `status.readyReplicas` 和 `status.selector`
- [ ] Pod 声明了 `resources.requests.cpu`
- [ ] 集群安装了 `metrics-server`

**满足以上 8 项，你的 CRD 就能被 HPA 完美管理。**
