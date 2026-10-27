# Build a static binary, then ship it in a tiny image.
FROM golang:1.24-alpine AS build
WORKDIR /src
COPY go.mod ./
COPY . .
RUN CGO_ENABLED=0 go build -ldflags="-s -w" -o /shortener ./cmd/server

FROM gcr.io/distroless/static-debian12
COPY --from=build /shortener /shortener
ENV DB_FILE=/data/links.json
VOLUME /data
EXPOSE 8080
ENTRYPOINT ["/shortener"]
