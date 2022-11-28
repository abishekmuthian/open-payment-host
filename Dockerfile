FROM golang:1.19.3-alpine AS builder
WORKDIR /build
RUN apk --no-cache add upx make git gcc libtool musl-dev ca-certificates dumb-init
COPY . .
RUN go mod tidy && CGO_ENABLED=1 go build -o /open-payment-host
EXPOSE 3000
CMD ["/open-payment-host"]