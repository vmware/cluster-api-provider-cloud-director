FROM scratch
COPY ./templates/infrastructure-components.yaml \
    ./metadata.yaml \
    ./artifacts/bom.json \
    ./artifacts/dependencies.txt /src/artifact/
ENTRYPOINT ["/bin/sh"]
