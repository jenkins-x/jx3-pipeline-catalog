FROM alpine:latest as certs
RUN apk --update add ca-certificates

FROM scratch
EXPOSE 8080 9090
ENTRYPOINT ["/REPLACE_ME_APP_NAME"]
COPY --from=certs /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/ca-certificates.crt
COPY ./bin/ /