FROM golang:1.17 as builder

RUN apt-get update && \
    apt-get -y install \
        bash \
        git  \
        make

ADD . /go/src/github.com/vmware/rdeprojector
WORKDIR /go/src/github.com/vmware/rdeprojector

ENV CGO_ENABLED=0
ENV GOOS=linux
ENV GOARCH=amd64
ENV GOPATH /go

# Build
RUN ["make", "build-within-docker"]


########################################################

FROM photon:4.0-20210910

WORKDIR /opt/vcloud/bin
COPY --from=builder /build/vcloud/rdeprojector .
RUN chmod +x /opt/vcloud/bin/rdeprojector

USER 65532:65532

ENTRYPOINT ["/opt/vcloud/bin/rdeprojector", "-l", "-c"]
