# Blue/Green installations with helm only
This type of installation is used for switch users to new version in one moment (kubernetes RollingUpdate strategy can simultaneously routing requests to new and to old version)

## Test Blue/Green
You need kubernetes cluster v1.15+ with helm v2.16+ and Ingress

For example use host name `http-echo.cluster-test.com`

install application version v1
```
helm upgrade --install helm-blue-green \
--namespace helm-blue-green \
--set host=http-echo.cluster-test.com \
--set version=v1 \
chart/helm-blue-green
```
run command to test
```
while true; do curl -sS http-echo.cluster-test.com ; sleep 0.1; done
```
update application to version v2
```
helm upgrade --install helm-blue-green \
--namespace helm-blue-green \
--set host=http-echo.cluster-test.com \
--set version=v2 \
chart/helm-blue-green
```
update application to version v3
```
helm upgrade --install helm-blue-green \
--namespace helm-blue-green \
--set host=http-echo.cluster-test.com \
--set version=v3 \
chart/helm-blue-green
```

## Clearing cluster after test
```
helm delete --purge helm-blue-green
kubectl delete ns helm-blue-green
```

## Known issues
* helm rollback doesn't work, version deployments is created by kubernetes job - helm can rollback only objects that was installed only by helm - for rollback - you need install requred version
