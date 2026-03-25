FROM golang:1.23-alpine AS build

WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -o /vox-caster-bot ./cmd/vox-caster-bot

FROM alpine:3.20
RUN apk add --no-cache ca-certificates tzdata
COPY --from=build /vox-caster-bot /usr/local/bin/vox-caster-bot
ENTRYPOINT ["vox-caster-bot"]
CMD ["-config", "/config/config.yaml"]
