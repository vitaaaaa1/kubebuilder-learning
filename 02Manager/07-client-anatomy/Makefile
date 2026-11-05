# 07-client-anatomy · 拆开 client.New

.PHONY: with-client manual both

with-client: ## 版本 A：client.New
	go run ./with-client

manual: ## 版本 B：手写 rest.RESTClient
	go run ./manual

both: ## 先后跑两个版本
	@echo "===== A: client.New ====="
	@go run ./with-client
	@echo
	@echo "===== B: rest.RESTClient ====="
	@go run ./manual