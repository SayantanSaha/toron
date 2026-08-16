FROM alpine:latest
WORKDIR /app
COPY dummy-services/dummy /app/dummy
EXPOSE 9001
ENTRYPOINT ["/app/dummy"]
CMD ["-port", "9001", "-count", "1"]
