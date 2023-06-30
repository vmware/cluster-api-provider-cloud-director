FROM alpine:3.18 as builder

WORKDIR /build/vcloud/

ARG CAPVCD_BUILD_DIR
ADD ${CAPVCD_BUILD_DIR}/cluster-api-provider-cloud-director .

RUN chmod +x /build/vcloud/cluster-api-provider-cloud-director
########################################################
FROM scratch

WORKDIR /opt/vcloud/bin

COPY --from=builder /build/vcloud/cluster-api-provider-cloud-director .
COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/ca-certificates.crt

# nobody user ID
USER 65534
ENTRYPOINT ["/opt/vcloud/bin/cluster-api-provider-cloud-director"]
