# Kubebuilder 风格的 Makefile
# 用于演示如何生成 Webhook manifests

# 获取当前目录的 Go module 名称
MODULE_NAME := example

# 控制器生成器二进制文件路径（假设已安装）
CONTROLLER_GEN ?= $(LOCALBIN)/controller-gen

.PHONY: manifests
manifests: controller-gen ## 生成 Webhook 和 RBAC 的 YAML 文件
	$(CONTROLLER_GEN) \
		paths="./..." \
		output:dir=config/webhook \
		webhook

.PHONY: generate
generate: controller-gen ## 生成包含 DeepCopy 等方法 的代码
	$(CONTROLLER_GEN) \
		object:headerFile="hack/boilerplate.go.txt" \
		paths="./..."

.PHONY: fmt
fmt: ## 运行 go fmt
	go fmt ./...

.PHONY: vet
vet: ## 运行 go vet
	go vet ./...

.PHONY: run
run: manifests generate fmt vet ## 在本地运行 Manager（不带 Webhook）
	ENABLE_WEBHOOKS=false go run ./cmd/main.go

.PHONY: run-webhook
run-webhook: manifests generate fmt vet ## 在本地运行 Manager（带 Webhook，需要证书）
	# 本地运行 Webhook 通常需要配置证书
	# 这里演示如何使用 makecert.sh 脚本生成证书
	./hack/setup-local-webhook.sh
	go run ./cmd/main.go

## --------------------------------------
## 工具下载
## --------------------------------------

LOCALBIN ?= $(shell pwd)/bin
$(LOCALBIN):
	mkdir -p $(LOCALBIN)

CONTROLLER_TOOLS_VERSION ?= v0.16.3

.PHONY: controller-gen
controller-gen: $(CONTROLLER_GEN) ## 下载 controller-gen 如果需要的话
$(CONTROLLER_GEN): $(LOCALBIN)
	$(call go-install-tool,$(CONTROLLER_GEN),sigs.k8s.io/controller-tools/cmd/controller-gen,$(CONTROLLER_TOOLS_VERSION))

define go-install-tool
@[ -f $(1) ] || { \
set -e ;\
TMP_DIR=$(shell mktemp -d) ;\
cd $$TMP_DIR ;\
go mod init tmp ;\
echo "Downloading $(2)" ;\
GOBIN=$(LOCALBIN) go install $(2)@$(3) ;\
rm -rf $$TMP_DIR ;\
}
endef
