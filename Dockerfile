FROM golang:1.10 as builder

WORKDIR /go/src/github.com/gofunky/goplayspace/

#disable crosscompiling
ENV CGO_ENABLED=0

#compile linux only
ENV GOOS=linux

COPY ./ .

RUN dep ensure

RUN mkdir build

# build client
RUN ./bin/build-client

# build server
RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -ldflags '-extldflags "-static"' -o ./build/goplayspace ./server/*.go

FROM scratch
COPY --from=builder /go/src/github.com/gofunky/goplayspace/build/goplayspace /app/
WORKDIR /app
CMD ["./goplayspace"]
