FROM golang:1.9-alpine AS build

WORKDIR /go/src/github.com/estesp/examplectr
COPY . .

RUN apk update && apk add make
RUN make static

FROM scratch
WORKDIR /usr/bin
COPY --from=build /go/src/github.com/estesp/examplectr/examplectr .
