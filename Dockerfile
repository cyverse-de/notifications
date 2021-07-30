FROM golang:1.16-alpine

RUN apk add --no-cache make
RUN apk add --no-cache git
RUN go get -u github.com/jstemmer/go-junit-report

ENV CGO_ENABLED=0

WORKDIR /go/src/github.com/cyverse-de/notifications
COPY . .
RUN make

FROM scratch

WORKDIR /app

COPY --from=0 /go/src/github.com/cyverse-de/notifications/notifications /bin/notifications
COPY --from=0 /go/src/github.com/cyverse-de/notifications/swagger.json swagger.json

ENTRYPOINT ["notifications"]

EXPOSE 8080
