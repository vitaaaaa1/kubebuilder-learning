# 10-multi-controller · 一个 Manager 挂多个 Controller

.PHONY: run demo

run: ## 启动两个控制器
	go run .

demo: ## 各触发一次事件
	kubectl create configmap demo-cm -n test --from-literal=a=1 --dry-run=client -o yaml | kubectl apply -f -
	kubectl create secret generic demo-secret -n test --from-literal=b=2 --dry-run=client -o yaml | kubectl apply -f -
	kubectl delete configmap demo-cm -n test --ignore-not-found
	kubectl delete secret demo-secret -n test --ignore-not-found
