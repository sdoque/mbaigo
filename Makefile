# Run tests and log the test coverage
test:
	go test -v -coverprofile=".cover.out" $$(go list ./... | grep -v /tmp)

# Runs source code linters and catches common errors
lint:
	test -z $$(gofmt -l .) || (echo "Code isn't gofmt'ed!" && exit 1)
	go vet $$(go list ./... | grep -v /tmp)
	gosec -quiet -fmt=golint -exclude-dir="tmp" ./...

# pointerinterface ./...

# Runs spellchecker on the code and comments
# This requires this tool to be installed from https://github.com/crate-ci/typos?tab=readme-ov-file
# Example installation (if you have rust installed): cargo install typos-cli
spellcheck:
	typos .

# Generate pretty coverage report
analyse:
	go tool cover -html=".cover.out" -o="cover.html"
	@echo -e "\nCOVERAGE\n===================="
	go tool cover -func=.cover.out
	@echo -e "\nCYCLOMATIC COMPLEXITY\n===================="
	gocyclo -avg -top 10 -ignore test.go .

# Updates 3rd party packages and tools
deps:
	go mod download
	go install github.com/securego/gosec/v2/cmd/gosec@latest
	go install github.com/fzipp/gocyclo/cmd/gocyclo@latest
	go install code.larus.se/lmas/pointerinterface@latest

# Clean up built binary and other temporary files (ignores errors from rm)
clean:
	go clean
	rm .cover.out cover.html
