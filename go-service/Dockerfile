FROM golang:1.27-alpine AS builder

WORKDIR /app

COPY go.mod go.sum ./

RUN go mod download

COPY . .

RUN CGO_ENABLED=0 GOOS=linux go build \
	-o /tracelight-server \
	./cmd/server


FROM alpine:3.20

RUN apk add --no-cache ca-certificates

WORKDIR /app

COPY --from=builder /tracelight-server ./tracelight-server

EXPOSE 8080

CMD ["./tracelight-server"]