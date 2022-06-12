FROM golang:1.18-alpine3.16 AS build

WORKDIR /app

COPY go.mod ./
COPY go.sum ./
RUN go mod download

COPY main.go ./
COPY bot ./bot
RUN go build -o /replay-bot

FROM alpine:3.16

ENV DISCORD_TOKEN=""
ENV DISCORD_GUILD_ID=""
ENV DISCORD_CHANNEL=""
ENV DEVELOPMENT="false"

RUN apk add --no-cache ffmpeg

WORKDIR /
COPY --from=build /replay-bot /replay-bot

ENTRYPOINT ["/replay-bot"]
