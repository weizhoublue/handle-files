build-macos:
	mkdir -p dist/macos-arm64 dist/macos-amd64
	GOOS=darwin GOARCH=arm64 go build -o dist/macos-arm64/compress-vedio ./cmd/compress-vedio
	GOOS=darwin GOARCH=arm64 go build -o dist/macos-arm64/check-copy ./cmd/check-copy
	GOOS=darwin GOARCH=amd64 go build -o dist/macos-amd64/compress-vedio ./cmd/compress-vedio
	GOOS=darwin GOARCH=amd64 go build -o dist/macos-amd64/check-copy ./cmd/check-copy
