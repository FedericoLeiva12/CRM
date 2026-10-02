.PHONY: check backend-check frontend-check format format-check
check: backend-check frontend-check
backend-check: format-check
	cd backend && go tool staticcheck ./... && go vet ./... && go test -race ./...
frontend-check:
	cd web && npm run check
format:
	gofmt -w backend/cmd backend/internal
	cd web && npm run format
format-check:
	@cd backend && unformatted=$$(gofmt -l cmd internal); if [ -n "$$unformatted" ]; then echo "Go files need formatting:"; echo "$$unformatted"; exit 1; fi
