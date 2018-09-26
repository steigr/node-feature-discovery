# Build the cpu tool
FROM golang as builder
ADD . /go/src/cpu
WORKDIR /go/src/cpu
RUN make

# Create a container for hosting the binary
FROM busybox
COPY --from=builder /go/src/cpu/cpu /cpu
