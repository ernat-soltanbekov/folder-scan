.PHONY: all test audit stress race vet fmt-check imports fuzz clean

all:
	go build -trimpath -o folder-scan .

test:
	go test ./...

audit: all imports
	python3 scripts/audit.py

stress: all
	python3 scripts/stress.py

race:
	go test -race ./...

vet:
	go vet ./...

fmt-check:
	@test -z "$$(gofmt -l main.go internal)" || (gofmt -l main.go internal; exit 1)

imports:
	python3 scripts/check_imports.py

fuzz:
	go test ./internal/ageprofile -run '^$$' -fuzz FuzzAgeBoundaries -fuzztime=10s
	go test ./internal/listing -run '^$$' -fuzz FuzzOptions -fuzztime=10s

clean:
	rm -f folder-scan coverage.out
