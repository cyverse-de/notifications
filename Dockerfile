FROM quay.io/goswagger/swagger as swagger

FROM golang:1.21 as build-root

WORKDIR /build

ENV CGO_ENABLED=0
ENV GOOS=linux
ENV GOARCH=amd64

COPY --from=swagger /usr/bin/swagger /usr/bin

COPY go.mod go.sum ./
RUN go mod download && go mod verify

COPY . .
RUN go build
RUN swagger generate spec -o ./swagger.json --scan-models

FROM scratch

WORKDIR /app

COPY --from=build-root /build/notifications /bin/notifications
COPY --from=build-root /build/swagger.json swagger.json

ENTRYPOINT ["notifications"]

EXPOSE 8080
