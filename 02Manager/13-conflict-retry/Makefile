# 13-conflict-retry · 冲突与 RetryOnConflict

.PHONY: naive with-retry both

naive: ## 冲突直接失败
	go run ./naive -workers 20

with-retry: ## 冲突后重读重写
	go run ./with-retry -workers 20

both: naive with-retry
