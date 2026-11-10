# 14-reconcile-dispatch · Reconcile 调度参数

.PHONY: 01 02 03 04 05 06 clean

01: ## 空 Result
	go run ./01-result-empty

02: ## RequeueAfter
	go run ./02-requeue-after

03: ## 返回 error，自动退避
	go run ./03-error-backoff

04: ## panic 被恢复
	go run ./04-panic-recover

05: ## 3 个 worker 并行
	go run ./05-concurrent-workers

06: ## SyncPeriod 周期重放
	go run ./06-sync-period

clean: ## 删除演示对象
	kubectl delete cm demo-a demo-b demo-c demo-dispatch -n test --ignore-not-found
