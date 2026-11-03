# 01 · 三层测试的边界

写 Operator 测试最常见的失败不是"不会写测试"，而是**在错误的一层写测试**：
用 fake client 测级联删除，用 envtest 测纯函数，或者拿真实集群测分支逻辑。
结果要么是假绿，要么是慢到没人跑。

## 三层对照

| | 纯单元测试 | fake client | envtest |
|---|---|---|---|
| **跑起来需要什么** | 什么都不需要 | 什么都不需要 | etcd + kube-apiserver 二进制 |
| **单条耗时** | 微秒 | 毫秒 | 秒级（起停 3–6s，用例本身毫秒） |
| **有没有 apiserver** | 没有 | 没有（内存 tracker） | 有（真实进程） |
| **校验 OpenAPI schema** | 不涉及 | ❌ 不校验 | ✅ 校验 |
| **分配 UID / 时间戳** | 不涉及 | ❌ 不分配 | ✅ 分配 |
| **resourceVersion 乐观锁** | 不涉及 | ❌ 不实现真冲突 | ✅ 实现 |
| **默认值 / webhook** | 不涉及 | ❌ 不跑 | ✅ 跑（需装 webhook 配置） |
| **garbage collector** | 不涉及 | ❌ 没有 | ❌ 也没有 |
| **真跑容器** | 不涉及 | ❌ | ❌（无 kubelet） |
| **适合测什么** | 名字生成、patch 构造、条件判断 | Reconcile 的分支、错误处理、幂等逻辑 | ownerRef、status 子资源、CRD 校验、webhook、RV 语义 |
| **不适合测什么** | 任何 K8s 交互 | 一切"由 apiserver 保证"的行为 | 容器真实运行、GC、调度 |

## 四个必须记住的幻觉

**幻觉 1：fake 里测试通过 = 生产行为正确。**
fake 是一个 map，它接受你给的任何字段。schema 写错、required 漏写、默认值缺失，
fake 一律照单全收，envtest 才会 422。

**幻觉 2：ownerReference 写好了 = 级联删除没问题。**
ownerRef 只是 declaration。真正执行回收的是 kube-controller-manager 里的
garbage collector —— envtest 不跑它，fake 更没有。
所以级联删除只能在 kind/真实集群上验证。

**幻觉 3：`Update` 成功 = 数据是对的。**
如果更新的对象是从缓存里读出来的旧版本，apiserver 会返回 409 Conflict。
fake 不实现这个语义，所以"更新冲突"的分支在 fake 里永远测不到。

**幻觉 4：status 子资源在 fake 下行为一致。**
fake 对 `Status().Update` 的处理不区分主资源与子资源，可能连 spec 一起写回。
真集群里 status 更新不会碰 spec —— 这个差异会掩盖你 Reconcile 里的越权写入。

## 本章的选择

01 小节只做两件事：

1. 用 fake 把**幻觉**演出来（`fake_test.go`），让你以后不会再误用它
2. 用 envtest 把**真语义**钉住（`env_test.go`）：ownerRef 的 UID、RV 幂等、自愈、NotFound

## 什么时候该升级到 kind

- 要验证级联删除、finalizer 真正阻塞删除的时序
- 要验证 Pod 真的被创建/调度
- 要验证 webhook 的 TLS 与 `failurePolicy`
- 要验证多副本 leader election

这些是 12、13 章的事。

## 实测记录（08-Testing 第一次跑出来的结果）

| 预期 | 实际 |
|---|---|
| fake 下缺 UID 时 `SetControllerReference` 报错 | **不报错**，静默写空 UID，测试全绿 |
| 名字里有下划线会被拒 | fake 全收，envtest 报 `Invalid value` |
| envtest 会做级联删除 | 不会，没有 GC |

结论：**"我以为 fake 会……"是这一章唯一需要记住的句式** —— 每一条都要用测试去验，而不是猜。
