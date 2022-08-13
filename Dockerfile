FROM alpine:latest

ENV VERSION_KUBERNETES=v1.23.9

RUN apk add --no-cache curl jq \
&& curl -LO https://storage.googleapis.com/kubernetes-release/release/$VERSION_KUBERNETES/bin/linux/amd64/kubectl \
&& chmod +x ./kubectl \
&& mv ./kubectl /usr/local/bin/kubectl
