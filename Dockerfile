# Build the manager binary
FROM golang:1.19 as builder

RUN apt-get update && \
    apt-get -y install \
        bash \
        git  \
        make

ADD . /go/src/github.com/vmware/cluster-api-provider-cloud-director
WORKDIR /go/src/github.com/vmware/cluster-api-provider-cloud-director

ENV GOPATH /go
ARG VERSION
RUN make build-within-docker VERSION=$VERSION && \
    chmod +x /build/vcloud/cluster-api-provider-cloud-director

########################################################

FROM photon:4.0-20210910

WORKDIR /opt/vcloud/bin

COPY --from=builder /build/vcloud/cluster-api-provider-cloud-director .

USER nobody
ENTRYPOINT ["/bin/bash", "-l", "-c"]
