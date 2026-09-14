FROM golang:1.24-alpine AS builder
WORKDIR /app
COPY main.go ./
RUN CGO_ENABLED=0 go build -ldflags="-s -w" -o /app/origin main.go

FROM alpine:latest
WORKDIR /app
COPY --from=builder /app/origin /app/origin
EXPOSE 9104
ENV PORT=9104
CMD ["/app/origin"]
