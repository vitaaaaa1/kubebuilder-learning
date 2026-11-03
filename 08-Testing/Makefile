# 08-Testing —— 本章的测试目标
#
# 目标：任何一条命令都能从零开始跑起来，不需要预先知道 envtest 是什么。

LOCALBIN        ?= $(shell pwd)/bin
ENVTEST         ?= $(LOCALBIN)/setup-envtest
# 与 07 章 k8s.io/* v0.31.3 对齐
ENVTEST_K8S_VERSION ?= 1.31.x
SETUP_ENVTEST_VERSION ?= latest

# 每次 make 都会求值：setup-envtest 不可用时返回空串，而不是报错
KUBEBUILDER_ASSETS ?= $(shell $(ENVTEST) use $(ENVTEST_K8S_VERSION) -p path 2>/dev/null)

.PHONY: setup-envtest envtest test test-fake test-env clean

setup-envtest: ## 安装 setup-envtest 工具（一次性）
	@mkdir -p $(LOCALBIN)
	GOBIN=$(LOCALBIN) go install sigs.k8s.io/controller-runtime/tools/setup-envtest@$(SETUP_ENVTEST_VERSION)

envtest: setup-envtest ## 下载 etcd / kube-apiserver 二进制（一次性，约 120MB）
	$(ENVTEST) use $(ENVTEST_K8S_VERSION) -p path

test: ## 跑全部测试（fake 反例 + envtest 真断言）
	KUBEBUILDER_ASSETS="$(KUBEBUILDER_ASSETS)" go test ./... -count=1

test-fake: ## 只跑 fake 反例：不启动 apiserver，秒级
	SKIP_ENVTEST=1 go test ./cmd/envtest-basic -run 'TestFake' -v -count=1

test-env: ## 只跑 envtest 断言：需要先 make envtest
	KUBEBUILDER_ASSETS="$(KUBEBUILDER_ASSETS)" go test ./cmd/envtest-basic -run 'TestEnv' -v -count=1

clean: ## 清理本地二进制
	rm -rf $(LOCALBIN)