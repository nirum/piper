# Build the piper binary
build:
    go build -o piper .

# Run all tests
test:
    go test ./...

# Run the program (pass args after --)
run *args:
    go run . {{args}}

# Remove build artifacts
clean:
    rm -f piper
