# 02-manager-and-cache · Manager 与缓存
#
# 这个 Makefile 只服务两件事：
#   make setup       一次性准备：装工具 + 下载 etcd / kube-apiserver
#   make test-simple 跑缓存实验：速度 / 滞后 / apiReader
#
# 日常只需要第二条；第一条在新机器上跑一次即可。

LOCALBIN            ?= $(shell pwd)/../bin
ENVTEST             ?= $(LOCALBIN)/setup-envtest
ENVTEST_K8S_VERSION ?= 1.31.x
SETUP_ENVTEST_VERSION ?= latest

# 每次 make 都会执行一次：工具不存在时返回空串（错误被丢弃），而不是中断
KUBEBUILDER_ASSETS ?= $(shell $(ENVTEST) use $(ENVTEST_K8S_VERSION) -p path 2>/dev/null)

.PHONY: test-simple setup

test-simple: ## 跑缓存实验：速度 / 滞后 / apiReader
	# KUBEBUILDER_ASSETS="/Users/vitaaaaa/Library/Application Support/io.kubebuilder.envtest/k8s/1.31.0-darwin-arm64" go test . -v -count=1
	KUBEBUILDER_ASSETS="$(KUBEBUILDER_ASSETS)" go test . -v -count=1

setup: ## 一次性准备：装 setup-envtest 工具 + 下载二进制（约 120MB）
	@mkdir -p $(LOCALBIN)
	# 安装工具本身 把“管理 envtest 资产的工具”编译好，放到你自己的 bin/ 目录里
	GOBIN=$(LOCALBIN) go install sigs.k8s.io/controller-runtime/tools/setup-envtest@$(SETUP_ENVTEST_VERSION)
	# 下载测试用的 k8s 组件 用那个工具，把“etcd + kube-apiserver”下载到缓存目录，并告诉我它们在哪
	$(ENVTEST) use $(ENVTEST_K8S_VERSION) -p path