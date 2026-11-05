# 05-build-and-add · Complete() 拆解

.PHONY: run run-builder run-no-watch step1 step2 step3 compare-manual compare-builder demo clean-demo

run: ## 手动装配 controller.New + Watch
	go run .

run-builder: ## Builder 装配 For + Complete
	go run ./builder

run-no-watch: ## 反例：不 Watch
	go run ./no-watch

step1: ## 分解演示：只 controller.New
	go run ./steps -mode=new-only

step2: ## 分解演示：controller.New + Watch
	go run ./steps -mode=new-watch

step3: ## 分解演示：For + Complete
	go run ./steps -mode=builder

compare-manual: ## 严格对照：手动装配
	go run ./compare/manual

compare-builder: ## 严格对照：Builder 装配
	go run ./compare/builder

demo: ## 触发一次事件
	kubectl create configmap demo-cm -n test \
		--from-literal=a=1 \
		--dry-run=client -o yaml | kubectl apply -f -
	kubectl delete configmap demo-cm -n test --ignore-not-found

clean-demo: ## 清理
	kubectl delete configmap demo-cm -n test --ignore-not-found