FROM golang:1.16

# Set the Current Working Directory inside the container
WORKDIR /consumer/

# Copy everything from the current directory to the PWD (Present Working Directory) inside the container
COPY . .


# Download all the dependencies
RUN go get -d -v ./...

#build
RUN go build -o ./out/consumer .

#exposes port 8080 to the outside world
EXPOSE 8080

# Run the executable
CMD ["./out/consumer"]