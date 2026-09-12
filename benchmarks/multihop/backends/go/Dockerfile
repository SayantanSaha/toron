FROM golang:1.24-alpine AS builder
WORKDIR /app
COPY main.go ./
RUN CGO_ENABLED=0 go build -o /app/origin main.go

FROM alpine:latest
WORKDIR /app
COPY --from=builder /app/origin /app/origin
EXPOSE 9103
ENV PORT=9103
CMD ["/app/origin"]
