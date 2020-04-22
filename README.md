# Blue/Green installations with helm only


## Test Blue/Green
You need kubernetes cluster v1.15 with helm and ingress

For example use host name http-echo.cluster-test.com

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