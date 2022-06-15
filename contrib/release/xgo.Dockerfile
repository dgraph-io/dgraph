ARG GOVERSION=1.17.3
FROM techknowlogick/xgo:go-${GOVERSION}
# https://github.com/techknowlogick/xgo/issues/104
RUN go env -w GO111MODULE=auto
