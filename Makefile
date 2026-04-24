# .PHONY tells make these are command names, not files on disk. Without it,
# a stray file named "test" in the repo would silently stop `make test` from
# running.
.PHONY: test lint spellcheck runchecks analyse installpkgs clean

# Run tests and log the test coverage
test:
	go test -v -race -coverprofile=".cover.out" $$(go list ./... | grep -v /tmp)

# Runs source code linters and catches common errors
lint:
	test -z $$(gofmt -l .) || (echo "Code isn't gofmt'ed!" && exit 1)
	go vet $$(go list ./... | grep -v /tmp)
	gosec -quiet -fmt=golint -exclude-dir="tmp" ./...
	# Let staticcheck infer the Go version from go.mod. Hard-coding -go 1.26
	# breaks CI when a staticcheck release does not yet know that version.
	staticcheck ./...
	govulncheck -test ./...
	# pointerinterface ./...

# Runs spellchecker on the code and comments
# This requires this tool to be installed from https://github.com/crate-ci/typos?tab=readme-ov-file
# Example installation (if you have rust installed): cargo install typos-cli
spellcheck:
	typos .

# All in one check
runchecks: test lint spellcheck
	
# Generate pretty coverage report
analyse:
	go tool cover -html=".cover.out" -o="cover.html"
	@echo -e "\nCOVERAGE\n===================="
	go tool cover -func=.cover.out
	@echo -e "\nCYCLOMATIC COMPLEXITY\n===================="
	gocyclo -avg -top 10 .

# Updates 3rd party packages and tools
installpkgs:
	go mod download
	go install github.com/fzipp/gocyclo/cmd/gocyclo@latest
	go install github.com/securego/gosec/v2/cmd/gosec@latest
	go install honnef.co/go/tools/cmd/staticcheck@latest
	go install golang.org/x/vuln/cmd/govulncheck@latest
	# go install code.larus.se/lmas/pointerinterface@latest

# Clean up built binary and other temporary files. The leading "-" on rm
# tells make to ignore the error if the files do not exist yet (e.g. before
# the first `make test`). -f also silences rm's own "no such file" message.
clean:
	go clean
	-rm -f .cover.out cover.html
