FROM alpine:latest

USER 30001

COPY ./helm-blue-green /helm-blue-green

ENTRYPOINT [ "/helm-blue-green" ]
