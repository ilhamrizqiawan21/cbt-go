.PHONY: run build test test-race vet fmt fmt-check check tidy

# Jalankan server dev (membaca .env di folder ini).
run:
	go run ./cmd/server

build:
	go build -o bin/server ./cmd/server

test:
	go test -p 1 ./...

# Tes dengan race detector: wajib sebelum item yang memakai goroutine dinyatakan DONE.
test-race:
	go test -p 1 -race ./...

vet:
	go vet ./...

fmt:
	gofmt -w .

# Gagal bila ada file yang belum diformat gofmt.
fmt-check:
	@out="$$(gofmt -l .)"; if [ -n "$$out" ]; then echo "Belum diformat:"; echo "$$out"; exit 1; fi

tidy:
	go mod tidy

# Gerbang kualitas standar: dijalankan eksekutor sebelum menandai item DONE.
check: fmt-check vet test
