# 02-for-and-scheme · For() 与 Scheme

.PHONY: run run-broken run-twice demo clean-demo

run: ## 两个控制器（ConfigMap + Secret）
	go run .

run-broken: ## 实验 A：Scheme 里没注册类型
	go run ./broken-scheme

run-twice: ## 实验 B：For 调用两次
	go run ./for-twice

demo: ## 创建 ConfigMap + Secret
	kubectl create configmap demo-cm -n test \
		--from-literal=a=1 \
		--dry-run=client -o yaml | kubectl apply -f -
	kubectl create secret generic demo-secret -n test \
		--from-literal=p=1 \
		--dry-run=client -o yaml | kubectl apply -f -

clean-demo: ## 删除演示对象
	kubectl delete configmap demo-cm -n test --ignore-not-found
	kubectl delete secret demo-secret -n test --ignore-not-found