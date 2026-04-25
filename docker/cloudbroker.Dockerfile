FROM golang:1.26-alpine AS build
RUN apk add --no-cache git
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -o /cloudbroker ./cmd/cloudbroker

FROM alpine:3.22
RUN apk add --no-cache wget
COPY --from=build /cloudbroker /cloudbroker
EXPOSE 8080
HEALTHCHECK --interval=5s --timeout=3s --retries=5 \
  CMD wget -q --spider http://localhost:8080/healthz || exit 1
ENTRYPOINT ["/cloudbroker"]
