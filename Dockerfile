FROM alpine:3.22

RUN apk add --no-cache ca-certificates tzdata
WORKDIR /app

ARG TARGETPLATFORM
COPY $TARGETPLATFORM/boreas /app/boreas

EXPOSE 8080
ENTRYPOINT ["/app/boreas"]
