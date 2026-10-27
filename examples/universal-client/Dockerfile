FROM scratch
COPY product-client /product-client
COPY ca-certificates.crt /etc/ssl/certs/ca-certificates.crt
COPY --chown=65532:65532 empty-data /data
USER 65532:65532
VOLUME ["/data"]
ENTRYPOINT ["/product-client","--config","/product/config.json","--data","/data"]
CMD ["serve"]
