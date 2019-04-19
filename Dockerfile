# Dockerfile for Golang application

FROM balenalib/raspberrypi3-debian-golang:latest AS builder

# Working directory outside $GOPATH
WORKDIR /src

# Copy go module files and download dependencies
COPY ./go.mod ./go.sum ./
RUN go mod download

# Copy source files
COPY ./ ./

# Build source files statically
RUN go build \
		-installsuffix 'static' \
		-o /app \
		.

# Minimal image for running the application
FROM balenalib/raspberrypi3-debian:latest AS final

# for sqlite3 and rpi binaries
RUN apt-get update -y && \
		apt-get install -y apt-utils libsqlite3-dev libraspberrypi-bin

# Copy files from temporary image
COPY --from=builder /app /

# Copy config file
COPY ./config.json /

# Open ports (if needed)
#EXPOSE 8080
#EXPOSE 80
#EXPOSE 443

# Entry point for the built application
ENTRYPOINT ["/app"]
