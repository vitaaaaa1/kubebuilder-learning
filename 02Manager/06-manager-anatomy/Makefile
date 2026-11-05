# 06-manager-anatomy · 拆开 ctrl.NewManager

.PHONY: manager manual both

manager: ## 版本 A：ctrl.NewManager
	go run ./with-manager

manual: ## 版本 B：手动组装
	go run ./manual

both: ## 先后跑两个版本
	@echo "===== A: ctrl.NewManager ====="
	@go run ./with-manager
	@echo
	@echo "===== B: 手动组装 ====="
	@go run ./manual