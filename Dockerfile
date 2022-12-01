FROM golang:1.19.3-alpine AS builder
ARG USER=default
ENV HOME /home/$USER
RUN apk --no-cache add upx make git gcc libtool musl-dev ca-certificates dumb-init sudo
RUN adduser -D $USER \
        && mkdir -p /etc/sudoers.d \
        && echo "$USER ALL=(ALL) NOPASSWD: ALL" > /etc/sudoers.d/$USER \
        && chmod 0440 /etc/sudoers.d/$USER
USER $USER
WORKDIR $HOME/build
COPY . .
RUN sudo chown -R $USER:$USER $HOME
RUN go mod tidy && CGO_ENABLED=1 go build -o ./exec/open-payment-host
EXPOSE 3000
CMD ["./exec/open-payment-host"]