# 09-controller-builder · 拆开 NewControllerManagedBy

.PHONY: with-builder manual demo

with-builder: ## 版本 A：NewControllerManagedBy
	go run ./with-builder

manual: ## 版本 B：五步手写
	go run ./manual

demo: ## 触发一次事件
	kubectl create configmap demo-builder -n test --from-literal=a=1 \
		--dry-run=client -o yaml | kubectl apply -f -
	kubectl delete configmap demo-builder -n test --ignore-not-found