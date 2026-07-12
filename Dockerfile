FROM golang:1.26 AS build
ARG VERSION=dev
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w -X main.version=${VERSION#v}" -o /flixbus-mcp ./cmd/flixbus-mcp

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /flixbus-mcp /flixbus-mcp
EXPOSE 8748
ENTRYPOINT ["/flixbus-mcp"]
CMD ["-transport", "http", "-addr", ":8748"]
