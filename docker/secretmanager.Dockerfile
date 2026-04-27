FROM golang:1.24-alpine AS builder
RUN go install github.com/blackwell-systems/gcp-secret-manager-emulator/cmd/server-dual@latest

FROM alpine:latest
COPY --from=builder /go/bin/server-dual /usr/local/bin/server-dual
EXPOSE 9090 8080
ENTRYPOINT ["server-dual"]
