GO_TEST_FLAGS=

.PHONY: run test docker-build docker-run compose-up compose-down

# run starts the Hertz HTTP server locally.
run:
	cd cmd/server && go run .

# test runs all go tests.
test:
	go test ./... $${GO_TEST_FLAGS}

# docker-build builds the hertz-track Docker image using deploy/Dockerfile.
docker-build:
	docker build -t hertz-track:latest -f deploy/Dockerfile .

# docker-run runs a single hertz-track container using the image built above.
# For local development without external DB, it can run with in-memory storage.
# If you need Mongo/MySQL, pass env vars via -e or --env-file.
docker-run:
	docker run --rm --name hertz-track-api -p $${APP_PORT:-8080}:8080 hertz-track:latest

# compose-up starts the API and Mongo stack using docker-compose.
compose-up:
	cd deploy && docker compose up -d

# compose-down stops and removes the stack created by compose-up.
compose-down:
	cd deploy && docker compose down
