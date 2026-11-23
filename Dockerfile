FROM golang:1.23-alpine AS build
WORKDIR /src
COPY go.mod ./
COPY *.go ./
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/on-context .

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/on-context /on-context
COPY config.example.json /etc/on-context/config.json
ENV ONCONTEXT_CONFIG=/etc/on-context/config.json
# nonroot = uid 65532; the mounted /data must be writable by it
VOLUME ["/data"]
EXPOSE 8080
USER nonroot:nonroot
ENTRYPOINT ["/on-context"]
