# 04-predicates · 事件过滤

.PHONY: run demo clean-demo

run: ## 三个控制器：无过滤 / 只标签变化 / 只 Create
	go run .

demo: ## 依次触发 create → data 更新 → label 更新 → delete
	kubectl create configmap demo-cm -n test \
		--from-literal=a=1 \
		--dry-run=client -o yaml | kubectl apply -f -
	@sleep 3
	kubectl patch configmap demo-cm -n test --type merge -p '{"data":{"a":"2"}}'
	@sleep 3
	kubectl label configmap demo-cm -n test stage=dev --overwrite
	@sleep 3
	kubectl delete configmap demo-cm -n test

clean-demo: ## 清理
	kubectl delete configmap demo-cm -n test --ignore-not-found