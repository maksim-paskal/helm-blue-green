test:
	helm lint --strict chart/helm-blue-green
	helm template chart/helm-blue-green | kubectl apply --dry-run -f -