FROM golang:1.24-alpine AS build
WORKDIR /src
COPY go.mod ./
COPY . .
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/kinoperiferia .

FROM gcr.io/distroless/static-debian12
COPY --from=build /out/kinoperiferia /kinoperiferia
EXPOSE 8080
USER nonroot:nonroot
ENTRYPOINT ["/kinoperiferia"]
