.PHONY: test

all: vendor fmt build bundle

update:
	test -d vendor && rm -rf vendor || exit 0
	glide up --strip-vcs --update-vendored

vendor:
	go list github.com/Masterminds/glide
	glide install --strip-vcs --update-vendored

clean-bundle:
	@test -d public && rm -rf public || true

clean:
	rm -rf vendor bin

fmt:
	gofmt -w .

test:
	go test -v .

build: fmt test
