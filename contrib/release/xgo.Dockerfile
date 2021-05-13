ARG GOVERSION=1.16.0
FROM techknowlogick/xgo:go-${GOVERSION}
# https://github.com/techknowlogick/xgo/issues/104
RUN go env -w GO111MODULE=auto
