# 01-controller-basics · 最小 Controller

BINARY := bin/controller

.PHONY: run build demo clean-demo tidy

run: ## 直接运行（用 ~/.kube/config 连集群）
	go run .

build: ## 编译到 bin/controller
	go build -o $(BINARY) .

tidy: ## 整理依赖
	go mod tidy

demo: ## 创建并修改一个 ConfigMap，触发 Reconcile
	kubectl create configmap demo-config -n test \
		--from-literal=k=v \
		--dry-run=client -o yaml | kubectl apply -f -
	kubectl label configmap demo-config -n test demo=1 --overwrite

clean-demo: ## 删除演示用的 ConfigMap
	kubectl delete configmap demo-config -n test --ignore-not-found