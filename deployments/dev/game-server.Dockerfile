FROM golang:1.23-alpine AS builder
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -o /game-server ./cmd/game-server

FROM alpine:3.20
RUN apk add --no-cache ca-certificates
COPY --from=builder /game-server /game-server
EXPOSE 9000/udp
ENTRYPOINT ["/game-server"]
