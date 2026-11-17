FROM golang:1.24-alpine AS build
WORKDIR /src
COPY go.mod ./
COPY . .
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/kinoperiferia . \
 && mkdir -p /out/data

FROM gcr.io/distroless/static-debian12
COPY --from=build /out/kinoperiferia /kinoperiferia
# Каталог данных (заявки и вложения) заранее принадлежит nonroot: иначе сайт
# не может его создать, а именованный том в compose наследует эти права.
COPY --from=build --chown=65532:65532 /out/data /data
ENV ORDERS_FILE=/data/orders.jsonl
EXPOSE 8080
USER nonroot:nonroot
ENTRYPOINT ["/kinoperiferia"]
