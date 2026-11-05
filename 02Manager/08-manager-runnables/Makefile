# 08-manager-runnables · Manager 的 6 组 runnables

.PHONY: run slow

run: ## 正常运行，观察启动/停止顺序
	go run .

slow: ## 触发优雅关闭超时
	go run . -hold 3s -slow 5s -grace 2s