# Build a Lambda image using a Dockerfile (required by TestSQS.md).
# Output is a custom runtime bootstrap binary for AL2.

FROM golang:1.23 AS build

WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download

COPY . ./

ARG TARGETOS
ARG TARGETARCH

# Which Go package to build as the Lambda bootstrap entrypoint.
# SAM Image functions can pass this via Metadata.DockerBuildArgs.GO_MAIN.
ARG GO_MAIN=./cmd/dispatcher

RUN CGO_ENABLED=0 GOOS=${TARGETOS:-linux} GOARCH=${TARGETARCH:-arm64} \
    go build -trimpath -ldflags="-s -w" -o /out/bootstrap ${GO_MAIN}

FROM public.ecr.aws/lambda/provided:al2
COPY --from=build /out/bootstrap /var/runtime/bootstrap

# (Optional) document the entrypoint; Lambda base image uses it by default.
CMD ["bootstrap"]
