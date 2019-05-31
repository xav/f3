# Build the application
FROM golang:1.12 as build

# Set the Current Working Directory inside the container
WORKDIR /f3query/

# Copy everything from the current directory to the PWD(Present Working Directory) inside the container
COPY . .

RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -ldflags "-s" -a -installsuffix cgo -o f3query .

# Copy it into the base image.
FROM gcr.io/distroless/base
COPY --from=build /f3query/f3query /
ENTRYPOINT ["/f3query"]
