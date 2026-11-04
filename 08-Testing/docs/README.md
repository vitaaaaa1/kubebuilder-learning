# 08-Testing · 笔记索引

本目录集中存放这一章的**章节级**笔记。单个沙盒自己的说明放在沙盒目录里（如 `../src/README.md`）。**编号 = 推荐阅读顺序**，目录名不带编号。

| 编号 | 文档 | 讲什么 | 读完能干什么 |
|---|---|---|---|
| 01 | [01-fake-client-syntax.md](01-fake-client-syntax.md) | `fake.NewClientBuilder().WithScheme().Build()` 三行的语法拆解 | 看懂任何一条 Reconcile 测试的开头，知道改哪三处 |
| 02 | [02-three-test-layers.md](02-three-test-layers.md) | 纯单测 / fake client / envtest 的边界对照 | 判断某条断言该写在哪一层，不再假绿 |
| 03 | [03-table-driven.md](03-table-driven.md) | 表驱动测试模板：一个函数覆盖多个场景 | 复制模板即可写新测试，知道怎么加 case |
| 04 | _待写_ | Manager + 缓存：`GetClient` vs `GetAPIReader` | — |
| 05 | _待写_ | 装 CRD 进 envtest：`CRDDirectoryPaths` | — |
| 06 | _待写_ | Webhook 测试与 TLS | — |

## 约定

- **文件名**：`NN-主题.md`，小写、连字符分隔。编号一旦定下就不再调整——新增笔记往后排，避免历史链接失效。
- **代码旁边的 README** 只写"怎么跑、目录结构、验收标准"，**原理和语法一律放这里**，不在两处重复。
- 每篇笔记结构固定：**它解决什么问题 → 逐行/逐层拆解 → 实验 → 报错对照 → 自测**。
- 实验必须是**跑通过的**，报错文本要贴真实输出，不写"类似"。

## 与代码的对应

| 笔记 | 对应代码 |
|---|---|
| 01 | [cmd/get-branch/reconciler_test.go](../cmd/get-branch/reconciler_test.go) |
| 02 | [cmd/envtest-basic/fake_test.go](../cmd/envtest-basic/fake_test.go) · [env_test.go](../cmd/envtest-basic/env_test.go) |
| 03 | [cmd/get-branch/table_test.go](../cmd/get-branch/table_test.go) |