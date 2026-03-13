FROM golang:1.26 AS builder
WORKDIR /src
COPY go.mod go.sum ./
RUN go env -w GOPROXY=https://proxy.golang.org
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -o /cloudstackctl main.go
RUN CGO_ENABLED=0 go build -o /cloudstackctl-controller ./cmd/controller

FROM alpine:3.20
RUN apk add --no-cache ca-certificates
COPY --from=builder /cloudstackctl /cloudstackctl
COPY --from=builder /cloudstackctl-controller /cloudstackctl-controller
# Expose controller health/apply port
EXPOSE 65426
# Use CMD so containers default to the CLI binary; override with the controller binary
CMD ["/cloudstackctl"]
