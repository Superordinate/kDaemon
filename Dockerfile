FROM golang:1.5.3-onbuild
ADD ./config/app.conf
EXPOSE 1337 13337
ENTRYPOINT [/go/bin/kDaemon]
