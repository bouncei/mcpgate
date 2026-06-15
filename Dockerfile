FROM golang:1.25-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -ldflags="-s -w" -o /out/mcpgate ./cmd/mcpgate

FROM gcr.io/distroless/static-debian12
COPY --from=build /out/mcpgate /usr/local/bin/mcpgate
ENTRYPOINT ["mcpgate"]
CMD ["serve", "-c", "/etc/mcpgate/config.yaml"]
