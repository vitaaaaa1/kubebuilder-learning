# 11-controller-watch · 拆开 Watch

.PHONY: with-source custom-source demo

with-source: ## 内置 source.Kind
	go run ./with-source

custom-source: ## 自定义事件源（凭空塞 Request）
	go run ./custom-source

demo: ## 给 with-source 触发一次事件
	kubectl create configmap demo-watch -n test --from-literal=a=1 --dry-run=client -o yaml | kubectl apply -f -
	kubectl delete configmap demo-watch -n test --ignore-not-found
