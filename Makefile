all: notifications

install-swagger:
	which swagger || go get -u github.com/go-swagger/go-swagger/cmd/swagger

swagger.json: install-swagger
	go mod vendor && swagger generate spec -o ./swagger.json --scan-models

notifications: swagger.json
	go build

clean:
	rm -rf swagger.json notifications vendor

.PHONY: install-swagger clean all
