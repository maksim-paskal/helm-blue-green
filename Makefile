KUBECONFIG=$(HOME)/.kube/dev
tag=dev
version=v1
image=paskalmaksim/helm-blue-green:$(tag)
hash=$(shell git rev-parse --short HEAD)
beta=beta-$(hash)

lint:
	helm dep up test/deploy --skip-refresh
	ct lint --all
	ct lint --charts test/deploy
	ct lint --charts e2e/chart
build:
	git tag -d `git tag -l "helm-blue-green-*"`
	git tag -d `git tag -l "helm-chart-*"`
	go run github.com/goreleaser/goreleaser/v2@latest build --clean --snapshot --skip=validate
	mv ./dist/helm-blue-green_linux_amd64_v1/helm-blue-green ./helm-blue-green
	docker buildx build --platform=linux/amd64 --pull --push . -t $(image)
promote-to-beta:
	git tag -d `git tag -l "helm-blue-green-*"`
	git tag -d `git tag -l "helm-chart-*"`
	go run github.com/goreleaser/goreleaser/v2@latest release --clean --snapshot
	# rename helm-blue-green to beta + hash
	docker tag paskalmaksim/helm-blue-green:beta-arm64 paskalmaksim/helm-blue-green:$(beta)-arm64
	docker tag paskalmaksim/helm-blue-green:beta-amd64 paskalmaksim/helm-blue-green:$(beta)-amd64
	# push beta + hash
	docker push paskalmaksim/helm-blue-green:$(beta)-arm64
	docker push paskalmaksim/helm-blue-green:$(beta)-amd64
	docker manifest create --amend paskalmaksim/helm-blue-green:$(beta) \
	paskalmaksim/helm-blue-green:$(beta)-arm64 \
	paskalmaksim/helm-blue-green:$(beta)-amd64
	docker manifest push --purge paskalmaksim/helm-blue-green:$(beta)
clean:
	helm --namespace helm-blue-green delete helm-blue-green || true
	kubectl delete ns helm-blue-green || true
install:
	rm -rf test/deploy/charts
	helm dep up test/deploy --skip-refresh

	helm upgrade --install helm-blue-green \
	--namespace helm-blue-green \
	--create-namespace \
	--set helm-blue-green.image.tag=$(tag) \
	--set helm-blue-green.image.pullPolicy=Always \
	--set helm-blue-green.version=$(version) \
	test/deploy

.PHONY: e2e
e2e:
	make clean
	helm upgrade --install helm-blue-green \
	--namespace helm-blue-green \
	--create-namespace \
	e2e/chart

	NAMESPACE=helm-blue-green go test -v ./e2e \
	--kubeconfig $(KUBECONFIG)
	
	make clean
.PHONY: test
test:
	./scripts/validate-license.sh
	go fmt ./cmd/... ./pkg/... ./internal/...
	go vet ./cmd/... ./pkg/... ./internal/...
	go test ./pkg/...
	go mod tidy
	go run github.com/golangci/golangci-lint/cmd/golangci-lint@latest run -v
run:
	NAMESPACE=default go run --race ./cmd \
	--config=./e2e/testdata/test2.yaml \
	--log.level=debug \
	--log.json=false \
	--kubeconfig $(KUBECONFIG)