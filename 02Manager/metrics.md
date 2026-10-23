# Controller Runtime 运行时指标说明文档

> 基于一次 `/metrics` 抓取样本整理。样本中 controller 仅有 `configmap` 一个，运行时间较短，多数计数器值很小或为 0。本文档用于说明指标含义、当前观测值及监控建议。

---

## 1. 文档说明

本文档覆盖样本中出现的指标类别：

| 类别 | 前缀/来源 | 说明 |
|---|---|---|
| 证书监听 | `certwatcher_*` | 证书文件读取次数与错误 |
| Controller Runtime | `controller_runtime_*` | Reconcile、Worker、Webhook、Conversion Webhook |
| 工作队列 | `workqueue_*` | Controller 工作队列深度、延迟、重试、处理耗时 |
| REST Client | `rest_client_*` | 访问 Kubernetes API Server 的 HTTP 请求 |
| Go Runtime | `go_*` | GC、内存、Goroutine、调度器、GODEBUG 等 |
| 进程 | `process_*` | CPU、内存、文件描述符、启动时间 |

指标类型说明：

- **counter**：单调递增，关注 `rate()` / `increase()`。
- **gauge**：瞬时值，关注当前值和趋势。
- **histogram**：分桶统计，关注 `_bucket`、`_sum`、`_count`，可用 `histogram_quantile()` 计算分位数。
- **summary**：客户端计算分位数，直接暴露 `quantile`、`_sum`、`_count`。

---

## 2. CertWatcher 指标

| 指标 | 类型 | 含义 | 当前值 |
|---|---|---|---|
| `certwatcher_read_certificate_total` | counter | 证书读取总次数 | 0 |
| `certwatcher_read_certificate_errors_total` | counter | 证书读取错误总次数 | 0 |

**解读**：当前未发生证书读取，或 certwatcher 尚未被触发。若使用 webhook 证书自动 reload，应关注 `read_certificate_errors_total` 是否增长。

**建议**：

- `certwatcher_read_certificate_errors_total > 0` 持续增长时告警，通常意味着证书文件缺失、权限错误或格式非法。

---

## 3. Controller Runtime 指标

### 3.1 Reconcile 相关

| 指标 | 类型 | 含义 | 当前值 |
|---|---|---|---|
| `controller_runtime_reconcile_total` | counter | 按 `result` 统计的 reconcile 次数 | `success=2`，其余 0 |
| `controller_runtime_reconcile_errors_total` | counter | reconcile 错误总数 | 0 |
| `controller_runtime_reconcile_panics_total` | counter | reconcile panic 总数 | 0 |
| `controller_runtime_reconcile_timeouts_total` | counter | reconcile 超时总数 | 0 |
| `controller_runtime_terminal_reconcile_errors_total` | counter | 终态 reconcile 错误数 | 0 |
| `controller_runtime_reconcile_time_seconds` | histogram | 每次 reconcile 耗时 | 2 次，总耗时 0.000498333s |

`reconcile_total` 的 `result` 标签包括：

- `success`：成功。
- `error`：返回错误。
- `requeue`：显式 requeue。
- `requeue_after`：按 `RequeueAfter` 重新入队。

**当前解读**：

- `configmap` controller 共 reconcile 2 次，全部成功。
- 总耗时约 **0.498ms**，平均每次约 **0.249ms**。
- 所有 histogram bucket 在 `0.005s` 内均已累计到 2，说明耗时极短。
- 无错误、无 panic、无超时。

### 3.2 Worker 与并发

| 指标 | 类型 | 含义 | 当前值 |
|---|---|---|---|
| `controller_runtime_active_workers` | gauge | 当前每个 controller 活跃 worker 数 | `configmap=0` |
| `controller_runtime_max_concurrent_reconciles` | gauge | 每个 controller 最大并发 reconcile 数 | `configmap=1` |

**解读**：`configmap` controller 最大并发为 1，当前没有正在处理的 worker。该值通常由 `Options.Controller.MaxConcurrentReconciles` 或 controller 级配置决定。

### 3.3 Webhook 与 Conversion Webhook

| 指标 | 类型 | 含义 | 当前值 |
|---|---|---|---|
| `controller_runtime_webhook_panics_total` | counter | webhook panic 总数 | 0 |
| `controller_runtime_conversion_webhook_panics_total` | counter | conversion webhook panic 总数 | 0 |

**解读**：当前无 webhook panic。样本中未出现 webhook 请求延迟/请求数指标，可能未启用 webhook 或未被抓取。

---

## 4. Workqueue 指标

样本中 workqueue 标签为 `controller="configmap", name="configmap"`。

| 指标 | 类型 | 含义 | 当前值 |
|---|---|---|---|
| `workqueue_adds_total` | counter | 入队总次数 | 2 |
| `workqueue_depth` | gauge | 当前队列深度（按 priority） | `priority="-100"` 为 0 |
| `workqueue_retries_total` | counter | 带非零延迟重新入队的次数 | 0 |
| `workqueue_longest_running_processor_seconds` | gauge | 最长运行中的 processor 已运行秒数 | 0 |
| `workqueue_unfinished_work_seconds` | gauge | 已开始但尚未被 work_duration 观测到的累计工作时间 | 0 |
| `workqueue_queue_duration_seconds` | histogram |  item 在队列中等待被处理的时长 | 2 次，总 0.202888499s |
| `workqueue_work_duration_seconds` | histogram | 处理一个 item 的耗时 | 2 次，总 0.000566042s |

**当前解读**：

- 入队 2 次，队列深度为 0，无重试。
- 平均排队时间约 **101.4ms**（0.202888499 / 2），说明 item 入队后等待了约 100ms 才被 worker 取走。
- 平均处理时间约 **0.283ms**（0.000566042 / 2）。
- `unfinished_work_seconds=0`，无卡住的 worker。
- `longest_running_processor_seconds=0`，当前无长任务。

**关注点**：

- `workqueue_depth` 持续大于 0 且上升：处理能力不足或 reconcile 变慢。
- `workqueue_queue_duration_seconds` 高分位上升：排队严重。
- `workqueue_retries_total` 快速增长：频繁限流或错误重试。
- `workqueue_unfinished_work_seconds` 持续增长：可能有 worker 卡死。

---

## 5. REST Client 指标

| 指标 | 类型 | 含义 | 当前值 |
|---|---|---|---|
| `rest_client_requests_total` | counter | 按 status code、method、host 统计的 HTTP 请求数 | `code=200, host=47.110.225.40:6443, method=GET` 为 3 |

**解读**：

- 启动后仅向 API Server 发出 3 次 GET 请求，均返回 200。
- 说明当前 API 交互很少，符合短时间运行、单 controller 的场景。
- 若出现 `code` 为 4xx/5xx、`method` 为 POST/PUT/PATCH/DELETE 的请求增长，需要关注权限、冲突、限流等问题。

**建议**：

- 关注非 2xx 比例：`sum(rate(rest_client_requests_total{code!~"2.."}[5m]))`。
- 关注 429 和 5xx，通常对应 API Server 限流或服务端异常。

---

## 6. Go Runtime 指标

样本中 Go 版本为 `go1.27.2`（以 `go_info` 为准）。Go Runtime 指标数量很多，按主题归类如下。

### 6.1 Goroutine 与调度器

| 指标 | 类型 | 含义 | 当前值 |
|---|---|---|---|
| `go_goroutines` | gauge | 当前 Goroutine 数 | 39 |
| `go_sched_goroutines_goroutines` | gauge | runtime/metrics 口径的活跃 Goroutine 数 | 57 |
| `go_sched_goroutines_created_goroutines_total` | counter | 累计创建 Goroutine 数 | 107 |
| `go_sched_goroutines_running_goroutines` | gauge | 正在执行的 Goroutine 数 | 3 |
| `go_sched_goroutines_runnable_goroutines` | gauge | 可运行但未执行的 Goroutine 数 | 0 |
| `go_sched_goroutines_waiting_goroutines` | gauge | 等待 I/O 或同步原语的 Goroutine 数 | 53 |
| `go_sched_goroutines_not_in_go_goroutines` | gauge | 在系统调用或 cgo 中的 Goroutine 数 | 1 |
| `go_sched_gomaxprocs_threads` | gauge | 当前 `GOMAXPROCS` | 11 |
| `go_sched_threads_total_threads` | gauge | Go runtime 拥有的 OS 线程数 | 14 |
| `go_threads` | gauge | 创建的 OS 线程数 | 15 |
| `go_sched_latencies_seconds` | histogram | Goroutine 在 runnable 状态等待被调度的时间 | 146 次 |

**说明**：`go_goroutines` 与 `go_sched_goroutines_goroutines` 口径不同，前者是 `runtime.NumGoroutine()`，后者来自 runtime/metrics，两者不应直接对比，建议各自看趋势。

**关注点**：

- Goroutine 数持续增长且不回落：可能存在泄漏。
- `go_sched_goroutines_runnable_goroutines` 长期偏高：CPU 竞争或调度压力。
- `go_sched_latencies_seconds` 高分位上升：调度延迟增加。

### 6.2 GC 与堆内存

| 指标 | 类型 | 含义 | 当前值 |
|---|---|---|---|
| `go_gc_cycles_total_gc_cycles_total` | counter | 完成的 GC 周期总数 | 5 |
| `go_gc_cycles_automatic_gc_cycles_total` | counter | runtime 自动触发的 GC 周期数 | 5 |
| `go_gc_cycles_forced_gc_cycles_total` | counter | 应用强制触发的 GC 周期数 | 0 |
| `go_gc_duration_seconds` | summary | STW GC 暂停时长分位数 | 5 次，sum 0.000325459s |
| `go_gc_gogc_percent` | gauge | `GOGC` 百分比 | 100 |
| `go_gc_gomemlimit_bytes` | gauge | `GOMEMLIMIT` | `9.22e18`，即默认接近无限制 |
| `go_gc_heap_goal_bytes` | gauge | 下一次 GC 的目标堆大小 | 约 9.67MB |
| `go_gc_heap_live_bytes` | gauge | 上一次 GC 标记的存活堆内存 | 约 4.65MB |
| `go_gc_heap_objects_objects` | gauge | 堆对象数（存活或未清扫） | 19571 |
| `go_gc_heap_allocs_bytes_total` | counter | 累计堆分配字节数 | 约 9.10MB |
| `go_gc_heap_allocs_objects_total` | counter | 累计堆分配对象数 | 39429 |
| `go_gc_heap_frees_bytes_total` | counter | 累计 GC 释放堆字节数 | 约 4.41MB |
| `go_gc_heap_frees_objects_total` | counter | 累计 GC 释放对象数 | 19858 |
| `go_gc_heap_tiny_allocs_objects_total` | counter | tiny 对象分配计数 | 2230 |
| `go_gc_scan_*` | gauge | 全局变量、堆、栈等可扫描空间大小 | 见样本 |
| `go_gc_stack_starting_size_bytes` | gauge | 新 Goroutine 初始栈大小 | 2048 |
| `go_gc_limiter_last_enabled_gc_cycle` | gauge | GC CPU limiter 上次启用的 GC 周期 | 0，表示从未启用 |

**当前解读**：

- 共 5 次 GC，均为自动触发。
- GC 暂停最大约 **132.6µs**（`go_gc_duration_seconds{quantile="1"}`），暂停很短。
- 当前堆分配约 **4.69MB**，GC 目标约 **9.67MB**，存活约 **4.65MB**。
- `GOMEMLIMIT` 未设置，仍为默认极大值。

**关注点**：

- `go_gc_duration_seconds` 高分位持续上升：GC 压力增大。
- `go_gc_heap_live_bytes` 持续上升且不回落：可能存在内存泄漏。
- `go_gc_gomemlimit_bytes` 建议在容器中结合 memory limit 设置，避免 OOM。
- `go_gc_limiter_last_enabled_gc_cycle > 0`：说明 GC CPU limiter 被启用过，通常与 `GOMEMLIMIT` 相关，需要关注 CPU/内存权衡。

### 6.3 内存分类与 memstats

| 指标 | 含义 | 当前值 |
|---|---|---|
| `go_memory_classes_heap_objects_bytes` | 存活及未释放对象占用堆内存 | 约 4.69MB |
| `go_memory_classes_heap_free_bytes` | 空闲但未归还的堆内存 | 约 1.24MB |
| `go_memory_classes_heap_released_bytes` | 已归还 OS 的堆内存 | 约 3.48MB |
| `go_memory_classes_heap_unused_bytes` | 预留给堆对象但未使用的内存 | 约 2.32MB |
| `go_memory_classes_heap_stacks_bytes` | Goroutine 栈占用 | 约 852KB |
| `go_memory_classes_metadata_*` | runtime 元数据（mcache、mspan、other） | 见样本 |
| `go_memory_classes_other_bytes` | 调试、finalizer、profiler 等 | 约 1.73MB |
| `go_memory_classes_profiling_buckets_bytes` | profiling 哈希表 | 约 1.45MB |
| `go_memory_classes_total_bytes` | Go runtime 映射的总读写内存 | 约 19.65MB |
| `go_memstats_*` | 传统 memstats 指标，多数与上述指标等价 | 见样本 |

**说明**：`go_memstats_*` 中很多指标与 `go_memory_classes_*` / `go_gc_*` 存在等价关系。新版本建议优先使用 `go_gc_*` 和 `go_memory_classes_*`，`go_memstats_*` 主要用于兼容旧面板。

### 6.4 CPU classes

| 指标 | 含义 | 当前值 |
|---|---|---|
| `go_cpu_classes_total_cpu_seconds_total` | 可用 CPU 总时间（GOMAXPROCS 积分） | 约 25.09s |
| `go_cpu_classes_user_cpu_seconds_total` | 用户 Go 代码 CPU 时间 | 约 0.0535s |
| `go_cpu_classes_idle_cpu_seconds_total` | 未使用 CPU 时间 | 约 25.03s |
| `go_cpu_classes_gc_total_cpu_seconds_total` | GC 总 CPU 时间 | 约 0.0140s |
| `go_cpu_classes_gc_mark_assist_cpu_seconds_total` | GC mark assist CPU 时间 | 约 0.000549s |
| `go_cpu_classes_gc_mark_dedicated_cpu_seconds_total` | 专用 GC mark CPU 时间 | 约 0.00882s |
| `go_cpu_classes_gc_mark_idle_cpu_seconds_total` | 空闲 CPU 上执行的 GC mark 时间 | 约 0.00232s |
| `go_cpu_classes_gc_pause_cpu_seconds_total` | GC STW 暂停 CPU 时间 | 约 0.00226s |
| `go_cpu_classes_scavenge_*` | 内存回收相关 CPU 时间 | 极小 |

**解读**：绝大部分 CPU 时间为 idle，GC CPU 占比很低，符合低负载短运行场景。

### 6.5 GC 暂停与调度暂停直方图

| 指标 | 说明 |
|---|---|
| `go_gc_pauses_seconds` | 已废弃，建议使用 `go_sched_pauses_total_gc_seconds` |
| `go_sched_pauses_stopping_gc_seconds` | GC 相关 STW 中“停止所有 P”的耗时 |
| `go_sched_pauses_stopping_other_seconds` | 非 GC 相关 STW 中“停止所有 P”的耗时 |
| `go_sched_pauses_total_gc_seconds` | GC 相关 STW 总暂停 |
| `go_sched_pauses_total_other_seconds` | 非 GC 相关 STW 总暂停 |

样本中 GC 暂停共 10 次，总约 **139.9µs**，最大落在 `8.19e-05` 到 `0.0009175` 秒桶之间；非 GC STW 为 0 次。

### 6.6 其他

| 指标 | 含义 | 当前值 |
|---|---|---|
| `go_info{version="go1.27.2"}` | Go 版本信息 | 1 |
| `go_cgo_go_to_c_calls_calls_total` | Go 调用 C 的次数 | 1 |
| `go_sync_mutex_wait_total_seconds_total` | 阻塞在 mutex/runtime 锁上的累计时间 | 约 0.000994s |
| `go_godebug_non_default_behavior_*` | 非默认 GODEBUG 行为触发次数 | 全部 0 |

`go_godebug_non_default_behavior_*` 指标数量多，但正常情况下均应为 0。出现非 0 说明使用了非默认 GODEBUG 设置，通常不需要告警，但可用于排查环境差异。

---

## 7. Process 指标

| 指标 | 类型 | 含义 | 当前值 |
|---|---|---|---|
| `process_cpu_seconds_total` | counter | 用户态+系统态 CPU 总时间 | 0.089922s |
| `process_resident_memory_bytes` | gauge | 常驻内存 RSS | 约 27.41MB |
| `process_virtual_memory_bytes` | gauge | 虚拟内存大小 | 约 447.18GB |
| `process_virtual_memory_max_bytes` | gauge | 最大可用虚拟内存 | `9.22e18` |
| `process_open_fds` | gauge | 当前打开文件描述符数 | 11 |
| `process_max_fds` | gauge | 最大文件描述符数 | 61440 |
| `process_start_time_seconds` | gauge | 进程启动 Unix 时间 | `1792660533` |

**解读**：

- RSS 约 **27.4MB**，与 Go runtime 总映射内存约 19.65MB 相比略高，包含非 Go 部分。
- 虚拟内存约 **447GB** 是 Go runtime 保留地址空间的正常现象，不代表真实占用。
- 打开 fd 仅 11 个，远低于上限 61440。
- 进程启动时间约对应 2026-10-22（以实际时区换算为准）。

**关注点**：

- `process_resident_memory_bytes` 接近容器 memory limit 时告警。
- `process_open_fds / process_max_fds` 比例过高时告警。
- `process_cpu_seconds_total` 用 `rate()` 观察 CPU 使用率。

---

## 8. 当前样本综合解读

| 维度 | 观测 | 结论 |
|---|---|---|
| Controller | 仅 `configmap`，最大并发 1 | 单 controller 低负载 |
| Reconcile | 2 次，全部 success，平均约 0.25ms | 正常 |
| 错误/panic/超时 | 均为 0 | 正常 |
| Workqueue | 入队 2，深度 0，重试 0，平均排队约 101ms，处理约 0.28ms | 正常，排队时间略高于处理时间但绝对值很小 |
| API 请求 | 3 次 GET 200 | 正常 |
| GC | 5 次，最大 STW 约 133µs | 正常 |
| 内存 | 堆分配约 4.69MB，RSS 约 27.4MB | 正常 |
| Goroutine | `go_goroutines=39`，`go_sched_goroutines=57` | 口径不同，无异常 |
| FD | 11 / 61440 | 正常 |
| CertWatcher | 读取 0 次，错误 0 | 未触发或未使用 |
| Webhook | panic 0 | 正常 |

样本中**未出现** Leader Election 相关指标（如 `leader_election_master_status`），说明可能未启用 leader election，或该指标未被抓取。

---

## 9. 告警与巡检建议

### 9.1 建议告警项

| 告警 | 表达式示例 | 说明 |
|---|---|---|
| Reconcile 错误增长 | `increase(controller_runtime_reconcile_errors_total[5m]) > 0` | 持续出现需排查 |
| Reconcile panic | `increase(controller_runtime_reconcile_panics_total[5m]) > 0` | 任何 panic 都建议告警 |
| Reconcile 超时 | `increase(controller_runtime_reconcile_timeouts_total[5m]) > 0` | 关注超时配置与阻塞 |
| 终态错误 | `increase(controller_runtime_terminal_reconcile_errors_total[5m]) > 0` | 终态错误通常不可自动恢复 |
| 队列积压 | `workqueue_depth > 0` 持续 5–10 分钟 | 结合处理速率判断 |
| 重试增长 | `increase(workqueue_retries_total[5m]) > 0` 持续 | 可能限流或错误 |
| 未完成工作时间增长 | `workqueue_unfinished_work_seconds` 持续上升 | 可能有卡死 worker |
| API 非 2xx | `sum(rate(rest_client_requests_total{code!~"2.."}[5m])) > 0` | 关注 401/403/429/5xx |
| 证书读取错误 | `increase(certwatcher_read_certificate_errors_total[5m]) > 0` | 证书问题 |
| Webhook panic | `increase(controller_runtime_webhook_panics_total[5m]) > 0` | 任何 panic 都建议告警 |
| Conversion Webhook panic | `increase(controller_runtime_conversion_webhook_panics_total[5m]) > 0` | 同上 |
| Goroutine 泄漏 | `go_goroutines` 持续上升且不回落 | 结合历史基线 |
| GC 暂停过高 | `histogram_quantile(0.99, rate(go_gc_duration_seconds[5m]))` 或对应 histogram | 视 SLA 设定阈值 |
| RSS 接近 limit | `process_resident_memory_bytes / container_memory_limit > 0.8` | 容器场景 |
| FD 接近上限 | `process_open_fds / process_max_fds > 0.8` | 文件描述符泄漏 |

### 9.2 建议巡检项

- `controller_runtime_max_concurrent_reconciles` 是否符合预期。
- `controller_runtime_active_workers` 是否长期打满。
- `workqueue_queue_duration_seconds` 高分位是否随负载上升。
- `workqueue_work_duration_seconds` 高分位是否与 reconcile 耗时一致。
- `go_gc_heap_live_bytes` 是否呈持续上升趋势。
- `go_sched_latencies_seconds` 高分位是否升高。
- `go_gc_gomemlimit_bytes` 是否已按容器限制设置。
- `go_info` 版本是否与预期一致。

---

## 10. 采集与使用注意事项

1. **Histogram 使用方式**  
   使用 `rate(metric_bucket[5m])` 配合 `histogram_quantile()` 计算分位数，不要直接对 `_bucket` 原始值求分位。

2. **Counter 使用方式**  
   使用 `rate()` 或 `increase()`，不要直接看累计值判断当前速率。

3. **`go_memstats_*` 与 `go_gc_*` / `go_memory_classes_*` 的关系**  
   很多指标语义等价。新面板建议优先使用 `go_gc_*`、`go_memory_classes_*`、`go_sched_*`，旧面板可继续使用 `go_memstats_*`。

4. **`go_gc_pauses_seconds` 已废弃**  
   建议迁移到 `go_sched_pauses_total_gc_seconds`。

5. **`go_goroutines` 与 `go_sched_goroutines_goroutines` 口径不同**  
   两者不要混用做同一趋势判断，选择其中一个作为主指标即可。

6. **`process_virtual_memory_bytes` 很大是正常的**  
   Go runtime 会保留大量虚拟地址空间，不代表真实内存占用，应以 `process_resident_memory_bytes` 为准。

7. **Controller label 基数**  
   `controller`、`name` 标签会随 controller 数量增长，注意控制抓取和存储成本。

8. **Leader Election 指标**  
   若启用 leader election，应额外关注 `leader_election_master_status` 等指标；本样本未包含。

9. **样本时间窗口短**  
   当前多数 counter 值很小，告警阈值应结合长期基线设置，避免短窗口误报。

---

## 11. 结论

该样本显示：

- `configmap` controller 运行正常，reconcile 全部成功且耗时极短；
- workqueue 无积压、无重试；
- API 请求正常；
- Go runtime GC、内存、Goroutine、FD 均处于低位健康状态；
- certwatcher 与 webhook 无异常，但样本中未体现其实际流量。

建议将本文档中的告警项接入 Prometheus/Alertmanager，并以 `controller`、`name` 为维度建立面板，重点观察 **reconcile 错误/耗时、workqueue 深度与重试、API 非 2xx、GC 暂停、RSS 与 FD** 这几类核心指标。