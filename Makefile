test:
	helm lint --strict chart/helm-blue-green
	helm template chart/helm-blue-green | kubectl apply --dry-run -f -
build:
	docker build . -t paskalmaksim/helm-blue-green:dev
	docker push paskalmaksim/helm-blue-green:dev
clean:
	helm delete --purge helm-blue-green || true
	kubectl delete ns helm-blue-green || true
install:
	helm delete --purge helm-blue-green || true
	kubectl delete ns helm-blue-green || true
	helm upgrade --install helm-blue-green \
	--namespace helm-blue-green \
	--set host=http-echo.cluster-test.com \
	--set version=v1 \
	chart/helm-blue-green || true
	kubectl -n helm-blue-green logs -lapp=helm-blue-green