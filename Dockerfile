FROM alpine:latest

RUN apk add --no-cache curl \
&& curl -LO https://storage.googleapis.com/kubernetes-release/release/v1.15.10/bin/linux/amd64/kubectl \
&& chmod +x ./kubectl \
&& mv ./kubectl /usr/local/bin/kubectl

