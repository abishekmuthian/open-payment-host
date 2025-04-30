FROM golang:1.24.2-alpine AS builder
ARG USER=default
ENV HOME /home/$USER
RUN apk --no-cache add upx make git gcc libtool musl-dev ca-certificates dumb-init sudo curl libcap-static libcap-dev build-base
RUN git clone --depth 1 https://git.kernel.org/pub/scm/libs/libcap/libcap
RUN gcc --static libcap/progs/setcap.c -o /bin/setcap -lcap
RUN adduser -D $USER \
        && mkdir -p /etc/sudoers.d \
        && echo "$USER ALL=(ALL) NOPASSWD: ALL" > /etc/sudoers.d/$USER \
        && chmod 0440 /etc/sudoers.d/$USER
USER $USER
WORKDIR $HOME/build
COPY . .
RUN sudo chown -R $USER:$USER $HOME
RUN go mod tidy && CGO_ENABLED=1 go build -o ./exec/open-payment-host
RUN sudo setcap CAP_NET_BIND_SERVICE=+eip ./exec/open-payment-host
EXPOSE 3000
RUN sudo chmod +x .railway/setup-volume.sh
CMD ["./.railway/setup-volume.sh"]