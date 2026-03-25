FROM golang:1.23-alpine AS build

WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -o /rss-bot ./cmd/rss-bot

FROM alpine:3.20
RUN apk add --no-cache ca-certificates tzdata
COPY --from=build /rss-bot /usr/local/bin/rss-bot
ENTRYPOINT ["rss-bot"]
CMD ["-config", "/config/config.yaml"]
