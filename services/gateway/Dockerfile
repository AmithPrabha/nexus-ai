FROM golang:1.27.4-alpine AS build
WORKDIR /src
COPY services/gateway/go.mod services/gateway/go.sum ./
RUN go mod download
COPY services/gateway/ ./
RUN CGO_ENABLED=0 go build -trimpath -o /api .
FROM alpine:3.23
COPY --from=build /api /usr/local/bin/api
USER 65532:65532
ENTRYPOINT ["api"]
