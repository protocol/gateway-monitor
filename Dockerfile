FROM golang:1.17-alpine as builder
WORKDIR /app
ENV GOOS=linux
ENV GOARCH=amd64
ENV CGO_ENABLED=0
RUN apk --update add ca-certificates
COPY go.mod go.sum ./
RUN go mod download
COPY . ./
RUN go build -ldflags="-w -s" -o /gateway-monitor

FROM scratch
COPY --from=builder /gateway-monitor .
COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/ca-certificates.crt
ENTRYPOINT ["./gateway-monitor"]
CMD ["daemon"]
