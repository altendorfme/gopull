FROM golang:1.21-alpine AS builder

RUN apk add --no-cache git openssh-client

WORKDIR /src

COPY go.mod go.sum* ./

RUN if [ -f go.mod ]; then go mod download; fi

COPY . .

RUN CGO_ENABLED=0 GOOS=linux go build -o /gopull ./cmd/gopull

FROM alpine:3.18

RUN apk add --no-cache git openssh-client

RUN mkdir -p /app /keys && \
    chmod 700 /keys

COPY --from=builder /gopull /usr/local/bin/gopull

EXPOSE 15800

ENV DEPLOY_KEY=""

VOLUME ["/app", "/keys"]

ENTRYPOINT ["/usr/local/bin/gopull"]