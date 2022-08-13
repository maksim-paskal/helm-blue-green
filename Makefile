KUBECONFIG=$(HOME)/.kube/dev
tag=dev
version=v1

lint:
	ct lint --all
	ct lint --charts test/deploy
build:
	docker build --pull --push . -t paskalmaksim/helm-blue-green:$(tag)
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
