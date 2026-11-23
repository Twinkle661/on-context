FROM golang:1.23-alpine AS build
WORKDIR /src
COPY go.mod ./
COPY *.go ./
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/on-context . \
 && mkdir -p /out/data

FROM gcr.io/distroless/static-debian12
COPY --from=build /out/on-context /on-context
COPY config.example.json /etc/on-context/config.json
# pre-create /data owned by 1000:100 so named volumes inherit it too
COPY --from=build --chown=1000:100 /out/data /data
ENV ONCONTEXT_CONFIG=/etc/on-context/config.json
WORKDIR /data
VOLUME ["/data"]
EXPOSE 8080
# default uid:gid = 1000:100 (typical first user : "users" group).
# Override with `user:` in compose if your host differs.
USER 1000:100
ENTRYPOINT ["/on-context"]
