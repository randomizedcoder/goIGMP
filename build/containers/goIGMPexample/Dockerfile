#
# Dockerfile
#
# Derived from
# https://docs.docker.com/language/golang/build-images/
# https://blog.baeke.info/2021/03/28/distroless-or-scratch-for-go-apps/
# argument for Go version
ARG GO_VERSION=1.22

# STAGE 1: building the executable
FROM golang:${GO_VERSION}-alpine AS build

RUN ls -la
RUN pwd

WORKDIR /go/src
# This COPY command uses a wildcard to copy all files with .go extension located in the
# current directory on the host (the directory where the Dockerfile is located) into the
# current directory inside the image.
COPY . .
#COPY go.mod go.sum ./
#RUN go mod download

# Run tests
#RUN CGO_ENABLED=0 go test -timeout 30s -v github.com/gbaeke/go-template/pkg/api

# Build the executable
RUN CGO_ENABLED=0 go build -o /go/src/cmd/goIGMPexample/goIGMPexample /go/src/cmd/goIGMPexample/

# STAGE 2: build the container to run
FROM gcr.io/distroless/static AS final

LABEL maintainer="randomizedcoder"
USER nonroot:nonroot

# copy compiled app
COPY --from=build --chown=nonroot:nonroot /go/src/cmd/goIGMPexample/goIGMPexample /goIGMPexample
COPY --from=build --chown=nonroot:nonroot /go/src/pcaps/ipmpv3_membership_report_s_172.17.200.10_g_232_0_0_1.payload /

# run binary; use vector form
ENTRYPOINT ["/goIGMPexample","--filename", "/ipmpv3_membership_report_s_172.17.200.10_g_232_0_0_1.payload","--outName","eth0","--inName","lo","-queryNotify","-membershipReportsFromNetwork","-membershipReportsToNetwork","-unicastMembershipReports","-connectQueryToReport","-membershipReportsReader"]

# end