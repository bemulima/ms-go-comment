FROM golang:1.23-alpine AS builder
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o ms-comment-service ./cmd/ms-comment-service

FROM alpine:3.19
WORKDIR /app
COPY --from=builder /app/ms-comment-service .
EXPOSE 8080
CMD ["/app/ms-comment-service"]
