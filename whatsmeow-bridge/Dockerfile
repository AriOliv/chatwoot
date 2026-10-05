FROM golang:1.26-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY *.go ./
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/whatsmeow-bridge .

FROM alpine:3.22
RUN apk add --no-cache ca-certificates tzdata && adduser -D -H bridge
USER bridge
COPY --from=build /out/whatsmeow-bridge /usr/local/bin/whatsmeow-bridge
EXPOSE 8080
ENTRYPOINT ["/usr/local/bin/whatsmeow-bridge"]
