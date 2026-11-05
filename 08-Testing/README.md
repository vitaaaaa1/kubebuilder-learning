# 08-Testing：从"没有一行测试"到 envtest 跑通

本章解决一个具体问题：**你的 01–07 章全是手测，改一行 Reconcile 就得重新搭集群。**
从这里开始，所有章节的验证方式都变成 `go test`。

## 小节路线

| 小节 | 主题 | 状态 |
|---|---|---|
| 01 | `fake client 会骗你` + 第一个 envtest | ✅ 本目录 |
| 02 | Manager + 缓存：`GetClient` 与 `GetAPIReader` 的差别、Eventually 等待 | 待写 |
| 03 | 装 CRD 进 envtest：`CRDDirectoryPaths`、status 子资源断言 | 待写 |
| 04 | Webhook 的测试：`WebhookInstallOptions`、TLS 证书 | 待写 |
| 05 | 表驱动 + 覆盖率 + CI 缓存 envtest 二进制 | 待写 |

## 快速开始

```bash
cd 08-Testing/src/00-get-branch-envtest

make setup        # 一次性：装 setup-envtest + 下载 etcd / kube-apiserver（约 165MB）
make test-simple  # 每次：跑测试
```

`cmd/` 下的旧示例没有 Makefile 入口，需要时直接 `go test`：

```bash
# fake 版，不需要任何二进制
go test ./cmd/get-branch -v

# 真 apiserver 版，需要 KUBEBUILDER_ASSETS
KUBEBUILDER_ASSETS="$(src/bin/setup-envtest use 1.31.x -p path)" \
  go test ./cmd/envtest-basic -v
```

## 目录

```text
08-Testing/
├── src/                              # ⭐ 三个递进的沙盒（从这里开始）
│   ├── README.md                     # 00 与 01 的定位对照 + 踩坑清单
│   ├── 00-get-branch-envtest/        #   最小调用：一条测试怎么跑起来
│   │   ├── Makefile                  #   make setup / make test-simple
│   │   ├── README.md                 #   七层完整链路
│   │   ├── reconciler.go             #   被测对象：Get + IgnoreNotFound
│   │   ├── suite_test.go             #   TestMain：起 etcd + apiserver
│   │   └── env_test.go               #   一条用例
│   ├── 01-get-branch-envtest/        #   在最小调用上加日志 / 错误 dump / UID
│   │   ├── reconciler.go             #   多两行日志
│   │   ├── suite_test.go
│   │   └── env_test.go               #   三条用例 + dumpK8sError
│   └── 02-manager-and-cache/         #   Manager 与缓存：速度 / 滞后 / apiReader
│       ├── suite_test.go             #   起 Manager + 等缓存同步
│       └── cache_test.go             #   三个实验
├── docs/                             # 章节级笔记
│   ├── 01-fake-client-syntax.md
│   ├── 02-three-test-layers.md
│   └── 03-table-driven.md
└── cmd/                              # 早期示例（保留，作为对照）
    ├── get-branch/                   #   fake 版：同一个 Reconcile
    └── envtest-basic/                #   完整版：ownerRef / 幂等 / 自愈 / 无 GC
```

**推荐顺序**（从零开始的话）：

1. `src/README.md` —— 先看清 00 和 01 各学什么
2. `src/00-get-branch-envtest/README.md` —— 再拆一条测试的七层链路
3. `docs/01-fake-client-syntax.md` —— 再懂那三行初始化
4. `cmd/get-branch/reconciler_test.go` —— 读懂一条最小测试
5. `docs/02-three-test-layers.md` —— 知道 fake 的边界在哪
6. `cmd/envtest-basic/` —— 最后看完整的 7 条断言

## 验收清单

跑完 `00-get-branch-envtest` 和 `cmd/envtest-basic` 后，你应该能回答：

- [ ] 为什么 fake client 下 ownerReference 看起来正常、实际却用不了？
- [ ] 为什么"幂等"用 ResourceVersion 判断，而不是"再调一次看有没有报错"？
- [ ] envtest 里为什么删了 ConfigMap，子 Secret 还在？
- [ ] 哪些断言**必须**写在 envtest 里，写进 fake 就是自欺欺人？

## 踩坑记录

（跑的过程中遇到的坑记在这里，比看别人的博客有用）

- `KUBEBUILDER_ASSETS` 没设置 → envtest 报 "unable to start control plane"。
- 忘了 `testEnv.Stop()` 或用了 `defer` → etcd 进程残留，下次端口占用。
- 用例复用固定对象名 → 第二次跑撞 `AlreadyExists`。

## 本节实测发现（第一次跑出来的，不是抄的）

1. **K8s 对象名不允许下划线。** 直接用 `strings.ToLower(t.Name())` 拼对象名，
   `TestEnv_CreatesSecret` 里的 `_` 会被 apiserver 以
   `metadata.name: Invalid value: ... must be a lowercase RFC 1123 subdomain` 拒绝；
   **fake client 完全不校验，同一个名字在 fake 下畅通无阻** —— 这本身就是一条幻觉。
2. **`SetControllerReference` 不校验 owner 的 UID。** 在 fake 里 owner 没有 UID 时，
   它不会报错，而是安静地把 `ownerReferences[0].uid = ""` 写进去，测试照样绿。
   真集群里 GC 靠 UID 匹配 owner，空 UID 等于匹配不到 —— 子对象变孤儿。
3. **envtest 里删除 owner 不会回收子对象。** 因为只跑 apiserver + etcd，
   没有 kube-controller-manager，也就没有 garbage collector。
